package node

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Service representa um módulo de código (raiz com `go.mod` no MVP de
// F-007). É um nó do **code plane** — não é Resource (não tem
// account/region/spec do provedor). Só implementa Node.
//
// URN: urn:ce:code:<repo>:service/<module-path>
//
// Onde `<module-path>` é o caminho relativo do `go.mod` dentro do repo
// (`.` se for a raiz, ou `cmd/cli`, `internal/api`, etc. para módulos
// aninhados em mono-repo).
type Service struct {
	Base
	Repo       string            `json:"repo"`        // canonical repo name
	ModulePath string            `json:"module_path"` // "." ou caminho do submódulo
	Language   string            `json:"language"`    // "go" no MVP
	GoModule   string            `json:"go_module"`   // valor da diretiva `module` em go.mod
	Tags       map[string]string `json:"tags,omitempty"`
}

// NewServiceURN produz a URN canônica para um Service.
func NewServiceURN(repo, modulePath string) URN {
	if modulePath == "" {
		modulePath = "."
	}
	return NewURN(ProviderCode, repo, KindService, modulePath)
}

// ContentHash dos campos significativos — exclui Meta. Usado pelo
// coletor para decidir Upsert (nova versão) vs Touch (mesmo conteúdo).
func (s Service) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(s.Repo)
	sb.WriteByte('|')
	sb.WriteString(s.ModulePath)
	sb.WriteByte('|')
	sb.WriteString(s.Language)
	sb.WriteByte('|')
	sb.WriteString(s.GoModule)
	sb.WriteByte('|')

	keys := make([]string, 0, len(s.Tags))
	for k := range s.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(s.Tags[k])
		sb.WriteByte(',')
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
