package golang

import (
	"path/filepath"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

func TestExtractCalls_HttpAndDataAccess(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex/repo\n")
	mustWrite(t, filepath.Join(root, "userservice", "svc.go"), `package userservice

import (
	"context"
	"database/sql"
	"net/http"
)

type Svc struct {
	DB     *sql.DB
	Client *http.Client
}

func (s *Svc) Create(ctx context.Context) error {
	_, _ = http.Get("https://api.example.com/v1/users")
	_, _ = s.DB.QueryContext(ctx, "SELECT * FROM users")
	return nil
}
`)

	cfg := Config{Repo: "ex", RunID: "run-1", ObservedAt: time.Unix(1700000000, 0).UTC()}
	res, err := Collect(root, cfg)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(res.Calls) != 2 {
		t.Fatalf("Calls=%d want 2: %+v", len(res.Calls), res.Calls)
	}

	var httpCall, dbCall *node.Call
	for i := range res.Calls {
		switch res.Calls[i].Kind_ {
		case node.CallHttpCall:
			httpCall = &res.Calls[i]
		case node.CallDataAccess:
			dbCall = &res.Calls[i]
		}
	}
	if httpCall == nil || dbCall == nil {
		t.Fatalf("kinds não bateram: %+v", res.Calls)
	}

	// HttpCall: URL literal capturada.
	if httpCall.TargetURL != "https://api.example.com/v1/users" {
		t.Errorf("TargetURL=%q", httpCall.TargetURL)
	}
	if httpCall.TargetMethod != "GET" {
		t.Errorf("TargetMethod=%q want GET", httpCall.TargetMethod)
	}

	// DataAccess: operation_kind.
	if dbCall.OperationKind != "select" {
		t.Errorf("OperationKind=%q want select", dbCall.OperationKind)
	}

	// CallerURN aponta para `(*Svc).Create`.
	if !contains(string(httpCall.CallerURN), "(*Svc).Create") {
		t.Errorf("CallerURN inesperado: %q", httpCall.CallerURN)
	}

	// Ordinals únicos dentro do mesmo caller.
	if httpCall.Ordinal == dbCall.Ordinal {
		t.Errorf("Ordinals colidem: %d", httpCall.Ordinal)
	}

	// Edges presentes e válidos.
	if len(res.Invokes) != 2 || len(res.Uses) != 2 {
		t.Fatalf("Invokes=%d Uses=%d want 2 cada", len(res.Invokes), len(res.Uses))
	}
	for _, inv := range res.Invokes {
		if err := edge.Validate(inv, node.KindFunction, node.KindCall); err != nil {
			t.Errorf("Validate INVOKES: %v", err)
		}
	}
	for _, u := range res.Uses {
		if err := edge.Validate(u, node.KindCall, node.KindFramework); err != nil {
			t.Errorf("Validate USES: %v", err)
		}
	}
}

func TestExtractCalls_Idempotent(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex\n")
	mustWrite(t, filepath.Join(root, "service", "s.go"), `package service

import "net/http"

func Do() {
	_, _ = http.Get("https://x.com")
}
`)
	cfg := Config{Repo: "ex"}
	a, err := Collect(root, cfg)
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := Collect(root, cfg)
	if err != nil {
		t.Fatalf("b: %v", err)
	}
	if len(a.Calls) != len(b.Calls) {
		t.Fatalf("len diverge")
	}
	if a.Calls[0].URN() != b.Calls[0].URN() {
		t.Errorf("URN diverge: %q vs %q", a.Calls[0].URN(), b.Calls[0].URN())
	}
}

func TestExtractCalls_IgnoresUnrelatedFunctions(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex\n")
	// `repo` não é pacote relevante (não bate handler/service/usecase),
	// então a Function não é indexada → call deve ser ignorado.
	mustWrite(t, filepath.Join(root, "repo", "r.go"), `package repo

import "net/http"

func Touch() {
	_, _ = http.Get("https://x.com")
}
`)
	res, err := Collect(root, Config{Repo: "ex"})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(res.Calls) != 0 {
		t.Errorf("esperado 0 calls (caller fora do filtro); got %d", len(res.Calls))
	}
}

func TestExtractCalls_InProcessMethodCall(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex/repo\n")
	// usecase package: define o método; service package: chama.
	mustWrite(t, filepath.Join(root, "usecase", "uc.go"), `package usecase

type UC struct{}

func (u *UC) Process() error { return nil }
`)
	mustWrite(t, filepath.Join(root, "service", "s.go"), `package service

import "ex/repo/usecase"

type Svc struct {
	UC *usecase.UC
}

func (s *Svc) Handle() error {
	return s.UC.Process()
}
`)
	res, err := Collect(root, Config{Repo: "ex"})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var mc *node.Call
	for i := range res.Calls {
		if res.Calls[i].Kind_ == node.CallMethodCall {
			mc = &res.Calls[i]
		}
	}
	if mc == nil {
		t.Fatalf("MethodCall não emitido; res.Calls=%+v", res.Calls)
	}

	// Como há um único `Process` indexado → TargetURN deve estar preenchido.
	if mc.TargetURN == "" {
		t.Errorf("TargetURN vazio (esperado resolução única): %+v", mc)
	}
	if mc.IsDynamic {
		t.Errorf("IsDynamic=true mas há match único")
	}

	// TARGETS edge presente.
	if len(res.Targets) != 1 {
		t.Errorf("Targets=%d want 1", len(res.Targets))
	}
	if len(res.Targets) == 1 {
		if err := edge.Validate(res.Targets[0], node.KindCall, node.KindFunction); err != nil {
			t.Errorf("Validate TARGETS: %v", err)
		}
	}
}

func TestExtractCalls_InProcessAmbiguousIsDynamic(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex/repo\n")
	// Dois `Run` em packages distintos → resolução ambígua.
	mustWrite(t, filepath.Join(root, "usecase", "a.go"), `package usecase

type A struct{}

func (a *A) Run() error { return nil }
`)
	mustWrite(t, filepath.Join(root, "usecase", "b.go"), `package usecase

type B struct{}

func (b *B) Run() error { return nil }
`)
	mustWrite(t, filepath.Join(root, "service", "s.go"), `package service

type Svc struct {
	X interface{ Run() error }
}

func (s *Svc) Handle() error {
	return s.X.Run()
}
`)
	res, err := Collect(root, Config{Repo: "ex"})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	var mc *node.Call
	for i := range res.Calls {
		if res.Calls[i].Kind_ == node.CallMethodCall {
			mc = &res.Calls[i]
		}
	}
	if mc == nil {
		t.Fatalf("MethodCall não emitido")
	}
	if mc.TargetURN != "" {
		t.Errorf("TargetURN não vazio em call ambíguo: %q", mc.TargetURN)
	}
	if !mc.IsDynamic {
		t.Errorf("IsDynamic deveria ser true")
	}
}

// F-020: chamada direta `Helper()` (sem qualifier) → FunctionCall.
func TestExtractCalls_InProcessFunctionCall(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex/repo\n")
	// Helper e caller no mesmo package — `Helper()` é ident direto.
	mustWrite(t, filepath.Join(root, "service", "helper.go"), `package service

func Helper() error { return nil }
`)
	mustWrite(t, filepath.Join(root, "service", "s.go"), `package service

type Svc struct{}

func (s *Svc) Handle() error {
	return Helper()
}
`)
	res, err := Collect(root, Config{Repo: "ex"})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var fc *node.Call
	for i := range res.Calls {
		if res.Calls[i].Kind_ == node.CallFunctionCall {
			fc = &res.Calls[i]
		}
	}
	if fc == nil {
		t.Fatalf("FunctionCall não emitido; res.Calls=%+v", res.Calls)
	}
	if fc.TargetSymbol != "Helper" {
		t.Errorf("TargetSymbol=%q want Helper", fc.TargetSymbol)
	}
	if fc.TargetURN == "" {
		t.Errorf("TargetURN vazio (esperado match único): %+v", fc)
	}
	if fc.IsDynamic {
		t.Errorf("IsDynamic=true em call resolvida")
	}
	// TARGETS edge presente para o Helper.
	hasTargets := false
	for _, tgt := range res.Targets {
		if tgt.FromURN == fc.URN() {
			hasTargets = true
			if err := edge.Validate(tgt, node.KindCall, node.KindFunction); err != nil {
				t.Errorf("Validate TARGETS: %v", err)
			}
		}
	}
	if !hasTargets {
		t.Errorf("TARGETS edge ausente para FunctionCall")
	}
}

// helper local: substring contains.
func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
