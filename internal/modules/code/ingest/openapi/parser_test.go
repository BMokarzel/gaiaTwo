package openapi

import (
	"strings"
	"testing"
)

const yamlSpec = `openapi: 3.0.3
info:
  title: Payments
  version: 1.2.3
paths:
  /v1/charges:
    get:
      operationId: listCharges
      summary: List charges
      tags: [billing]
      responses:
        "200":
          description: ok
    post:
      operationId: createCharge
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                amount: { type: integer }
      responses:
        "201":
          description: created
  /v1/charges/{id}:
    get:
      operationId: getCharge
      responses:
        "200": { description: ok }
        "404": { description: nope }
`

const jsonSpec = `{
  "openapi": "3.1.0",
  "info": { "title": "T", "version": "0.0.1" },
  "paths": {
    "/ping": {
      "get": { "operationId": "ping", "responses": { "200": { "description": "pong" } } }
    }
  }
}`

func TestParse_YAML(t *testing.T) {
	spec, err := Parse(strings.NewReader(yamlSpec))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if spec.Info.Version != "1.2.3" {
		t.Fatalf("version=%q", spec.Info.Version)
	}
	ops := spec.Operations()
	if len(ops) != 3 {
		t.Fatalf("ops=%d want=3", len(ops))
	}
	// Determinismo (path asc, method asc): GET /v1/charges, POST /v1/charges, GET /v1/charges/{id}.
	if ops[0].Method != "GET" || ops[0].Path != "/v1/charges" {
		t.Fatalf("ops[0]=%+v", ops[0])
	}
	if ops[1].Method != "POST" || ops[1].Path != "/v1/charges" {
		t.Fatalf("ops[1]=%+v", ops[1])
	}
	if ops[1].RequestSchemaHash == "" {
		t.Fatalf("expected request hash")
	}
	if ops[0].RequestSchemaHash != "" {
		t.Fatalf("get sem body deveria ter hash vazio")
	}
	if ops[2].ResponseSchemaHash == "" {
		t.Fatalf("get/id sem responses?")
	}
}

func TestParse_JSON(t *testing.T) {
	spec, err := Parse(strings.NewReader(jsonSpec))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if spec.OpenAPI != "3.1.0" {
		t.Fatalf("openapi=%q", spec.OpenAPI)
	}
	ops := spec.Operations()
	if len(ops) != 1 || ops[0].OperationID != "ping" {
		t.Fatalf("ops=%+v", ops)
	}
}

func TestParse_RejectsSwagger2(t *testing.T) {
	_, err := Parse(strings.NewReader("swagger: \"2.0\"\npaths:\n  /x: {get: {}}\n"))
	if err == nil {
		t.Fatal("expected error for missing openapi 3.x")
	}
}

func TestParse_RejectsEmpty(t *testing.T) {
	_, err := Parse(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParse_NoPaths(t *testing.T) {
	_, err := Parse(strings.NewReader("openapi: 3.0.0\ninfo: {title: x, version: 1}\n"))
	if err == nil {
		t.Fatal("expected error for no paths")
	}
}

func TestHash_DeterministicAcrossRuns(t *testing.T) {
	body := map[string]any{
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{"type": "object"},
			},
		},
	}
	h1 := hashAny(body)
	h2 := hashAny(body)
	if h1 != h2 || h1 == "" {
		t.Fatalf("h1=%q h2=%q", h1, h2)
	}
	// Ordem das chaves não muda o hash.
	body2 := map[string]any{
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{"type": "object"},
			},
		},
	}
	if hashAny(body2) != h1 {
		t.Fatalf("hash mudou com mesma estrutura")
	}
}
