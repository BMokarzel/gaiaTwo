package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/code"
	"costEngine/internal/modules/code/collector/golang"
	"costEngine/internal/modules/code/service"
	"costEngine/internal/repository/memory"
)

// buildCodeFixture coleta um pseudo-repo via golang.Collect e aplica no
// memory.Repo com o Writer. Devolve `code.Service` apontando para o
// mesmo repo, pronto para reads.
func buildCodeFixture(t *testing.T) code.Service {
	t.Helper()
	dir := t.TempDir()
	write := func(p, body string) {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/app\n")
	write("userservice/svc.go", `package userservice
type Svc struct{}
func (s *Svc) Create(name string) error { return nil }
func (s *Svc) List() ([]string, error) { return nil, nil }
`)
	write("api/handlers/http.go", `package handlers
import "net/http"
func register() {
	http.HandleFunc("/v1/users", listUsers)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/users/{id}", getUser)
}
func listUsers(w http.ResponseWriter, r *http.Request) {}
func getUser(w http.ResponseWriter, r *http.Request) {}
`)
	res, err := golang.Collect(dir, golang.Config{
		Repo:       "myrepo",
		ObservedAt: time.Unix(1700000000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	repo := memory.New()
	w := service.Writer{Nodes: repo, Edges: repo.AsEdgeRepo()}
	if _, err := w.Apply(context.Background(), res); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return service.New(repo, repo.AsEdgeRepo())
}

func TestReader_ListServices(t *testing.T) {
	svc := buildCodeFixture(t)
	p, err := svc.ListServices(context.Background(), code.ListServicesQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if len(p.Items) == 0 {
		t.Fatal("expected at least one Service in fixture")
	}
	// Counts derivados pelo service.
	var withEndpoints, withFunctions int
	for _, s := range p.Items {
		if s.EndpointCount > 0 {
			withEndpoints++
		}
		if s.FunctionCount > 0 {
			withFunctions++
		}
	}
	if withEndpoints == 0 {
		t.Errorf("no Service has EndpointCount > 0 (counts not derived)")
	}
	if withFunctions == 0 {
		t.Errorf("no Service has FunctionCount > 0 (counts not derived)")
	}
}

func TestReader_GetService_NotFound_TypedError(t *testing.T) {
	svc := buildCodeFixture(t)
	_, err := svc.GetService(
		context.Background(),
		node.NewServiceURN("nope", "."),
		code.AsOfOptions{},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	var typed *code.ErrServiceNotFound
	if !errors.As(err, &typed) {
		t.Fatalf("expected *code.ErrServiceNotFound, got %T", err)
	}
	if typed.HTTPStatus() != 404 || typed.Code() != "code.service.not_found" {
		t.Errorf("status/code: %d / %q", typed.HTTPStatus(), typed.Code())
	}
}

func TestReader_GetService_WrongKind_InvalidURN(t *testing.T) {
	svc := buildCodeFixture(t)
	// Listar Endpoints para obter uma URN existente do Kind errado.
	services, _ := svc.ListServices(context.Background(), code.ListServicesQuery{Limit: 10})
	if len(services.Items) == 0 {
		t.Fatal("no services in fixture")
	}
	eps, err := svc.ListEndpointsOfService(
		context.Background(), services.Items[0].Service.URN(),
		code.ListEndpointsQuery{Limit: 10},
	)
	if err != nil || len(eps.Items) == 0 {
		t.Fatalf("ListEndpointsOfService: %v / items=%d", err, len(eps.Items))
	}
	_, err = svc.GetService(context.Background(), eps.Items[0].Endpoint.URN(), code.AsOfOptions{})
	if err == nil {
		t.Fatal("expected error for Endpoint URN passed to GetService")
	}
	var inv *code.ErrInvalidURN
	if !errors.As(err, &inv) {
		t.Fatalf("expected *code.ErrInvalidURN, got %T", err)
	}
}

func TestReader_ListEndpointsOfService_FiltersByServiceURN(t *testing.T) {
	svc := buildCodeFixture(t)
	services, _ := svc.ListServices(context.Background(), code.ListServicesQuery{Limit: 10})
	// Encontrar o Service que tem endpoints (raiz, com api/handlers/).
	var withEPs node.URN
	for _, s := range services.Items {
		if s.EndpointCount > 0 {
			withEPs = s.Service.URN()
			break
		}
	}
	if withEPs == "" {
		t.Fatal("no Service has endpoints in fixture")
	}
	p, err := svc.ListEndpointsOfService(context.Background(), withEPs, code.ListEndpointsQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListEndpointsOfService: %v", err)
	}
	for _, ep := range p.Items {
		if ep.Endpoint.ServiceURN != withEPs {
			t.Errorf("endpoint %s leaked from other service (ServiceURN=%s)",
				ep.Endpoint.URN(), ep.Endpoint.ServiceURN)
		}
	}
}

func TestReader_ListFunctionsOfService_FiltersByServiceURN(t *testing.T) {
	svc := buildCodeFixture(t)
	services, _ := svc.ListServices(context.Background(), code.ListServicesQuery{Limit: 10})
	var withFns node.URN
	for _, s := range services.Items {
		if s.FunctionCount > 0 {
			withFns = s.Service.URN()
			break
		}
	}
	if withFns == "" {
		t.Fatal("no Service has functions in fixture")
	}
	p, err := svc.ListFunctionsOfService(context.Background(), withFns, code.ListFunctionsQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListFunctionsOfService: %v", err)
	}
	for _, fn := range p.Items {
		if fn.Function.ServiceURN != withFns {
			t.Errorf("function %s leaked (ServiceURN=%s)",
				fn.Function.URN(), fn.Function.ServiceURN)
		}
	}
}

func TestReader_GetEndpoint_WrongKind(t *testing.T) {
	svc := buildCodeFixture(t)
	services, _ := svc.ListServices(context.Background(), code.ListServicesQuery{Limit: 10})
	if len(services.Items) == 0 {
		t.Fatal("no services")
	}
	_, err := svc.GetEndpoint(context.Background(), services.Items[0].Service.URN(), code.AsOfOptions{})
	var inv *code.ErrInvalidURN
	if !errors.As(err, &inv) {
		t.Fatalf("expected *code.ErrInvalidURN, got %T", err)
	}
}

func TestReader_GetFunction_NotFound(t *testing.T) {
	svc := buildCodeFixture(t)
	_, err := svc.GetFunction(
		context.Background(),
		node.URN("urn:ce:code:function:myrepo:./pkg.NoSuchFn"),
		code.AsOfOptions{},
	)
	var typed *code.ErrFunctionNotFound
	if !errors.As(err, &typed) {
		t.Fatalf("expected *code.ErrFunctionNotFound, got %T (err=%v)", err, err)
	}
	if typed.HTTPStatus() != 404 {
		t.Errorf("status=%d", typed.HTTPStatus())
	}
}

func TestReader_Search_EmptyQ_TypedError(t *testing.T) {
	svc := buildCodeFixture(t)
	_, err := svc.Search(context.Background(), code.SearchQuery{Q: "  ", Limit: 10})
	if err == nil {
		t.Fatal("expected error")
	}
	var inv *code.ErrInvalidURN
	if !errors.As(err, &inv) {
		t.Fatalf("expected *code.ErrInvalidURN, got %T", err)
	}
}

func TestReader_Search_FiltersToCodePlane(t *testing.T) {
	svc := buildCodeFixture(t)
	// Q amplo que casa nomes de pacote/símbolo do fixture.
	res, err := svc.Search(context.Background(), code.SearchQuery{Q: "user", Limit: 50})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, n := range res.Items {
		switch n.Kind() {
		case node.KindService, node.KindEndpoint, node.KindFunction:
			// ok
		default:
			t.Errorf("Search returned non-code-plane Kind=%s", n.Kind())
		}
	}
}

func TestReader_ListServicesOfService_ServiceMissing_TypedError(t *testing.T) {
	svc := buildCodeFixture(t)
	_, err := svc.ListEndpointsOfService(
		context.Background(),
		node.NewServiceURN("nope", "."),
		code.ListEndpointsQuery{Limit: 10},
	)
	var typed *code.ErrServiceNotFound
	if !errors.As(err, &typed) {
		t.Fatalf("expected *code.ErrServiceNotFound, got %T", err)
	}
}
