package node

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// CallKind classifica a família do call site (ADR-007, F-019/F-020).
//
// Boundary calls (atravessam fronteira de processo/serviço):
//   - HttpCall, RpcCall, EventPublish, EventSubscribe,
//     QueueSend, QueueReceive, DataAccess, JobSchedule
//
// In-process calls:
//   - FunctionCall, MethodCall
type CallKind string

const (
	CallHttpCall       CallKind = "http"
	CallRpcCall        CallKind = "rpc"
	CallEventPublish   CallKind = "event_publish"
	CallEventSubscribe CallKind = "event_subscribe"
	CallQueueSend      CallKind = "queue_send"
	CallQueueReceive   CallKind = "queue_receive"
	CallDataAccess     CallKind = "data_access"
	CallJobSchedule    CallKind = "job_schedule"
	CallFunctionCall   CallKind = "function"
	CallMethodCall     CallKind = "method"
)

// ArgSlot é o argumento posicional de uma Call (ADR-008). `Source`
// descreve a origem do valor — literal, variável, parâmetro, retorno
// de outra call. Alimenta simulação de fluxo de dados.
//
// Convenção de `Source`:
//   - `literal:<repr>`
//   - `variable:<name>`
//   - `param:<name>`
//   - `call:<call-urn>`
//   - `unknown`
type ArgSlot struct {
	Name     string `json:"name,omitempty"`
	Position int    `json:"position"`
	TypeRef  string `json:"type_ref,omitempty"`
	Source   string `json:"source,omitempty"`
}

// Call representa um site de invocação dentro de uma Function. É a
// família F-019/F-020 (ADR-007) — todo call (boundary ou in-process)
// é um nó de primeira classe.
//
// URN: urn:ce:code:<repo>:call/<kind>!<caller-function-id>#<idx>
//
// `<idx>` é o ordinal AST (0-based, em ordem de aparição) dentro da
// função declarante — estável a refactors que preservam ordem.
type Call struct {
	Base
	Kind_       CallKind `json:"call_kind"` // evita colisão com Node.Kind()
	CallerURN   URN      `json:"caller_urn"` // Function que contém o call
	Ordinal     int      `json:"ordinal"`    // posição no AST do caller
	Location    Location `json:"location,omitempty"`

	// Alvo resolvido (opcional — Call.Kind define a semântica esperada):
	//   - HttpCall: TargetURN pode ser Endpoint quando resolvido;
	//                TargetURL / TargetMethod sempre tentados.
	//   - RpcCall:  TargetURN é Function ou Endpoint do serviço alvo.
	//   - DataAccess: TargetURN é Persistence; OperationKind é select/insert/...
	//   - EventPublish/Subscribe: TargetURN é Messaging; Topic preenche o canal.
	//   - QueueSend/Receive: TargetURN é Messaging.
	//   - JobSchedule: TargetURN é Function (callback agendado).
	//   - FunctionCall: TargetURN é Function alvo (quando resolvível).
	//   - MethodCall: TargetURN é Function alvo; IsDynamic=true quando
	//     dispatch via interface não-resolvível.
	TargetURN     URN    `json:"target_urn,omitempty"`
	TargetSymbol  string `json:"target_symbol,omitempty"` // forma legível ("pkg.Fn", "User.Save")
	TargetURL     string `json:"target_url,omitempty"`
	TargetMethod  string `json:"target_method,omitempty"`
	OperationKind string `json:"operation_kind,omitempty"` // select|insert|update|delete|exec…
	Topic         string `json:"topic,omitempty"`
	IsDynamic     bool   `json:"is_dynamic,omitempty"`     // dispatch dinâmico

	FrameworkURN URN       `json:"framework_urn,omitempty"` // F-023, USES
	Args         []ArgSlot `json:"args,omitempty"`
	Returns      []ArgSlot `json:"returns,omitempty"`

	FeatureTags []string `json:"feature_tags,omitempty"`
}

// NewCallURN produz a URN canônica para uma Call.
func NewCallURN(repo string, kind CallKind, callerURN URN, ordinal int) URN {
	id := string(kind) + "!" + string(callerURN) + "#" + strconv.Itoa(ordinal)
	return NewURN(ProviderCode, repo, KindCall, id)
}

// ContentHash dos campos significativos.
func (c Call) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(string(c.Kind_))
	sb.WriteByte('|')
	sb.WriteString(string(c.CallerURN))
	sb.WriteByte('|')
	sb.WriteString(strconv.Itoa(c.Ordinal))
	sb.WriteByte('|')
	sb.WriteString(string(c.TargetURN))
	sb.WriteByte('|')
	sb.WriteString(c.TargetSymbol)
	sb.WriteByte('|')
	sb.WriteString(c.TargetURL)
	sb.WriteByte('|')
	sb.WriteString(c.TargetMethod)
	sb.WriteByte('|')
	sb.WriteString(c.OperationKind)
	sb.WriteByte('|')
	sb.WriteString(c.Topic)
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatBool(c.IsDynamic))
	sb.WriteByte('|')
	sb.WriteString(string(c.FrameworkURN))
	sb.WriteByte('|')

	for _, a := range c.Args {
		sb.WriteString(strconv.Itoa(a.Position))
		sb.WriteByte(':')
		sb.WriteString(a.Name)
		sb.WriteByte(':')
		sb.WriteString(a.TypeRef)
		sb.WriteByte(':')
		sb.WriteString(a.Source)
		sb.WriteByte(',')
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
