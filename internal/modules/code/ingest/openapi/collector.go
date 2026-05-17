package openapi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

// CollectorName identifica a fonte das observações emitidas pelo
// ingest OpenAPI no campo `Source.Collector` (e em edges `source`).
const CollectorName = "code/ingest/openapi"

// Framework é o valor literal gravado em `Endpoint.Framework` para
// origens vindas de spec OpenAPI. Diferencia visualmente de Endpoints
// descobertos por AST (`net/http`, `chi`, etc.).
const Framework = "openapi"

// Config controla uma execução de ingest sobre uma spec.
type Config struct {
	// ServiceURN é o nó-pai dos Endpoints; precisa pré-existir no grafo.
	// Sua estrutura informa Endpoint.URN (slot account = "repo" da URN
	// canônica de Endpoint; slot id = serviceModulePath).
	ServiceURN node.URN

	// RunID rastreia a execução do ingest em `Source.RunID`.
	RunID string

	// ObservedAt fixa o instante das observações (default: time.Now UTC).
	ObservedAt time.Time
}

// Result agrupa o que o ingest produz para uma spec inteira.
type Result struct {
	Endpoints []node.Endpoint
	Edges     []edge.DefinedIn

	// Schemas e Imports vêm de `components.schemas` (F-022). Cada item
	// em `components.schemas` vira um `node.Schema` Format=openapi;
	// `$ref` entre eles vira uma aresta `Imports`.
	Schemas []node.Schema
	Imports []edge.Imports

	// Spec versionada (extracted from Info.Version) e contagem de
	// operações ignoradas para diagnóstico.
	SpecVersion string
}

// Collect carrega `specPath` (arquivo local) e converte cada operação
// em Endpoint + edge DefinedIn(Endpoint→Service).
func Collect(specPath string, cfg Config) (Result, error) {
	f, err := os.Open(specPath)
	if err != nil {
		return Result{}, fmt.Errorf("openapi: open %s: %w", specPath, err)
	}
	defer f.Close()
	return CollectFromReader(f, cfg)
}

// CollectFromReader é a variante para entradas in-memory / streams; usado
// principalmente em testes.
func CollectFromReader(r io.Reader, cfg Config) (Result, error) {
	if cfg.ServiceURN == "" {
		return Result{}, fmt.Errorf("openapi: ServiceURN is required")
	}
	parts, err := node.ParseURN(cfg.ServiceURN)
	if err != nil {
		return Result{}, fmt.Errorf("openapi: invalid ServiceURN: %w", err)
	}
	if parts.Kind != node.KindService {
		return Result{}, fmt.Errorf("openapi: ServiceURN kind must be %q, got %q",
			node.KindService, parts.Kind)
	}
	if cfg.ObservedAt.IsZero() {
		cfg.ObservedAt = time.Now().UTC()
	}

	spec, err := Parse(r)
	if err != nil {
		return Result{}, err
	}

	ops := spec.Operations()
	res := Result{SpecVersion: spec.Info.Version}

	for _, op := range ops {
		ep := buildEndpoint(parts, spec, op, cfg)
		res.Endpoints = append(res.Endpoints, ep)
		res.Edges = append(res.Edges, newDefinedIn(ep.URN(), cfg.ServiceURN, cfg))
	}

	// F-022: components.schemas → node.Schema + IMPORTS edges.
	schemas := spec.Schemas()
	urnByName := make(map[string]node.URN, len(schemas))
	for _, ps := range schemas {
		s := buildSchema(parts, spec, ps, cfg)
		res.Schemas = append(res.Schemas, s)
		urnByName[ps.Name] = s.URN()
	}
	for _, ps := range schemas {
		fromURN, ok := urnByName[ps.Name]
		if !ok {
			continue
		}
		for _, ref := range ps.References {
			toURN, ok := urnByName[ref]
			if !ok {
				// $ref aponta para schema externo ou desconhecido; ignora.
				// MVP não resolve refs cross-file.
				continue
			}
			res.Imports = append(res.Imports, newImports(fromURN, toURN, cfg))
		}
	}
	return res, nil
}

// buildEndpoint converte uma Operation em `node.Endpoint`. URN é
// determinística em (service, method, path); o handler é o
// `operationId` quando presente, ou um hash de fallback.
func buildEndpoint(svc node.URNParts, spec Spec, op Operation, cfg Config) node.Endpoint {
	handler := op.OperationID
	if handler == "" {
		handler = fallbackHandler(op.Method, op.Path)
	}

	urn := node.NewEndpointURN(svc.Account, svc.ID, op.Method, op.Path)

	props := map[string]any{
		"openapi_operation_id":  op.OperationID,
		"openapi_summary":       op.Summary,
		"openapi_tags":          op.Tags,
		"openapi_spec_version":  spec.Info.Version,
		"request_schema_hash":   op.RequestSchemaHash,
		"response_schema_hash":  op.ResponseSchemaHash,
		"source":                "openapi",
	}

	var request *node.BodyRef
	if op.RequestSchemaHash != "" {
		request = &node.BodyRef{TypeRef: "sha256:" + op.RequestSchemaHash}
	}
	var responses map[string]node.BodyRef
	if op.ResponseSchemaHash != "" {
		responses = map[string]node.BodyRef{
			"default": {TypeRef: "sha256:" + op.ResponseSchemaHash},
		}
	}

	return node.Endpoint{
		Base: node.Base{
			NodeURN:  urn,
			NodeKind: node.KindEndpoint,
			NodeMeta: node.Meta{
				Version:    1,
				ValidFrom:  cfg.ObservedAt,
				ObservedAt: cfg.ObservedAt,
				Source: node.Source{
					Collector: CollectorName,
					RunID:     cfg.RunID,
					Method:    node.MethodDeclared,
				},
				Confidence: 1.0,
				Properties: props,
			},
		},
		ServiceURN: cfg.ServiceURN,
		Method:     op.Method,
		Route:      op.Path,
		Handler:    handler,
		Framework:  Framework,
		Request:    request,
		Responses:  responses,
	}
}

// fallbackHandler produz um identificador estável quando a operação
// não declara `operationId`. Usa um hash curto de (method+path) — não
// quer-se inventar um nome bonito.
func fallbackHandler(method, path string) string {
	sum := sha256.Sum256([]byte(method + " " + path))
	return "openapi:" + hex.EncodeToString(sum[:8])
}

// buildSchema converte um `ParsedSchema` em `node.Schema`. `Version`
// recebe a versão da spec; mudanças em raw (não cobertas por Fields)
// ainda invalidam via `RawHash` em `Properties.raw_hash`.
func buildSchema(svc node.URNParts, spec Spec, ps ParsedSchema, cfg Config) node.Schema {
	fields := make([]node.FieldSlot, 0, len(ps.Fields))
	for i, f := range ps.Fields {
		fields = append(fields, node.FieldSlot{
			Name:     f.Name,
			Position: i,
			TypeRef:  f.TypeRef,
			Optional: !f.Required,
		})
	}
	props := map[string]any{
		"openapi_spec_version": spec.Info.Version,
		"openapi_raw_hash":     ps.RawHash,
		"openapi_json_type":    ps.JSONType,
		"source":               "openapi",
	}
	return node.Schema{
		Base: node.Base{
			NodeURN:  node.NewSchemaURN(svc.Account, svc.ID, ps.Name),
			NodeKind: node.KindSchema,
			NodeMeta: node.Meta{
				Version:    1,
				ValidFrom:  cfg.ObservedAt,
				ObservedAt: cfg.ObservedAt,
				Source: node.Source{
					Collector: CollectorName,
					RunID:     cfg.RunID,
					Method:    node.MethodDeclared,
				},
				Confidence: 1.0,
				Properties: props,
			},
		},
		ServiceURN: cfg.ServiceURN,
		Format:     node.SchemaOpenAPI,
		Symbol:     ps.Name,
		Version:    spec.Info.Version,
		Fields:     fields,
	}
}

func newImports(from, to node.URN, cfg Config) edge.Imports {
	return edge.Imports{
		Base: edge.Base{
			EdgeID:   edge.DeterministicID(from, edge.TypeImports, to, cfg.ObservedAt),
			EdgeType: edge.TypeImports,
			FromURN:  from,
			ToURN:    to,
			EdgeMeta: edge.Meta{
				ValidFrom:   cfg.ObservedAt,
				ObservedAt:  cfg.ObservedAt,
				Source:      node.Source{Collector: CollectorName, RunID: cfg.RunID, Method: node.MethodDeclared},
				Confidence:  1.0,
				Directional: true,
				Properties:  map[string]any{"source": "openapi"},
			},
		},
	}
}

// newDefinedIn cria a edge Endpoint→Service com source="openapi" via
// `Properties` (não há campo dedicado em edge.Meta no MVP).
func newDefinedIn(from, to node.URN, cfg Config) edge.DefinedIn {
	return edge.DefinedIn{
		Base: edge.Base{
			EdgeID:   edge.DeterministicID(from, edge.TypeDefinedIn, to, cfg.ObservedAt),
			EdgeType: edge.TypeDefinedIn,
			FromURN:  from,
			ToURN:    to,
			EdgeMeta: edge.Meta{
				ValidFrom:   cfg.ObservedAt,
				ObservedAt:  cfg.ObservedAt,
				Source:      node.Source{Collector: CollectorName, RunID: cfg.RunID, Method: node.MethodDeclared},
				Confidence:  1.0,
				Directional: true,
				Properties:  map[string]any{"source": "openapi"},
			},
		},
	}
}
