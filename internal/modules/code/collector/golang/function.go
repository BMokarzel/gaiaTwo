package golang

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"costEngine/internal/entity/node"
)

// relevantPkgSubstrings é a heurística MVP (F-007 D5/D6): inclui funções
// exportadas de pacotes cujo nome curto contém "handler", "service" ou
// "usecase". É deliberadamente conservador — preferimos falsos
// negativos a poluir o grafo.
var relevantPkgSubstrings = []string{"handler", "service", "usecase"}

// FuncFilterOptions controla quais funções entram no `EmitFunctions`.
// `IncludePkgSubstrings` substitui o default se não-nil; passar lista
// vazia (não-nil) desabilita o filtro por nome de pacote (qualquer
// pacote serve) — útil em testes.
type FuncFilterOptions struct {
	IncludePkgSubstrings []string
	SkipPathSubstrings   []string // default: "vendor/", "gen/", "mocks/"
}

// ExtractFunctions percorre `serviceAbsPath` (diretório do `go.mod`) e
// devolve `node.Function` por declaração relevante. Idempotente: roda
// duas vezes sobre o mesmo source → mesmas URNs.
//
// `serviceURN` é a URN do Service dono; `serviceModulePath` é o RelPath
// (".", "cmd/cli", etc.) — alimenta a URN da function.
// `repoRoot` é a raiz do repo (para resolver paths relativos no campo
// File).
func ExtractFunctions(
	repoRoot, serviceAbsPath, serviceModulePath string,
	serviceURN node.URN,
	repo string,
	opts FuncFilterOptions,
	emit EmitOptions,
) ([]node.Function, error) {
	includes := opts.IncludePkgSubstrings
	if includes == nil {
		includes = relevantPkgSubstrings
	}
	skipPaths := opts.SkipPathSubstrings
	if skipPaths == nil {
		skipPaths = []string{"vendor/", "gen/", "mocks/"}
	}
	now := emit.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	fset := token.NewFileSet()
	var out []node.Function

	werr := filepath.WalkDir(serviceAbsPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			if strings.HasPrefix(name, ".") && path != serviceAbsPath {
				return filepath.SkipDir
			}
			// Se este diretório contém outro go.mod (submódulo), pulamos —
			// pertence a outro Service.
			if path != serviceAbsPath {
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		slash := filepath.ToSlash(path)
		for _, s := range skipPaths {
			if strings.Contains(slash, s) {
				return nil
			}
		}

		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			// Ignorar arquivos malformados: outras ferramentas pegam.
			return nil
		}
		pkgName := file.Name.Name
		if !pkgMatches(pkgName, includes) {
			return nil
		}

		relFile, _ := filepath.Rel(repoRoot, path)
		relFile = filepath.ToSlash(relFile)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if fn.Name == nil || !isExported(fn.Name.Name) {
				continue
			}
			symbol := funcSymbol(fn)
			signature := renderSignature(fn.Type)
			sigSum := sha256.Sum256([]byte(signature))

			urn := node.NewFunctionURN(repo, serviceModulePath, pkgName, symbol)
			pos := fset.Position(fn.Pos())

			out = append(out, node.Function{
				Base: node.Base{
					NodeURN:  urn,
					NodeKind: node.KindFunction,
					NodeMeta: node.Meta{
						Version:    1,
						ValidFrom:  now,
						ObservedAt: now,
						Source: node.Source{
							Collector: "code/golang",
							RunID:     emit.RunID,
							Method:    node.MethodDeclared,
						},
						Confidence: 1.0,
					},
				},
				ServiceURN:    serviceURN,
				Package:       pkgName,
				Symbol:        symbol,
				File:          relFile,
				Line:          pos.Line,
				SignatureHash: hex.EncodeToString(sigSum[:]),
				Signature:     signature,
			})
		}
		return nil
	})
	if werr != nil {
		return nil, fmt.Errorf("extract functions: %w", werr)
	}
	return out, nil
}

// pkgMatches retorna true se a lista de includes for não-vazia E o nome
// curto do pacote contiver pelo menos uma das substrings. Se includes
// for vazia, qualquer pacote passa (modo "tudo", para testes).
func pkgMatches(pkg string, includes []string) bool {
	if len(includes) == 0 {
		return true
	}
	lower := strings.ToLower(pkg)
	for _, s := range includes {
		if strings.Contains(lower, strings.ToLower(s)) {
			return true
		}
	}
	return false
}

func isExported(name string) bool {
	if name == "" {
		return false
	}
	return unicode.IsUpper([]rune(name)[0])
}

// funcSymbol formata o nome da função. Métodos viram `(T).Method` ou
// `(*T).Method` — preserva polimorfismo no grafo.
func funcSymbol(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := fn.Recv.List[0].Type
	typ := exprToString(recv)
	return "(" + typ + ")." + fn.Name.Name
}

// renderSignature produz string canônica determinística para a
// assinatura. Não é Go-válida — só identifica unicamente.
func renderSignature(t *ast.FuncType) string {
	var sb strings.Builder
	sb.WriteByte('(')
	if t.Params != nil {
		for i, p := range t.Params.List {
			if i > 0 {
				sb.WriteByte(',')
			}
			// Cada param.Names pode ter múltiplos identifiers (`a, b int`).
			n := 1
			if len(p.Names) > 1 {
				n = len(p.Names)
			}
			ty := exprToString(p.Type)
			for j := 0; j < n; j++ {
				if j > 0 {
					sb.WriteByte(',')
				}
				sb.WriteString(ty)
			}
		}
	}
	sb.WriteByte(')')
	if t.Results != nil && len(t.Results.List) > 0 {
		sb.WriteByte('(')
		for i, r := range t.Results.List {
			if i > 0 {
				sb.WriteByte(',')
			}
			n := 1
			if len(r.Names) > 1 {
				n = len(r.Names)
			}
			ty := exprToString(r.Type)
			for j := 0; j < n; j++ {
				if j > 0 {
					sb.WriteByte(',')
				}
				sb.WriteString(ty)
			}
		}
		sb.WriteByte(')')
	}
	return sb.String()
}

// exprToString serializa uma expressão de tipo. Cobre os casos comuns
// (ident, *T, []T, map[K]V, selector, ...T, func(...)).
func exprToString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return "*" + exprToString(v.X)
	case *ast.SelectorExpr:
		return exprToString(v.X) + "." + v.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprToString(v.Elt)
	case *ast.MapType:
		return "map[" + exprToString(v.Key) + "]" + exprToString(v.Value)
	case *ast.Ellipsis:
		return "..." + exprToString(v.Elt)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.ChanType:
		return "chan " + exprToString(v.Value)
	case *ast.FuncType:
		return "func" + renderSignature(v)
	default:
		return fmt.Sprintf("%T", e)
	}
}

