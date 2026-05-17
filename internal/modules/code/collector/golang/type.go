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

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

// ExtractTypes percorre `serviceAbsPath` e devolve `node.Type` para
// cada `*ast.TypeSpec` exportado em pacotes relevantes (mesmo filtro
// de ExtractFunctions). Suporta:
//
//   - struct  → TypeKindStruct + Fields populados (ADR-008)
//   - interface → TypeKindInterface + Methods (assinaturas serializadas)
//   - alias `type X = Y` → TypeKindAlias + ALIASES edge
//   - definição `type X Y` → tratada como struct/alias conforme RHS
//
// Embedded fields em struct emitem aresta EXTENDS Type→Type quando o
// nome do embed bate com algum Type indexado no service.
func ExtractTypes(
	repoRoot, serviceAbsPath, serviceModulePath, goModule string,
	serviceURN node.URN,
	repo string,
	opts FuncFilterOptions,
	emit EmitOptions,
) ([]node.Type, []node.Variable, []edge.Extends, []edge.Aliases, error) {
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
	var types []node.Type
	var vars []node.Variable
	// Pass 1: coleta types — precisamos do índice para resolver EXTENDS/ALIASES.
	type seen struct {
		urn       node.URN
		namespace string
		symbol    string
	}
	var collected []seen
	type pending struct {
		typeURN    node.URN
		embedded   []string // nomes de embed dentro de struct
		aliasOf    string   // nome do RHS quando alias
		namespace  string
	}
	var pendings []pending

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
		for _, s := range skipPaths {
			if strings.Contains(slash, s) {
				return nil
			}
		}
		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution|parser.ParseComments)
		if perr != nil {
			return nil
		}
		pkgName := file.Name.Name
		if !pkgMatches(pkgName, includes) {
			return nil
		}
		relFile, _ := filepath.Rel(repoRoot, path)
		relFile = filepath.ToSlash(relFile)
		namespace := goNamespace(goModule, serviceAbsPath, filepath.Dir(path))

		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			switch gen.Tok {
			case token.TYPE:
				for _, spec := range gen.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if !isExported(ts.Name.Name) {
						continue
					}
					doc := ts.Doc
					if doc == nil {
						doc = gen.Doc
					}
					typeNode, p := buildType(ts, fset, repo, serviceModulePath, namespace, relFile, serviceURN, now, emit)
					typeNode.FeatureTags = extractFeatureTagsFromDoc(doc)
					types = append(types, typeNode)
					collected = append(collected, seen{typeNode.URN(), namespace, ts.Name.Name})
					if p != nil {
						pendings = append(pendings, *p)
					}
				}
			case token.VAR, token.CONST:
				mut := node.VariableMut
				if gen.Tok == token.CONST {
					mut = node.VariableConst
				}
				for _, spec := range gen.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					doc := vs.Doc
					if doc == nil {
						doc = gen.Doc
					}
					tags := extractFeatureTagsFromDoc(doc)
					for _, ident := range vs.Names {
						if !isExported(ident.Name) {
							continue
						}
						v := buildVariable(ident, vs, mut, repo, serviceModulePath, namespace, relFile, serviceURN, fset, now, emit)
						v.FeatureTags = tags
						vars = append(vars, v)
					}
				}
			}
		}
		return nil
	})
	if werr != nil {
		return nil, nil, nil, nil, fmt.Errorf("extract types: %w", werr)
	}

	// Pass 2: resolver EXTENDS/ALIASES via símbolo curto dentro do mesmo namespace.
	symIdx := map[string]map[string]node.URN{} // namespace → symbol → URN
	for _, s := range collected {
		if symIdx[s.namespace] == nil {
			symIdx[s.namespace] = map[string]node.URN{}
		}
		symIdx[s.namespace][s.symbol] = s.urn
	}

	var extends []edge.Extends
	var aliases []edge.Aliases
	for _, p := range pendings {
		idx := symIdx[p.namespace]
		for _, name := range p.embedded {
			if target, ok := idx[name]; ok {
				extends = append(extends, newExtends(p.typeURN, target, now, emit))
			}
		}
		if p.aliasOf != "" {
			if target, ok := idx[p.aliasOf]; ok {
				aliases = append(aliases, newAliases(p.typeURN, target, now, emit))
			}
		}
	}
	return types, vars, extends, aliases, nil
}

// buildType monta o `node.Type` + lista de embeds/alias para
// resolução posterior.
func buildType(
	ts *ast.TypeSpec,
	fset *token.FileSet,
	repo, serviceModulePath, namespace, relFile string,
	serviceURN node.URN,
	now time.Time,
	emit EmitOptions,
) (node.Type, *struct {
	typeURN   node.URN
	embedded  []string
	aliasOf   string
	namespace string
}) {
	symbol := ts.Name.Name
	urn := node.NewTypeURN(repo, serviceModulePath, namespace, symbol)
	start := fset.Position(ts.Pos())
	end := fset.Position(ts.End())

	out := node.Type{
		Base: node.Base{
			NodeURN:  urn,
			NodeKind: node.KindType,
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
		ServiceURN: serviceURN,
		Namespace:  namespace,
		Symbol:     symbol,
		Location: node.Location{
			File:     relFile,
			LineInit: start.Line,
			LineEnd:  end.Line,
			ColInit:  start.Column,
			ColEnd:   end.Column,
		},
		Exported: true,
	}

	pending := &struct {
		typeURN   node.URN
		embedded  []string
		aliasOf   string
		namespace string
	}{typeURN: urn, namespace: namespace}

	switch t := ts.Type.(type) {
	case *ast.StructType:
		out.Kind_ = node.TypeKindStruct
		if t.Fields != nil {
			pos := 0
			for _, f := range t.Fields.List {
				typeRef := exprToString(f.Type)
				if len(f.Names) == 0 {
					// Campo embedded.
					embedName := lastIdent(typeRef)
					if embedName != "" {
						pending.embedded = append(pending.embedded, embedName)
					}
					out.Fields = append(out.Fields, node.FieldSlot{
						Name:     embedName,
						Position: pos,
						TypeRef:  typeRef,
						Tags:     extractStructTags(f.Tag),
					})
					pos++
					continue
				}
				for _, n := range f.Names {
					out.Fields = append(out.Fields, node.FieldSlot{
						Name:     n.Name,
						Position: pos,
						TypeRef:  typeRef,
						Tags:     extractStructTags(f.Tag),
					})
					pos++
				}
			}
		}
	case *ast.InterfaceType:
		out.Kind_ = node.TypeKindInterface
		if t.Methods != nil {
			for _, m := range t.Methods.List {
				for _, name := range m.Names {
					out.Methods = append(out.Methods, node.MethodSlot{Name: name.Name})
				}
				if len(m.Names) == 0 {
					// Embedded interface.
					embedName := lastIdent(exprToString(m.Type))
					if embedName != "" {
						pending.embedded = append(pending.embedded, embedName)
					}
				}
			}
		}
	default:
		// `type X Y` (named definition) ou `type X = Y` (alias).
		if ts.Assign.IsValid() {
			out.Kind_ = node.TypeKindAlias
		} else {
			out.Kind_ = node.TypeKindAlias // tratamos definidos como alias no MVP
		}
		pending.aliasOf = lastIdent(exprToString(ts.Type))
	}

	if len(pending.embedded) == 0 && pending.aliasOf == "" {
		return out, nil
	}
	return out, pending
}

// buildVariable monta o `node.Variable` para um ValueSpec.
func buildVariable(
	ident *ast.Ident,
	vs *ast.ValueSpec,
	mut node.VariableMutability,
	repo, serviceModulePath, namespace, relFile string,
	serviceURN node.URN,
	fset *token.FileSet,
	now time.Time,
	emit EmitOptions,
) node.Variable {
	urn := node.NewVariableURN(repo, serviceModulePath, namespace, ident.Name)
	start := fset.Position(ident.Pos())
	end := fset.Position(ident.End())
	typeRef := ""
	if vs.Type != nil {
		typeRef = exprToString(vs.Type)
	}
	return node.Variable{
		Base: node.Base{
			NodeURN:  urn,
			NodeKind: node.KindVariable,
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
		ServiceURN: serviceURN,
		Namespace:  namespace,
		Symbol:     ident.Name,
		TypeRef:    typeRef,
		Mutability: mut,
		HasInit:    len(vs.Values) > 0,
		Exported:   true,
		Location: node.Location{
			File:     relFile,
			LineInit: start.Line,
			LineEnd:  end.Line,
			ColInit:  start.Column,
			ColEnd:   end.Column,
		},
	}
}

// lastIdent extrai o último identifier "alpha-num" de uma string de
// tipo serializada (`*pkg.Foo` → `Foo`; `[]Bar` → `Bar`).
func lastIdent(s string) string {
	if s == "" {
		return ""
	}
	// Remove modifiers comuns.
	s = strings.TrimPrefix(s, "*")
	s = strings.TrimPrefix(s, "[]")
	s = strings.TrimPrefix(s, "...")
	if i := strings.LastIndex(s, "."); i >= 0 {
		s = s[i+1:]
	}
	// Mantém apenas até o primeiro char não alfanum ou _.
	for i, r := range s {
		if !(r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return s[:i]
		}
	}
	return s
}

// extractStructTags faz parse minimal de struct tags `key:"value"`.
// Retorna nil se vazio.
func extractStructTags(lit *ast.BasicLit) map[string]string {
	if lit == nil || lit.Kind != token.STRING {
		return nil
	}
	raw := lit.Value
	if len(raw) >= 2 {
		raw = raw[1 : len(raw)-1] // remove ` ou "
	}
	if raw == "" {
		return nil
	}
	out := map[string]string{}
	for _, part := range strings.Fields(raw) {
		i := strings.Index(part, ":")
		if i < 0 {
			continue
		}
		key := part[:i]
		val := strings.Trim(part[i+1:], "\"")
		out[key] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func newExtends(from, to node.URN, now time.Time, emit EmitOptions) edge.Extends {
	return edge.Extends{
		Base: edge.Base{
			EdgeID:   edge.DeterministicID(from, edge.TypeExtends, to, now),
			EdgeType: edge.TypeExtends,
			FromURN:  from,
			ToURN:    to,
			EdgeMeta: edge.Meta{
				ValidFrom:   now,
				ObservedAt:  now,
				Source:      node.Source{Collector: "code/golang", RunID: emit.RunID, Method: node.MethodDeclared},
				Confidence:  1.0,
				Directional: true,
			},
		},
	}
}

func newAliases(from, to node.URN, now time.Time, emit EmitOptions) edge.Aliases {
	return edge.Aliases{
		Base: edge.Base{
			EdgeID:   edge.DeterministicID(from, edge.TypeAliases, to, now),
			EdgeType: edge.TypeAliases,
			FromURN:  from,
			ToURN:    to,
			EdgeMeta: edge.Meta{
				ValidFrom:   now,
				ObservedAt:  now,
				Source:      node.Source{Collector: "code/golang", RunID: emit.RunID, Method: node.MethodDeclared},
				Confidence:  1.0,
				Directional: true,
			},
		},
	}
}
