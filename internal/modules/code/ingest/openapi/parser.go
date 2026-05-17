package openapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// httpMethods é o conjunto fechado de métodos reconhecidos como
// operações dentro de um path-item OpenAPI 3.x.
var httpMethods = map[string]struct{}{
	"get": {}, "put": {}, "post": {}, "delete": {},
	"patch": {}, "options": {}, "head": {}, "trace": {},
}

// Spec é a fração de um documento OpenAPI 3.0/3.1 que o ingest
// consome. Campos fora desse subset são ignorados (sem `DisallowUnknownFields`).
type Spec struct {
	OpenAPI    string                       `yaml:"openapi" json:"openapi"`
	Info       Info                         `yaml:"info" json:"info"`
	Paths      map[string]map[string]rawOp  `yaml:"paths" json:"paths"`
	Components Components                   `yaml:"components" json:"components"`
}

// Components captura `components.schemas` (F-022). Outros tipos de
// component (parameters, responses, securitySchemes) são ignorados.
type Components struct {
	Schemas map[string]map[string]any `yaml:"schemas" json:"schemas"`
}

// Info expõe apenas a versão da spec — bumpá-la força nova versão de
// todos os Endpoints derivados.
type Info struct {
	Title   string `yaml:"title" json:"title"`
	Version string `yaml:"version" json:"version"`
}

// rawOp captura uma operação preservando os corpos de schema crus para
// que a função de hash trabalhe sobre eles diretamente. Mapas YAML
// passam por `yaml.Node` → `any` durante unmarshal.
type rawOp struct {
	OperationID string                 `yaml:"operationId" json:"operationId"`
	Summary     string                 `yaml:"summary" json:"summary"`
	Tags        []string               `yaml:"tags" json:"tags"`
	RequestBody map[string]any         `yaml:"requestBody" json:"requestBody"`
	Responses   map[string]any         `yaml:"responses" json:"responses"`
}

// Operation é a forma normalizada que o collector consome.
type Operation struct {
	Method      string
	Path        string
	OperationID string
	Summary     string
	Tags        []string

	// RequestSchemaHash e ResponseSchemaHash sintetizam o corpo das
	// requisições/respostas via SHA-256. Hash de mapa vazio = "".
	RequestSchemaHash  string
	ResponseSchemaHash string
}

// Parse decodifica um stream OpenAPI (JSON ou YAML). A detecção é
// best-effort: tenta JSON primeiro (estritamente), cai para YAML.
//
// Validações mínimas:
//   - Campo `openapi` presente e começando com "3.".
//   - Pelo menos um path declarado.
func Parse(r io.Reader) (Spec, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return Spec{}, fmt.Errorf("openapi: read: %w", err)
	}
	if len(raw) == 0 {
		return Spec{}, errors.New("openapi: empty spec")
	}

	var spec Spec
	// Heurística simples: se o primeiro byte significativo é '{' ou '[',
	// tentamos JSON antes de YAML. JSON também é YAML válido, mas o
	// decoder JSON falha mais rápido com erros de schema.
	if firstByte(raw) == '{' {
		if err := json.Unmarshal(raw, &spec); err != nil {
			return Spec{}, fmt.Errorf("openapi: json decode: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(raw, &spec); err != nil {
			return Spec{}, fmt.Errorf("openapi: yaml decode: %w", err)
		}
	}

	if !strings.HasPrefix(spec.OpenAPI, "3.") {
		return Spec{}, fmt.Errorf("openapi: unsupported version %q (only 3.x)", spec.OpenAPI)
	}
	if len(spec.Paths) == 0 {
		return Spec{}, errors.New("openapi: no paths declared")
	}
	return spec, nil
}

// ParsedSchema é a forma normalizada de um item em
// `components.schemas`. Captura apenas o que F-022 precisa para um
// `node.Schema` + arestas `IMPORTS`.
type ParsedSchema struct {
	Name string

	// Type cru ("object", "array", "string"…). Em OpenAPI 3.1 pode ser
	// um array; aqui pegamos o primeiro elemento se for o caso.
	JSONType string

	// Ref preenchido se o schema é um envelope `{ $ref: "..." }`.
	Ref string

	// Fields populado quando JSONType=="object" (a partir de
	// `properties`). Para outros tipos fica vazio — o ContentHash do
	// node.Schema ainda detecta drift via campo agregado.
	Fields []ParsedField

	// RawHash é o sha256 do payload completo (canonical). Usado em
	// `Version` do node.Schema para invalidar quando algo fora dos
	// fields conhecidos mudou (e.g. `oneOf`, `allOf`, validações).
	RawHash string

	// References lista nomes de outros components.schemas referenciados
	// direta ou indiretamente (via $ref em properties/items). Usado
	// para emitir arestas IMPORTS.
	References []string
}

// ParsedField representa uma propriedade dentro de um schema "object".
type ParsedField struct {
	Name     string
	TypeRef  string // tipo primitivo, "$ref:Foo", "array<Foo>", ou "object" se inline
	Required bool
}

// Schemas linealiza `components.schemas` em ordem determinística.
func (s Spec) Schemas() []ParsedSchema {
	if len(s.Components.Schemas) == 0 {
		return nil
	}
	names := make([]string, 0, len(s.Components.Schemas))
	for n := range s.Components.Schemas {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]ParsedSchema, 0, len(names))
	for _, name := range names {
		out = append(out, parseSchema(name, s.Components.Schemas[name]))
	}
	return out
}

// parseSchema extrai forma + fields + referências de um schema cru.
func parseSchema(name string, raw map[string]any) ParsedSchema {
	ps := ParsedSchema{Name: name, RawHash: hashAny(raw)}

	if ref, _ := raw["$ref"].(string); ref != "" {
		ps.Ref = ref
		ps.References = append(ps.References, refName(ref))
		return ps
	}

	ps.JSONType = jsonType(raw["type"])

	required := stringSet(raw["required"])
	if props, ok := raw["properties"].(map[string]any); ok {
		fnames := make([]string, 0, len(props))
		for k := range props {
			fnames = append(fnames, k)
		}
		sort.Strings(fnames)
		for _, fn := range fnames {
			sub, _ := props[fn].(map[string]any)
			tref, refs := typeRefOf(sub)
			ps.Fields = append(ps.Fields, ParsedField{
				Name: fn, TypeRef: tref, Required: required[fn],
			})
			ps.References = append(ps.References, refs...)
		}
	}

	// Itens diretos (caso de `type: array` no top-level): também referencia.
	if items, ok := raw["items"].(map[string]any); ok {
		_, refs := typeRefOf(items)
		ps.References = append(ps.References, refs...)
	}
	// allOf/oneOf/anyOf: caça refs recursivamente.
	for _, key := range []string{"allOf", "oneOf", "anyOf"} {
		if arr, ok := raw[key].([]any); ok {
			for _, e := range arr {
				if em, ok := e.(map[string]any); ok {
					_, refs := typeRefOf(em)
					ps.References = append(ps.References, refs...)
				}
			}
		}
	}

	ps.References = dedupe(ps.References)
	return ps
}

// typeRefOf devolve a representação compacta do tipo de uma sub-schema
// (campo, item, etc.) e a lista de nomes referenciados via $ref.
func typeRefOf(sub map[string]any) (string, []string) {
	if sub == nil {
		return "", nil
	}
	if ref, _ := sub["$ref"].(string); ref != "" {
		return "$ref:" + refName(ref), []string{refName(ref)}
	}
	t := jsonType(sub["type"])
	switch t {
	case "array":
		items, _ := sub["items"].(map[string]any)
		inner, refs := typeRefOf(items)
		return "array<" + inner + ">", refs
	case "object":
		// Inline object — sem nome.
		return "object", nil
	default:
		return t, nil
	}
}

// jsonType lida com a divergência 3.0 vs 3.1 (3.1 aceita `type` como
// array). Em ambos os casos, devolvemos uma string primária.
func jsonType(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok && s != "null" {
				return s
			}
		}
	}
	return ""
}

// refName extrai o nome local de `#/components/schemas/Foo` → "Foo".
// Refs externas (URLs) ou sem nome local retornam o ref bruto.
func refName(ref string) string {
	const prefix = "#/components/schemas/"
	if strings.HasPrefix(ref, prefix) {
		return strings.TrimPrefix(ref, prefix)
	}
	return ref
}

// stringSet converte um []any (vindo de YAML) num set de strings.
func stringSet(v any) map[string]bool {
	out := map[string]bool{}
	if arr, ok := v.([]any); ok {
		for _, e := range arr {
			if s, ok := e.(string); ok {
				out[s] = true
			}
		}
	}
	return out
}

// dedupe preserva ordem e remove repetições.
func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// Operations linealiza o spec em uma lista determinística de operações,
// ordenada por (path, method). Métodos não-HTTP são ignorados — chaves
// como `summary`, `parameters` no path-item são pulados silenciosamente.
func (s Spec) Operations() []Operation {
	paths := make([]string, 0, len(s.Paths))
	for p := range s.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var out []Operation
	for _, p := range paths {
		item := s.Paths[p]
		methods := make([]string, 0, len(item))
		for m := range item {
			if _, ok := httpMethods[strings.ToLower(m)]; ok {
				methods = append(methods, m)
			}
		}
		sort.Strings(methods)
		for _, m := range methods {
			op := item[m]
			out = append(out, Operation{
				Method:             strings.ToUpper(m),
				Path:               p,
				OperationID:        op.OperationID,
				Summary:            op.Summary,
				Tags:               append([]string(nil), op.Tags...),
				RequestSchemaHash:  hashAny(op.RequestBody),
				ResponseSchemaHash: hashAny(op.Responses),
			})
		}
	}
	return out
}

// hashAny serializa um valor arbitrário (mapa, slice) em JSON canônico
// (chaves ordenadas) e retorna SHA-256 hex. Vazio → "".
func hashAny(v map[string]any) string {
	if len(v) == 0 {
		return ""
	}
	canon := canonical(v)
	b, err := json.Marshal(canon)
	if err != nil {
		// Fallback determinístico: marca o erro com tag para nunca colidir
		// com hash legítimo.
		return "err:" + err.Error()
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// canonical produz uma representação determinística (mapas → slices
// ordenados de pares) para hashing. Necessário porque `map[string]any`
// não tem ordem garantida na serialização.
func canonical(v any) any {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([][2]any, 0, len(keys))
		for _, k := range keys {
			pairs = append(pairs, [2]any{k, canonical(x[k])})
		}
		return pairs
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = canonical(e)
		}
		return out
	default:
		return x
	}
}

// firstByte devolve o primeiro byte não-whitespace de um buffer; 0 se
// só whitespace.
func firstByte(b []byte) byte {
	for _, c := range b {
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return c
		}
	}
	return 0
}
