package golang

import (
	"path/filepath"
	"reflect"
	"testing"

	"costEngine/internal/entity/node"
)

// TestFeatureTags_PropagateAcrossNodes confirma F-028: o coletor lê
// `// @feature: ...` no doc da declaração e populates `FeatureTags`
// em Function, Type, Endpoint e Call (herdado do caller).
func TestFeatureTags_PropagateAcrossNodes(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module ex/repo\n")
	mustWrite(t, filepath.Join(root, "userservice", "svc.go"), `package userservice

import (
	"context"
	"net/http"
)

// @feature: checkout, pix
type Order struct {
	ID string
}

// HandleCreate registra rota.
// @feature: checkout
func HandleCreate(mux *http.ServeMux) {
	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {})
}

// Create executa boundary call.
// @feature(pix)
func Create(ctx context.Context) error {
	_, _ = http.Get("https://api.example.com/charge")
	return nil
}
`)
	res, err := Collect(root, Config{Repo: "ex"})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// Function tags.
	var createFn *node.Function
	for i := range res.Functions {
		if res.Functions[i].Symbol == "Create" {
			createFn = &res.Functions[i]
		}
	}
	if createFn == nil {
		t.Fatalf("Create function não emitida")
	}
	if !reflect.DeepEqual(createFn.FeatureTags, []string{"pix"}) {
		t.Errorf("Create.FeatureTags=%v want [pix]", createFn.FeatureTags)
	}

	// Type tags (sorted: checkout,pix).
	var orderType *node.Type
	for i := range res.Types {
		if res.Types[i].Symbol == "Order" {
			orderType = &res.Types[i]
		}
	}
	if orderType == nil {
		t.Fatalf("Order type não emitido")
	}
	want := []string{"checkout", "pix"}
	if !reflect.DeepEqual(orderType.FeatureTags, want) {
		t.Errorf("Order.FeatureTags=%v want %v", orderType.FeatureTags, want)
	}

	// Endpoint tags (herda do HandleCreate).
	if len(res.Endpoints) == 0 {
		t.Fatalf("nenhum endpoint emitido")
	}
	ep := res.Endpoints[0]
	if !reflect.DeepEqual(ep.FeatureTags, []string{"checkout"}) {
		t.Errorf("Endpoint.FeatureTags=%v want [checkout]", ep.FeatureTags)
	}

	// Call tags (herda do Create → [pix]).
	if len(res.Calls) == 0 {
		t.Fatalf("nenhum call emitido")
	}
	if !reflect.DeepEqual(res.Calls[0].FeatureTags, []string{"pix"}) {
		t.Errorf("Call.FeatureTags=%v want [pix]", res.Calls[0].FeatureTags)
	}
}
