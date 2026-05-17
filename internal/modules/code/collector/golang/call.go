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

// callRule descreve um pattern reconhecido: `pkg.Method(...)` ou
// `<recv>.Method(...)`. Encaixa por nome do selector (`Sel.Name`) e,
// quando relevante, pelo qualifier (`X.Name`).
//
// Estamos no MVP F-019/F-020: detecção sintática, sem type resolution.
// Falsos negativos > falsos positivos (mesmo princípio do endpoint).
type callRule struct {
	// Match
	pkgQualifier string         // "" = ignora; "http" significa que receiver é Ident "http"
	method       string         // "Get", "Query", etc.
	kind         node.CallKind  // família resultante
	operation    string         // só usado em DataAccess (select/insert/...)
	framework    string         // ecossistema/framework para USES
}

// boundaryRules lista os pares reconhecidos. Lista intencionalmente
// curta — começamos por net/http (cliente) + database/sql, depois
// estendida para gRPC, AWS messaging e schedulers (F-019).
//
// Detecção é sintática (sem type-resolve): preferimos falsos negativos
// a falsos positivos. Confidence no Call é 0.9 — caller pode filtrar.
var boundaryRules = []callRule{
	// --- net/http client ---
	{pkgQualifier: "http", method: "Get", kind: node.CallHttpCall, operation: "GET", framework: "net/http"},
	{pkgQualifier: "http", method: "Post", kind: node.CallHttpCall, operation: "POST", framework: "net/http"},
	{pkgQualifier: "http", method: "PostForm", kind: node.CallHttpCall, operation: "POST", framework: "net/http"},
	{pkgQualifier: "http", method: "Head", kind: node.CallHttpCall, operation: "HEAD", framework: "net/http"},
	{pkgQualifier: "http", method: "Do", kind: node.CallHttpCall, framework: "net/http"},

	// --- database/sql, pgx, gorm — by method name (não exigimos qualifier) ---
	{method: "Query", kind: node.CallDataAccess, operation: "select", framework: "database/sql"},
	{method: "QueryContext", kind: node.CallDataAccess, operation: "select", framework: "database/sql"},
	{method: "QueryRow", kind: node.CallDataAccess, operation: "select", framework: "database/sql"},
	{method: "QueryRowContext", kind: node.CallDataAccess, operation: "select", framework: "database/sql"},
	{method: "Exec", kind: node.CallDataAccess, operation: "exec", framework: "database/sql"},
	{method: "ExecContext", kind: node.CallDataAccess, operation: "exec", framework: "database/sql"},

	// --- gRPC (google.golang.org/grpc) ---
	// `conn.Invoke(ctx, "/svc/Method", req, resp)` é o caminho unário
	// genérico; stubs gerados são indistinguíveis de method calls sem
	// type-resolve, então só capturamos o sintático claro aqui.
	{method: "Invoke", kind: node.CallRpcCall, framework: "google.golang.org/grpc"},
	{method: "NewStream", kind: node.CallRpcCall, operation: "stream", framework: "google.golang.org/grpc"},

	// --- AWS SQS (aws-sdk-go / v2) — métodos do client ---
	{method: "SendMessage", kind: node.CallQueueSend, framework: "aws-sdk-go-v2/sqs"},
	{method: "SendMessageBatch", kind: node.CallQueueSend, operation: "batch", framework: "aws-sdk-go-v2/sqs"},
	{method: "ReceiveMessage", kind: node.CallQueueReceive, framework: "aws-sdk-go-v2/sqs"},
	{method: "DeleteMessage", kind: node.CallQueueReceive, operation: "ack", framework: "aws-sdk-go-v2/sqs"},

	// --- AWS SNS ---
	{method: "Publish", kind: node.CallEventPublish, framework: "aws-sdk-go-v2/sns"},
	{method: "PublishBatch", kind: node.CallEventPublish, operation: "batch", framework: "aws-sdk-go-v2/sns"},

	// --- AWS Kinesis ---
	{method: "PutRecord", kind: node.CallEventPublish, framework: "aws-sdk-go-v2/kinesis"},
	{method: "PutRecords", kind: node.CallEventPublish, operation: "batch", framework: "aws-sdk-go-v2/kinesis"},

	// --- Schedulers (robfig/cron v3) ---
	// `c.AddFunc("@every 5m", fn)` / `c.AddJob(spec, job)` registram um
	// callback agendado; a função alvo é o callback (resolvido como
	// FunctionCall em pass separado quando expressão é identificador).
	{method: "AddFunc", kind: node.CallJobSchedule, framework: "github.com/robfig/cron"},
	{method: "AddJob", kind: node.CallJobSchedule, framework: "github.com/robfig/cron"},
}

// ExtractCalls percorre `serviceAbsPath` e devolve `node.Call` para
// cada callsite reconhecido como boundary call dentro de uma Function
// declarante. Funções reconhecidas vêm do índice `functionsByPos` —
// callsites fora de uma Function indexada (top-level init, var =
// expr) são ignorados no MVP.
//
// `functionsByPos` mapeia `(file, lineStart, lineEnd)` → Function URN.
// Construído pelo caller a partir do output de ExtractFunctions.
func ExtractCalls(
	repoRoot, serviceAbsPath, serviceModulePath string,
	serviceURN node.URN,
	repo string,
	skipPathSubstrings []string,
	functions []node.Function,
	emit EmitOptions,
) ([]node.Call, []edge.Invokes, []edge.Targets, []edge.Uses, error) {
	if skipPathSubstrings == nil {
		skipPathSubstrings = []string{"vendor/", "gen/", "mocks/"}
	}
	now := emit.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	// Index Functions por file → spans.
	byFile := map[string][]funcSpan{}
	for _, f := range functions {
		byFile[f.Location.File] = append(byFile[f.Location.File], funcSpan{f.URN(), f.Location.LineInit, f.Location.LineEnd})
	}
	// Index para resolução in-process (F-020): "method-name" → URN(s).
	// Mantemos por nome curto pra suportar `recv.Method(...)`. Quando
	// múltiplos matches → IsDynamic=true (não tem como desambiguar sem
	// type-resolve).
	bySymbol := map[string][]node.URN{}
	for _, f := range functions {
		bySymbol[shortMethodName(f.Symbol)] = append(bySymbol[shortMethodName(f.Symbol)], f.URN())
	}
	// F-028: Call herda feature_tags da Function declarante.
	tagsByCaller := map[node.URN][]string{}
	for _, f := range functions {
		if len(f.FeatureTags) > 0 {
			tagsByCaller[f.URN()] = f.FeatureTags
		}
	}

	fset := token.NewFileSet()
	var calls []node.Call
	var invokes []edge.Invokes
	var targets []edge.Targets
	var uses []edge.Uses

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

		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return nil
		}
		relFile, _ := filepath.Rel(repoRoot, path)
		relFile = filepath.ToSlash(relFile)

		// Ordinal por function — reset a cada FuncDecl.
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			start := fset.Position(fn.Pos()).Line
			end := fset.Position(fn.End()).Line
			caller := findCaller(byFile[relFile], start, end)
			if caller == "" {
				continue
			}
			ordinal := 0
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				rule, matched := matchBoundaryCall(call)
				if !matched {
					// F-020 in-process: tenta resolver como MethodCall
					// quando o target bate no índice de Functions.
					if c, inv, tgt, ok := tryInProcess(call, fset, caller, repo, ordinal, relFile, bySymbol, now, emit); ok {
						c.FeatureTags = tagsByCaller[caller]
						calls = append(calls, c)
						invokes = append(invokes, inv)
						if tgt != nil {
							targets = append(targets, *tgt)
						}
						ordinal++
					}
					return true
				}
				pos := fset.Position(call.Pos())
				endPos := fset.Position(call.End())
				urn := node.NewCallURN(repo, rule.kind, caller, ordinal)
				c := node.Call{
					Base: node.Base{
						NodeURN:  urn,
						NodeKind: node.KindCall,
						NodeMeta: node.Meta{
							Version:    1,
							ValidFrom:  now,
							ObservedAt: now,
							Source: node.Source{
								Collector: "code/golang",
								RunID:     emit.RunID,
								Method:    node.MethodDeclared,
							},
							Confidence: 0.9, // sintático sem type-resolve
						},
					},
					Kind_:         rule.kind,
					CallerURN:     caller,
					Ordinal:       ordinal,
					OperationKind: rule.operation,
					TargetSymbol:  callSymbol(call),
					Location: node.Location{
						File:     relFile,
						LineInit: pos.Line,
						LineEnd:  endPos.Line,
						ColInit:  pos.Column,
						ColEnd:   endPos.Column,
					},
				}
				// HttpCall: tenta URL literal no primeiro arg.
				if rule.kind == node.CallHttpCall && len(call.Args) > 0 {
					if u, ok := stringLit(call.Args[0]); ok {
						c.TargetURL = u
					}
					if rule.operation != "" {
						c.TargetMethod = rule.operation
					}
				}
				// gRPC Invoke: o 2º arg é o method path "/pkg.Svc/Method".
				if rule.kind == node.CallRpcCall && len(call.Args) >= 2 {
					if u, ok := stringLit(call.Args[1]); ok {
						c.TargetURL = u
					}
				}
				// Schedulers: 1º arg é a spec ("@every 5m", "0 0 * * *").
				if rule.kind == node.CallJobSchedule && len(call.Args) >= 1 {
					if spec, ok := stringLit(call.Args[0]); ok {
						c.Topic = spec
					}
				}

				// FrameworkURN para USES.
				fwURN := node.NewFrameworkURN("go", rule.framework)
				c.FrameworkURN = fwURN

				// F-028: herda feature_tags do caller.
				c.FeatureTags = tagsByCaller[caller]

				calls = append(calls, c)
				invokes = append(invokes, newInvokes(caller, urn, now, emit))
				if c.TargetURL != "" {
					// Sem TARGETS quando alvo é URL literal externa: registramos
					// só TARGETURL no Call (alvo URN não-resolvível). Mantemos o
					// nó sem aresta TARGETS — query depende disso pra distinguir.
				}
				uses = append(uses, newUses(urn, fwURN, now, emit))
				ordinal++
				return true
			})
		}
		return nil
	})
	if werr != nil {
		return nil, nil, nil, nil, fmt.Errorf("extract calls: %w", werr)
	}
	return calls, invokes, targets, uses, nil
}

// funcSpan referencia a Function declarante por intervalo de linhas
// no arquivo. Como ExtractFunctions só emite funções "relevantes"
// (pacotes handler/service/usecase), callers fora desse índice ficam
// invisíveis ao detector de Call no MVP.
type funcSpan struct {
	urn      node.URN
	lineInit int
	lineEnd  int
}

// findCaller devolve a Function URN cujo span (lineInit..lineEnd)
// envolve `start..end`. Como funções não se sobrepõem, basta o
// primeiro match. Retorna "" quando não há caller indexado.
func findCaller(spans []funcSpan, start, end int) node.URN {
	for _, s := range spans {
		if start >= s.lineInit && end <= s.lineEnd {
			return s.urn
		}
	}
	return ""
}

// matchBoundaryCall confere se a CallExpr bate alguma regra.
func matchBoundaryCall(call *ast.CallExpr) (callRule, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return callRule{}, false
	}
	method := sel.Sel.Name
	var qualifier string
	if id, ok := sel.X.(*ast.Ident); ok {
		qualifier = id.Name
	}
	for _, r := range boundaryRules {
		if r.method != method {
			continue
		}
		if r.pkgQualifier != "" && r.pkgQualifier != qualifier {
			continue
		}
		return r, true
	}
	return callRule{}, false
}

// callSymbol devolve forma legível "<recv>.<method>", "<pkg>.<method>"
// ou "<func>" para chamadas diretas.
func callSymbol(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		return exprToString(fn.X) + "." + fn.Sel.Name
	case *ast.Ident:
		return fn.Name
	}
	return ""
}

func newInvokes(from, to node.URN, now time.Time, emit EmitOptions) edge.Invokes {
	return edge.Invokes{
		Base: edge.Base{
			EdgeID:   edge.DeterministicID(from, edge.TypeInvokes, to, now),
			EdgeType: edge.TypeInvokes,
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

// shortMethodName devolve só a parte do método em símbolos formatados
// `(Type).Method` / `(*Type).Method` — usado como chave de resolução
// in-process. Em funções top-level, devolve o próprio símbolo.
func shortMethodName(symbol string) string {
	if i := strings.LastIndex(symbol, ")."); i >= 0 {
		return symbol[i+2:]
	}
	return symbol
}

// tryInProcess inspeciona uma CallExpr não-boundary e tenta resolvê-la
// como FunctionCall/MethodCall via `bySymbol`. Retorna o nó Call já
// preenchido, a aresta INVOKES, e (opcionalmente) TARGETS quando o
// símbolo bate unicamente. Quando não casa nenhum símbolo, retorna
// `ok=false` — call não é emitido (controla ruído no MVP).
func tryInProcess(
	call *ast.CallExpr,
	fset *token.FileSet,
	caller node.URN,
	repo string,
	ordinal int,
	relFile string,
	bySymbol map[string][]node.URN,
	now time.Time,
	emit EmitOptions,
) (node.Call, edge.Invokes, *edge.Targets, bool) {
	// Caso 1: chamada qualificada — `pkg.Func(...)` ou `recv.Method(...)`.
	// MVP trata ambos como MethodCall (indistinguíveis sem type-resolve).
	//
	// Caso 2: chamada direta — `Helper(args)`. Quando o ident bate em
	// uma Function indexada, emitimos FunctionCall (F-020).
	var (
		method     string
		kind       node.CallKind
	)
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		method = fn.Sel.Name
		kind = node.CallMethodCall
	case *ast.Ident:
		method = fn.Name
		kind = node.CallFunctionCall
	default:
		return node.Call{}, edge.Invokes{}, nil, false
	}
	candidates := bySymbol[method]
	if len(candidates) == 0 {
		return node.Call{}, edge.Invokes{}, nil, false
	}

	urn := node.NewCallURN(repo, kind, caller, ordinal)
	pos := fset.Position(call.Pos())
	endPos := fset.Position(call.End())

	c := node.Call{
		Base: node.Base{
			NodeURN:  urn,
			NodeKind: node.KindCall,
			NodeMeta: node.Meta{
				Version:    1,
				ValidFrom:  now,
				ObservedAt: now,
				Source: node.Source{
					Collector: "code/golang",
					RunID:     emit.RunID,
					Method:    node.MethodDeclared,
				},
				Confidence: 0.7, // resolução por nome — falsos positivos possíveis
			},
		},
		Kind_:        kind,
		CallerURN:    caller,
		Ordinal:      ordinal,
		TargetSymbol: callSymbol(call),
		Location: node.Location{
			File:     relFile,
			LineInit: pos.Line,
			LineEnd:  endPos.Line,
			ColInit:  pos.Column,
			ColEnd:   endPos.Column,
		},
	}

	inv := newInvokes(caller, urn, now, emit)
	var tgt *edge.Targets
	if len(candidates) == 1 {
		c.TargetURN = candidates[0]
		t := newTargets(urn, candidates[0], now, emit)
		tgt = &t
	} else {
		// Múltiplos matches → dispatch ambíguo no MVP.
		c.IsDynamic = true
	}
	return c, inv, tgt, true
}

func newTargets(from, to node.URN, now time.Time, emit EmitOptions) edge.Targets {
	return edge.Targets{
		Base: edge.Base{
			EdgeID:   edge.DeterministicID(from, edge.TypeTargets, to, now),
			EdgeType: edge.TypeTargets,
			FromURN:  from,
			ToURN:    to,
			EdgeMeta: edge.Meta{
				ValidFrom:   now,
				ObservedAt:  now,
				Source:      node.Source{Collector: "code/golang", RunID: emit.RunID, Method: node.MethodDeclared},
				Confidence:  0.7,
				Directional: true,
			},
		},
	}
}

func newUses(from, to node.URN, now time.Time, emit EmitOptions) edge.Uses {
	return edge.Uses{
		Base: edge.Base{
			EdgeID:   edge.DeterministicID(from, edge.TypeUses, to, now),
			EdgeType: edge.TypeUses,
			FromURN:  from,
			ToURN:    to,
			EdgeMeta: edge.Meta{
				ValidFrom:   now,
				ObservedAt:  now,
				Source:      node.Source{Collector: "code/golang", RunID: emit.RunID, Method: node.MethodDeclared},
				Confidence:  0.8, // dependência derivada de pattern; sujeita a falso positivo
				Directional: true,
			},
		},
	}
}
