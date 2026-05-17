package openapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository/memory"
)

const schemaSpec = `openapi: 3.0.3
info: { title: T, version: 2.0.0 }
paths:
  /charges:
    get:
      operationId: list
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Charge"
components:
  schemas:
    Charge:
      type: object
      required: [id, customer]
      properties:
        id: { type: string }
        amount: { type: integer }
        customer:
          $ref: "#/components/schemas/Customer"
        tags:
          type: array
          items: { type: string }
    Customer:
      type: object
      properties:
        id: { type: string }
        name: { type: string }
    Money:
      type: integer
`

func TestParse_Components(t *testing.T) {
	spec, err := Parse(strings.NewReader(schemaSpec))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := spec.Schemas()
	if len(got) != 3 {
		t.Fatalf("schemas=%d want=3 (%+v)", len(got), got)
	}
	// Ordem alfabética: Charge, Customer, Money.
	if got[0].Name != "Charge" || got[1].Name != "Customer" || got[2].Name != "Money" {
		t.Fatalf("order=%v %v %v", got[0].Name, got[1].Name, got[2].Name)
	}

	// Charge tem 4 fields; `customer` referencia Customer.
	if len(got[0].Fields) != 4 {
		t.Fatalf("Charge fields=%d (%+v)", len(got[0].Fields), got[0].Fields)
	}
	var customer ParsedField
	for _, f := range got[0].Fields {
		if f.Name == "customer" {
			customer = f
		}
	}
	if customer.TypeRef != "$ref:Customer" {
		t.Fatalf("customer typeRef=%q", customer.TypeRef)
	}
	if !customer.Required {
		t.Fatalf("customer should be required")
	}
	// References inclui Customer.
	hasRef := false
	for _, r := range got[0].References {
		if r == "Customer" {
			hasRef = true
		}
	}
	if !hasRef {
		t.Fatalf("Charge.References missing Customer: %v", got[0].References)
	}

	// Field "tags" → array<string>.
	var tags ParsedField
	for _, f := range got[0].Fields {
		if f.Name == "tags" {
			tags = f
		}
	}
	if tags.TypeRef != "array<string>" {
		t.Fatalf("tags typeRef=%q", tags.TypeRef)
	}
	if tags.Required {
		t.Fatalf("tags should not be required")
	}
}

func TestCollect_Schemas(t *testing.T) {
	svc := node.NewServiceURN("acme/pay", ".")
	res, err := CollectFromReader(strings.NewReader(schemaSpec), Config{
		ServiceURN: svc, ObservedAt: time.Unix(1700000000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(res.Schemas) != 3 {
		t.Fatalf("schemas=%d", len(res.Schemas))
	}
	// Cada Schema é openapi-format + linkado ao service.
	for _, s := range res.Schemas {
		if s.Format != node.SchemaOpenAPI {
			t.Fatalf("format=%q", s.Format)
		}
		if s.ServiceURN != svc {
			t.Fatalf("svc=%q", s.ServiceURN)
		}
		if s.Version != "2.0.0" {
			t.Fatalf("ver=%q", s.Version)
		}
	}
	// Charge → Customer IMPORTS edge.
	if len(res.Imports) != 1 {
		t.Fatalf("imports=%d want=1 (%+v)", len(res.Imports), res.Imports)
	}
	chargeURN := node.NewSchemaURN("acme/pay", ".", "Charge")
	customerURN := node.NewSchemaURN("acme/pay", ".", "Customer")
	if res.Imports[0].FromURN != chargeURN || res.Imports[0].ToURN != customerURN {
		t.Fatalf("imports edge=%+v", res.Imports[0])
	}
}

func TestApply_SchemasAndImports(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	svc := node.Service{
		Base: node.Base{
			NodeURN:  node.NewServiceURN("acme/pay", "."),
			NodeKind: node.KindService,
			NodeMeta: node.Meta{Version: 1, ValidFrom: time.Now().UTC(), ObservedAt: time.Now().UTC(), Confidence: 1},
		},
		Repo: "acme/pay", ModulePath: ".", Language: "go",
	}
	if err := repo.Upsert(ctx, svc); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := CollectFromReader(strings.NewReader(schemaSpec), Config{
		ServiceURN: svc.URN(), ObservedAt: time.Unix(1700000000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	w := Writer{Nodes: repo, Edges: repo.AsEdgeRepo()}
	st, err := w.Apply(ctx, res, svc.URN())
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if st.Schemas != 3 {
		t.Fatalf("st.Schemas=%d", st.Schemas)
	}
	if st.Imports != 1 {
		t.Fatalf("st.Imports=%d", st.Imports)
	}
}

func TestCollect_NoComponents(t *testing.T) {
	res, err := CollectFromReader(strings.NewReader(collectorSpec), Config{
		ServiceURN: node.NewServiceURN("r", "."),
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(res.Schemas) != 0 || len(res.Imports) != 0 {
		t.Fatalf("expected no schemas/imports: schemas=%d imports=%d",
			len(res.Schemas), len(res.Imports))
	}
}
