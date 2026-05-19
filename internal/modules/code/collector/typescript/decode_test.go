package typescript

import (
	"strings"
	"testing"
	"time"

	"costEngine/internal/entity/node"
)

// TestDecodeStream_HappyPath exercita o pipeline NDJSON sem precisar de
// Node real — alimenta o decoder diretamente.
func TestDecodeStream_HappyPath(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	cfg := Config{Repo: "acme", RunID: "run-1", ObservedAt: now}

	stream := strings.Join([]string{
		`{"$schema":"v1","kind":"init","payload":{"sidecar_version":"0.1.0","node_version":"v20"}}`,
		`{"$schema":"v1","kind":"service","payload":{"slug":"api","module_path":".","namespace":"@acme/api","manifest":"package.json","manifest_type":"package.json","language":"typescript"}}`,
		`{"$schema":"v1","kind":"module","payload":{"service_module_path":".","namespace":"src/users","path":"src/users"}}`,
		`{"$schema":"v1","kind":"function","payload":{"service_module_path":".","module_namespace":"src/users","namespace":"src/users","symbol":"findById","exported":true,"signature":"(id:string)=>User","location":{"file":"src/users/users.service.ts","line":12}}}`,
		`{"$schema":"v1","kind":"endpoint","payload":{"service_module_path":".","module_namespace":"src/users","method":"GET","path":"/users/:id","framework":"express","handler_symbol":"findById","location":{"file":"src/users/routes.ts","line":7}}}`,
		`{"$schema":"v1","kind":"type","payload":{"service_module_path":".","module_namespace":"src/users","namespace":"src/users","symbol":"User","kind":"interface","exported":true,"location":{"file":"src/users/types.ts","line":1}}}`,
		`{"$schema":"v1","kind":"variable","payload":{"service_module_path":".","module_namespace":"src/users","symbol":"DEFAULT_LIMIT","type_text":"number","exported":true,"location":{"file":"src/users/const.ts","line":1}}}`,
		`{"$schema":"v1","kind":"framework","payload":{"ecosystem":"npm","name":"express","latest_version":"^4.19.0","is_dev_only":false}}`,
		`{"$schema":"v1","kind":"call","payload":{"service_module_path":".","from_function_urn":"urn:ce:code:acme:function/.!src/users!findById","subkind":"db!","callee_expression":"prisma.user.findUnique","target_hint":"user"}}`,
		`{"$schema":"v1","kind":"edge","payload":{"type":"Contains","from_urn":"urn:ce:code:acme:service/.","to_urn":"urn:ce:code:acme:module/.!src/users"}}`,
		`{"$schema":"v1","kind":"done","payload":{"services":1,"modules":1,"endpoints":1,"functions":1,"calls":1,"types":1,"variables":1}}`,
	}, "\n") + "\n"

	res, err := decodeStream(strings.NewReader(stream), cfg)
	if err != nil {
		t.Fatalf("decodeStream: %v", err)
	}

	if got, want := len(res.Services), 1; got != want {
		t.Errorf("Services=%d want %d", got, want)
	}
	if got, want := len(res.Modules), 1; got != want {
		t.Errorf("Modules=%d want %d", got, want)
	}
	if got, want := len(res.Functions), 1; got != want {
		t.Errorf("Functions=%d want %d", got, want)
	}
	if got, want := len(res.Endpoints), 1; got != want {
		t.Errorf("Endpoints=%d want %d", got, want)
	}
	if got, want := len(res.Types), 1; got != want {
		t.Errorf("Types=%d want %d", got, want)
	}
	if got, want := len(res.Variables), 1; got != want {
		t.Errorf("Variables=%d want %d", got, want)
	}
	if got, want := len(res.Frameworks), 1; got != want {
		t.Errorf("Frameworks=%d want %d", got, want)
	}
	if got, want := len(res.Calls), 1; got != want {
		t.Errorf("Calls=%d want %d", got, want)
	}
	if got, want := len(res.Contains), 1; got != want {
		t.Errorf("Contains=%d want %d", got, want)
	}

	// Service URN deterministica.
	svc := res.Services[0]
	wantURN := node.NewServiceURN("acme", ".")
	if svc.URN() != wantURN {
		t.Errorf("Service URN=%s want %s", svc.URN(), wantURN)
	}
	if svc.Language != "typescript" {
		t.Errorf("Service.Language=%s want typescript", svc.Language)
	}

	// Endpoint URN bate com (repo, ".", "GET", "/users/:id")
	ep := res.Endpoints[0]
	wantEP := node.NewEndpointURN("acme", ".", "GET", "/users/:id")
	if ep.URN() != wantEP {
		t.Errorf("Endpoint URN=%s want %s", ep.URN(), wantEP)
	}
	if ep.Method != "GET" || ep.Route != "/users/:id" || ep.Framework != "express" {
		t.Errorf("endpoint fields wrong: %+v", ep)
	}

	// Call: subkind="db!" mapeia para CallDataAccess
	if got := res.Calls[0].Kind_; got != node.CallDataAccess {
		t.Errorf("Call.Kind=%s want %s", got, node.CallDataAccess)
	}

	// Source carrega tag do collector.
	if got := svc.Meta().Source.Collector; got != collectorTag {
		t.Errorf("Source.Collector=%s want %s", got, collectorTag)
	}
}

func TestDecodeStream_NoInit(t *testing.T) {
	stream := `{"$schema":"v1","kind":"done","payload":{}}` + "\n"
	_, err := decodeStream(strings.NewReader(stream), Config{Repo: "x"})
	if err == nil || !strings.Contains(err.Error(), "init") {
		t.Errorf("expected init-missing error, got %v", err)
	}
}

func TestDecodeStream_NoDone(t *testing.T) {
	stream := `{"$schema":"v1","kind":"init","payload":{}}` + "\n"
	_, err := decodeStream(strings.NewReader(stream), Config{Repo: "x"})
	if err == nil || !strings.Contains(err.Error(), "done") {
		t.Errorf("expected done-missing error, got %v", err)
	}
}

func TestDecodeStream_ErrorEvent(t *testing.T) {
	stream := strings.Join([]string{
		`{"$schema":"v1","kind":"init","payload":{}}`,
		`{"$schema":"v1","kind":"error","payload":{"code":"E_PARSE","message":"failed to load tsconfig","where":"index.ts"}}`,
	}, "\n") + "\n"
	_, err := decodeStream(strings.NewReader(stream), Config{Repo: "x"})
	if err == nil || !strings.Contains(err.Error(), "E_PARSE") {
		t.Errorf("expected sidecar error to surface, got %v", err)
	}
}

func TestDecodeStream_SchemaMismatch(t *testing.T) {
	stream := `{"$schema":"v2","kind":"init","payload":{}}` + "\n"
	_, err := decodeStream(strings.NewReader(stream), Config{Repo: "x"})
	if err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Errorf("expected schema mismatch, got %v", err)
	}
}

func TestDecodeStream_UnknownKindForwardCompat(t *testing.T) {
	// Eventos com kind desconhecido devem ser ignorados, não falhar.
	stream := strings.Join([]string{
		`{"$schema":"v1","kind":"init","payload":{}}`,
		`{"$schema":"v1","kind":"unknown_future_event","payload":{"foo":"bar"}}`,
		`{"$schema":"v1","kind":"done","payload":{}}`,
	}, "\n") + "\n"
	_, err := decodeStream(strings.NewReader(stream), Config{Repo: "x"})
	if err != nil {
		t.Errorf("unknown kind should be ignored, got %v", err)
	}
}

func TestSidecarBundleHash_Stable(t *testing.T) {
	h1 := SidecarBundleHash()
	h2 := SidecarBundleHash()
	if h1 != h2 {
		t.Errorf("hash not stable: %s vs %s", h1, h2)
	}
	if len(h1) != 16 {
		t.Errorf("hash should be 16 hex chars, got %d", len(h1))
	}
}

func TestIsStub(t *testing.T) {
	if !isStub() {
		t.Error("expected embedded bundle to still be the stub (run npm run build to produce real bundle)")
	}
}

func TestCollect_StubBundleFailsFast(t *testing.T) {
	// Sem o bundle real, Collect deve falhar com ErrSidecarStub antes
	// de tentar invocar Node.
	_, err := Collect(testContext(t), ".", Config{Repo: "x"})
	if err == nil {
		t.Fatal("expected error from stub bundle, got nil")
	}
	if !strings.Contains(err.Error(), "stub") {
		t.Errorf("expected stub error, got: %v", err)
	}
}
