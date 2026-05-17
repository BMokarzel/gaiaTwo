package golang

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"costEngine/internal/entity/node"
)

// chiMethods são os métodos chamados em `*chi.Mux` / `chi.Router` que
// registram um handler para um (METHOD, ROUTE). `Handle` e `MethodFunc`
// recebem o método como string literal (tratamos no callsite).
var chiMethods = map[string]string{
	"Get":     "GET",
	"Post":    "POST",
	"Put":    "PUT",
	"Delete": "DELETE",
	"Patch":  "PATCH",
	"Head":   "HEAD",
	"Options": "OPTIONS",
	"Connect": "CONNECT",
	"Trace":   "TRACE",
}

// ExtractEndpoints percorre o Service e devolve `node.Endpoint` para
// cada chamada que registra um handler HTTP. Suporta:
//
//   - net/http: `http.HandleFunc("/x", h)`, `mux.HandleFunc("/x", h)`,
//     `mux.Handle("/x", h)`. Method = "ANY".
//   - go-chi/chi: `r.Get/Post/...("/x", h)`, `r.Handle/Method/MethodFunc(...)`.
//
// Detecção é puramente sintática (não resolve types) — preferimos falsos
// negativos a inflar o grafo com matches espúrios.
func ExtractEndpoints(
	repoRoot, serviceAbsPath, serviceModulePath string,
	serviceURN node.URN,
	repo string,
	skipPathSubstrings []string,
	emit EmitOptions,
) ([]node.Endpoint, error) {
	if skipPathSubstrings == nil {
		skipPathSubstrings = []string{"vendor/", "gen/", "mocks/"}
	}
	now := emit.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	fset := token.NewFileSet()
	var out []node.Endpoint

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
		for _, s := range skipPathSubstrings {
			if strings.Contains(slash, s) {
				return nil
			}
		}

		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution|parser.ParseComments)
		if perr != nil {
			return nil
		}
		pkgName := file.Name.Name
		relFile, _ := filepath.Rel(repoRoot, path)
		relFile = filepath.ToSlash(relFile)

		// Pré-coleta tags por FuncDecl (span de linhas) — endpoints
		// registrados dentro herdam o `// @feature:` da função pai.
		type funcTags struct {
			lineInit, lineEnd int
			tags              []string
		}
		var fnTagSpans []funcTags
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				ts := extractFeatureTagsFromDoc(fn.Doc)
				if len(ts) == 0 {
					continue
				}
				fnTagSpans = append(fnTagSpans, funcTags{
					lineInit: fset.Position(fn.Pos()).Line,
					lineEnd:  fset.Position(fn.End()).Line,
					tags:     ts,
				})
			}
		}
		tagsForLine := func(line int) []string {
			for _, s := range fnTagSpans {
				if line >= s.lineInit && line <= s.lineEnd {
					return s.tags
				}
			}
			return nil
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			method, route, handler, framework, found := matchEndpointCall(call, pkgName)
			if !found {
				return true
			}
			urn := node.NewEndpointURN(repo, serviceModulePath, method, route)
			pos := fset.Position(call.Pos())
			endPos := fset.Position(call.End())
			out = append(out, node.Endpoint{
				Base: node.Base{
					NodeURN:  urn,
					NodeKind: node.KindEndpoint,
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
				ServiceURN:  serviceURN,
				Method:      method,
				Route:       route,
				Handler:     handler,
				Framework:   framework,
				FeatureTags: tagsForLine(pos.Line),
				Location: node.Location{
					File:     relFile,
					LineInit: pos.Line,
					LineEnd:  endPos.Line,
					ColInit:  pos.Column,
					ColEnd:   endPos.Column,
				},
				// Deprecated mirrors.
				File: relFile,
				Line: pos.Line,
			})
			return true
		})
		return nil
	})
	if werr != nil {
		return nil, fmt.Errorf("extract endpoints: %w", werr)
	}
	return out, nil
}

// matchEndpointCall inspeciona uma CallExpr e devolve (method, route,
// handler, framework, true) se for um registro de handler HTTP
// reconhecido.
//
// `pkgName` é o pacote do arquivo — usado só para qualificar handlers
// passados por identifier simples.
func matchEndpointCall(call *ast.CallExpr, pkgName string) (string, string, string, string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", "", "", "", false
	}
	name := sel.Sel.Name

	// --- net/http ---
	if name == "HandleFunc" || name == "Handle" {
		// http.HandleFunc / mux.HandleFunc / mux.Handle
		// (também aceitamos chi: HandleFunc com 2 args literal,
		// mas chi.Handle recebe (pattern, http.Handler) — mesmo shape).
		if len(call.Args) < 2 {
			return "", "", "", "", false
		}
		route, ok := stringLit(call.Args[0])
		if !ok {
			return "", "", "", "", false
		}
		handler := exprToString(call.Args[1])
		handler = qualifyHandler(handler, pkgName)

		fw := "net/http"
		// Heurística suave: se o receiver não é `http` nem identifier
		// `*ServeMux`-like, assumimos chi quando o symbol bate seu set.
		// Não temos type info aqui; manter "net/http" como default é
		// mais conservador.
		if recv, ok := sel.X.(*ast.Ident); ok && recv.Name == "http" {
			fw = "net/http"
		}
		return "ANY", route, handler, fw, true
	}

	// --- chi: r.Get/Post/Put/Delete/Patch/Head/Options/Connect/Trace ---
	if m, ok := chiMethods[name]; ok {
		if len(call.Args) < 2 {
			return "", "", "", "", false
		}
		route, ok := stringLit(call.Args[0])
		if !ok {
			return "", "", "", "", false
		}
		handler := qualifyHandler(exprToString(call.Args[1]), pkgName)
		return m, route, handler, "chi", true
	}

	// --- chi: r.Method(METHOD, pattern, h)  /  r.MethodFunc(METHOD, pattern, h) ---
	if name == "Method" || name == "MethodFunc" {
		if len(call.Args) < 3 {
			return "", "", "", "", false
		}
		methodStr, ok := stringLit(call.Args[0])
		if !ok {
			return "", "", "", "", false
		}
		route, ok := stringLit(call.Args[1])
		if !ok {
			return "", "", "", "", false
		}
		handler := qualifyHandler(exprToString(call.Args[2]), pkgName)
		return strings.ToUpper(methodStr), route, handler, "chi", true
	}

	return "", "", "", "", false
}

// stringLit extrai o valor de um string literal, ou ("", false) se não
// for literal. Endpoints com route dinâmica (variável) ficam fora — o
// grafo só guarda o que conseguimos identificar deterministicamente.
func stringLit(e ast.Expr) (string, bool) {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s := bl.Value
	if len(s) >= 2 && (s[0] == '"' || s[0] == '`') {
		return s[1 : len(s)-1], true
	}
	return s, true
}

// qualifyHandler garante que o handler tenha forma `pkg.Symbol`. Se vier
// como identifier simples (`HandleX`) e tivermos o pacote do arquivo,
// prefixa.
func qualifyHandler(expr, pkg string) string {
	if expr == "" {
		return ""
	}
	// já qualificado / método / lambda → deixa como está
	if strings.ContainsAny(expr, ".(){}") {
		return expr
	}
	if pkg == "" {
		return expr
	}
	return pkg + "." + expr
}
