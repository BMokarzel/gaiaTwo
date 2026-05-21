package typescript

import (
	"encoding/json"
	"time"
)

// SchemaVersion é checada no `init` event. Bump major rompe; minor
// apenas adiciona campos opcionais.
const SchemaVersion = "v1"

// ConfigEnvelope é o JSON enviado no stdin do sidecar (uma linha).
//
// O sidecar não negocia: usa o que vier ou aborta com `error` event.
type ConfigEnvelope struct {
	Schema     string   `json:"$schema"`
	Root       string   `json:"root"`
	Repo       string   `json:"repo"`
	RunID      string   `json:"run_id"`
	ObservedAt string   `json:"observed_at"` // RFC3339 UTC
	SkipDirs   []string `json:"skip_dirs,omitempty"`
	Verbose    bool     `json:"verbose,omitempty"`
}

// EventKind enumera os tipos de eventos que o sidecar pode emitir.
type EventKind string

const (
	EventInit      EventKind = "init"
	EventService   EventKind = "service"
	EventModule    EventKind = "module"
	EventEndpoint  EventKind = "endpoint"
	EventFunction  EventKind = "function"
	EventCall      EventKind = "call"
	EventType      EventKind = "type"
	EventVariable  EventKind = "variable"
	EventFramework EventKind = "framework"
	EventEdge      EventKind = "edge"
	EventProgress  EventKind = "progress"
	EventDone      EventKind = "done"
	EventError     EventKind = "error"
)

// Envelope é a forma comum de todos os eventos NDJSON. O payload é
// decodificado em segundo passo, conforme `Kind`.
type Envelope struct {
	Schema  string          `json:"$schema"`
	Kind    EventKind       `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

// InitPayload é o primeiro evento — handshake de versão.
type InitPayload struct {
	SidecarVersion string `json:"sidecar_version"`
	NodeVersion    string `json:"node_version"`
	TsMorphVersion string `json:"ts_morph_version,omitempty"`
}

// ServicePayload — 1 por `package.json` encontrado.
type ServicePayload struct {
	Slug         string            `json:"slug"`        // name do package.json (sanitizado)
	ModulePath   string            `json:"module_path"` // rel ao Root; "." se raiz
	Namespace    string            `json:"namespace"`   // = `name` no package.json
	Manifest     string            `json:"manifest"`    // ex.: "package.json" ou "packages/api/package.json"
	ManifestType string            `json:"manifest_type"` // "package.json"
	Language     string            `json:"language"`    // "typescript"
	Tags         map[string]string `json:"tags,omitempty"`
	FeatureTags  []string          `json:"feature_tags,omitempty"`
}

// ModulePayload — 1 por diretório com .ts files dentro de um Service.
type ModulePayload struct {
	ServiceModulePath string `json:"service_module_path"`
	Namespace         string `json:"namespace"` // dir relativo ao Service root, slash-separated
	Path              string `json:"path"`      // dir relativo ao Repo root
}

// EndpointPayload — handler HTTP detectado.
type EndpointPayload struct {
	ServiceModulePath string            `json:"service_module_path"`
	ModuleNamespace   string            `json:"module_namespace"`
	Method            string            `json:"method"`    // GET, POST, ...
	Path              string            `json:"path"`      // "/users/:id" (já com prefix do router/controller)
	Framework         string            `json:"framework"` // "express" | "nestjs"
	HandlerSymbol     string            `json:"handler_symbol,omitempty"`
	Location          SourceLocation    `json:"location"`
	Tags              map[string]string `json:"tags,omitempty"`
}

// FunctionPayload — function/method/arrow exportada ou referenciada
// por endpoint.
type FunctionPayload struct {
	ServiceModulePath string         `json:"service_module_path"`
	ModuleNamespace   string         `json:"module_namespace"`
	Namespace         string         `json:"namespace"` // ex.: "src/users.UsersService"
	Symbol            string         `json:"symbol"`    // ex.: "findById"
	Receiver          string         `json:"receiver,omitempty"`
	Exported          bool           `json:"exported"`
	SignatureHash     string         `json:"signature_hash"`
	Signature         string         `json:"signature"`
	Location          SourceLocation `json:"location"`
}

// CallPayload — um call site detectado dentro de uma Function.
// O Targets é emitido como `edge` separado (Call→<resolvido>).
type CallPayload struct {
	ServiceModulePath string         `json:"service_module_path"`
	FromFunctionURN   string         `json:"from_function_urn"`
	Subkind           string         `json:"subkind"` // "http!" | "db!" | "mq!" | "in-process" | "unresolved!"
	CalleeExpression  string         `json:"callee_expression"`
	TargetHint        string         `json:"target_hint,omitempty"`     // URL pattern, table name, etc.
	FrameworkName     string         `json:"framework_name,omitempty"` // npm package quando classificado (axios, fetch, typeorm, ...)
	Location          SourceLocation `json:"location"`
}

// TypePayload — interface/type/class/enum (F-021).
type TypePayload struct {
	ServiceModulePath string         `json:"service_module_path"`
	ModuleNamespace   string         `json:"module_namespace"`
	Namespace         string         `json:"namespace"`
	Symbol            string         `json:"symbol"`
	Kind              string         `json:"kind"` // "interface" | "type" | "class" | "enum"
	Exported          bool           `json:"exported"`
	Location          SourceLocation `json:"location"`
}

// VariablePayload — const/let no escopo de módulo.
type VariablePayload struct {
	ServiceModulePath string         `json:"service_module_path"`
	ModuleNamespace   string         `json:"module_namespace"`
	Symbol            string         `json:"symbol"`
	TypeText          string         `json:"type_text,omitempty"`
	Exported          bool           `json:"exported"`
	Location          SourceLocation `json:"location"`
}

// FrameworkPayload — dep declarada no package.json.
type FrameworkPayload struct {
	Ecosystem     string `json:"ecosystem"` // "npm"
	Name          string `json:"name"`
	LatestVersion string `json:"latest_version"`
	IsDevOnly     bool   `json:"is_dev_only"`
}

// EdgePayload — edge cross-entidade que precisa ser materializado.
// Ex.: Function→Call (Invokes), Call→Framework (Uses), Type→Type
// (Extends/Aliases). Edges Service→Module/Module→Module são derivados
// no Go-side a partir dos eventos `service`/`module`.
type EdgePayload struct {
	Type     string  `json:"type"`     // "Invokes" | "Targets" | "Uses" | "Extends" | "Aliases"
	FromURN  string  `json:"from_urn"` // pode usar placeholders resolvidos no Go-side
	ToURN    string  `json:"to_urn"`
	Subkind  string  `json:"subkind,omitempty"`
	Strength float64 `json:"strength,omitempty"` // Invokes Strength
}

// ProgressPayload — pulse opcional para UX/logs.
type ProgressPayload struct {
	Stage   string `json:"stage"`
	Done    int    `json:"done"`
	Total   int    `json:"total,omitempty"`
	Message string `json:"message,omitempty"`
}

// ErrorPayload — sidecar abortou. Não tem `done` depois.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Where   string `json:"where,omitempty"`
}

// DonePayload — fim normal; sidecar fecha stdout em seguida.
type DonePayload struct {
	Services  int           `json:"services"`
	Modules   int           `json:"modules"`
	Endpoints int           `json:"endpoints"`
	Functions int           `json:"functions"`
	Calls     int           `json:"calls"`
	Types     int           `json:"types"`
	Variables int           `json:"variables"`
	Elapsed   time.Duration `json:"elapsed_ns"`
}

// SourceLocation aponta para o arquivo:linha onde o símbolo foi detectado.
type SourceLocation struct {
	File   string `json:"file"`            // rel ao Repo root
	Line   int    `json:"line"`            // 1-based
	Column int    `json:"column,omitempty"`
}
