package errs

// HTTPProblem é a interface que erros de domínio implementam para
// controlar como viram resposta HTTP. Compatível com errors.Is/As
// idiomático Go — pode aparecer em qualquer ponto da cadeia de wrap.
//
// Mínimo obrigatório: HTTPStatus + Code. Para enriquecer body, ver
// Detailer e Titler abaixo.
type HTTPProblem interface {
	error
	// HTTPStatus retorna o status HTTP correspondente (4xx/5xx).
	HTTPStatus() int
	// Code retorna o identificador estável, namespaced
	// (ex.: "org.team.not_found"). Vira p.Code e parte de p.Type.
	Code() string
}

// Detailer é uma interface opcional. Erros que implementam adicionam
// campos top-level ao JSON do Problem (URN, conflicts, etc.).
type Detailer interface {
	// Details devolve campos extras a serem achatados no body.
	// Chaves que colidem com campos fixos do Problem (type, title,
	// status, code, detail, trace_id) são silenciosamente sobrescritas
	// pelos campos fixos — não tente renomear via Details.
	Details() map[string]any
}

// Titler é uma interface opcional para sobrescrever o título humano
// padrão (que vem de http.StatusText do status retornado).
type Titler interface {
	Title() string
}
