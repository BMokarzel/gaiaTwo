package node

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Person representa uma pessoa do org plane (F-010). É um Node puro
// (não-Resource): não tem account/region/spec de provedor.
//
// PII handling (F-010 D2/D8):
//   - URN usa hash do email — `urn:ce:org:<tenant>:person/<email-hash>`.
//   - O hash é determinístico (SHA-256 do email lower+trim, 16 bytes hex).
//   - EmailHint é a forma exibível com mascaramento (`jo***@domain.com`).
//   - Email plain NUNCA é persistido nem logado.
type Person struct {
	Base
	Tenant    string `json:"tenant"`     // multi-org (ex.: "acme")
	EmailHash string `json:"email_hash"` // 32 hex chars (sha256 truncado)
	EmailHint string `json:"email_hint"` // forma ofuscada para UX
	Name      string `json:"name"`
	Role      string `json:"role,omitempty"`
	SquadURN  URN    `json:"squad_urn,omitempty"`
	ManagerHash string `json:"manager_hash,omitempty"` // EmailHash do manager

	// GithubHandle é o handle público no GitHub (sem `@`), normalizado em
	// lower-case (F-011). Usado para resolver `@user` em CODEOWNERS para
	// a URN da Person. Opcional — quando ausente, owner pessoal não é
	// resolvível e é relatado.
	GithubHandle string `json:"github_handle,omitempty"`
}

// HashEmail produz o hash determinístico para PII (F-010 D2): SHA-256
// do email lower+trim, primeiros 16 bytes em hex (32 chars).
func HashEmail(email string) string {
	norm := strings.ToLower(strings.TrimSpace(email))
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:16])
}

// MaskEmail produz a forma ofuscada `xx***@domain` (F-010 D8). Para
// email curto (<2 chars antes do @), usa "***@domain".
func MaskEmail(email string) string {
	norm := strings.ToLower(strings.TrimSpace(email))
	at := strings.Index(norm, "@")
	if at < 0 {
		return "***"
	}
	local, domain := norm[:at], norm[at+1:]
	if len(local) < 2 {
		return "***@" + domain
	}
	return local[:2] + "***@" + domain
}

// NewPersonURN produz a URN canônica para uma Person.
func NewPersonURN(tenant, emailHash string) URN {
	return NewURN(ProviderOrg, tenant, KindPerson, emailHash)
}

// ContentHash dos campos significativos.
func (p Person) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(p.EmailHash)
	sb.WriteByte('|')
	sb.WriteString(p.Name)
	sb.WriteByte('|')
	sb.WriteString(p.Role)
	sb.WriteByte('|')
	sb.WriteString(string(p.SquadURN))
	sb.WriteByte('|')
	sb.WriteString(p.ManagerHash)
	sb.WriteByte('|')
	sb.WriteString(p.GithubHandle)
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

// NormalizeGithubHandle aceita formas comuns vindas de CODEOWNERS e
// retorna a forma canônica (sem `@`, lower-case, sem espaços). Trata
// `@org/team` devolvendo `""` — handle de team é resolvido por outro
// caminho (lookup por nome de Team).
func NormalizeGithubHandle(s string) string {
	t := strings.ToLower(strings.TrimSpace(s))
	t = strings.TrimPrefix(t, "@")
	if strings.Contains(t, "/") {
		return ""
	}
	return t
}
