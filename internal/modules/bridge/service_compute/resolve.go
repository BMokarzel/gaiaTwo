package service_compute

import (
	"regexp"
	"strings"

	"costEngine/internal/entity/node"
)

// Source identifica a estratégia que resolveu o vínculo Service→Compute.
// O valor é persistido no edge RUNS_ON (Meta.Properties["source"]).
type Source string

const (
	SourceTag            Source = "tag"
	SourceNameConvention Source = "name_convention"
	SourceManifest       Source = "manifest" // futuro
)

// Confidence policy por source. ADR-004 fixa os valores; mantenha em
// sync com a tabela do ADR caso evolua.
const (
	ConfidenceTag            float32 = 1.0
	ConfidenceNameConvention float32 = 0.7
	ConfidenceManifest       float32 = 1.0
)

// Resolution descreve o resultado de uma estratégia: qual Service o
// Compute aponta e com que confiança/origem.
//
// ServiceURN vazia = estratégia não conseguiu resolver (sem sinal).
type Resolution struct {
	ServiceURN node.URN
	Source     Source
	Confidence float32
}

// HasMatch retorna true quando a estratégia produziu um candidato.
func (r Resolution) HasMatch() bool { return r.ServiceURN != "" }

// resolveByTag lê tags do Compute em ordem de precedência:
//
//  1. service.urn (URN absoluta, sem ambiguidade)
//  2. Service (nome, lookup pelo mapa de Services)
//
// Tag absente ou nome desconhecido → Resolution vazia.
func resolveByTag(c node.Compute, byName map[string]node.URN) Resolution {
	if c.Tags == nil {
		return Resolution{}
	}
	if urn, ok := c.Tags["service.urn"]; ok && urn != "" {
		return Resolution{
			ServiceURN: node.URN(urn),
			Source:     SourceTag,
			Confidence: ConfidenceTag,
		}
	}
	if name, ok := c.Tags["Service"]; ok && name != "" {
		if urn, found := byName[name]; found {
			return Resolution{
				ServiceURN: urn,
				Source:     SourceTag,
				Confidence: ConfidenceTag,
			}
		}
	}
	return Resolution{}
}

// nameConventionRegex captura o "miolo" da tag Name após o prefixo
// `svc-` e antes de um sufixo final no formato `-<seg>`. O service em
// si é resolvido contra o mapa byName por longest-match (ver
// resolveByNameConvention) — o regex apenas extrai a janela onde o
// nome do service pode estar.
//
// Restrições:
//   - prefixo literal "svc-".
//   - existe pelo menos um sufixo `-<seg>` após o nome do service,
//     impedindo que `svc-foo` cru vire match acidental.
//
// Casos cobertos por teste em resolve_test.go.
var nameConventionRegex = regexp.MustCompile(
	`^svc-([a-z0-9]+(?:-[a-z0-9]+)*)-[a-z0-9]+$`,
)

// resolveByNameConvention extrai o service a partir da tag Name do
// Compute usando a convenção `svc-<service>-<sufixo>`.
//
// Como o nome do service em si pode conter hífens (`billing-repo`,
// `payments-api`), o matcher tenta **longest-match** contra o mapa
// byName: dado `svc-billing-repo-prod-01`, testa `billing-repo-prod`,
// `billing-repo`, `billing` (do mais longo para o mais curto) e
// devolve o primeiro que existir. Se nenhum match, Resolution vazia —
// proteção contra falso positivo por service deletado.
func resolveByNameConvention(c node.Compute, byName map[string]node.URN) Resolution {
	if c.Tags == nil {
		return Resolution{}
	}
	name := strings.TrimSpace(c.Tags["Name"])
	if name == "" {
		return Resolution{}
	}
	m := nameConventionRegex.FindStringSubmatch(strings.ToLower(name))
	if len(m) < 2 {
		return Resolution{}
	}
	// m[1] = "<service>-<...sufixos>" sem o último segmento. Tenta do
	// prefixo mais longo para o mais curto para casar serviços com
	// hífen no nome (ex.: "billing-repo").
	segs := strings.Split(m[1], "-")
	for i := len(segs); i >= 1; i-- {
		cand := strings.Join(segs[:i], "-")
		if urn, ok := byName[cand]; ok {
			return Resolution{
				ServiceURN: urn,
				Source:     SourceNameConvention,
				Confidence: ConfidenceNameConvention,
			}
		}
	}
	return Resolution{}
}

// resolveOne aplica as estratégias em ordem fixa e devolve a primeira
// que matchear. A ordem é parte do contrato: tag > name_convention.
// Manifesto entra aqui quando F-009 ganhar a estratégia (hoje, no-op).
func resolveOne(c node.Compute, byName map[string]node.URN) Resolution {
	if r := resolveByTag(c, byName); r.HasMatch() {
		return r
	}
	if r := resolveByNameConvention(c, byName); r.HasMatch() {
		return r
	}
	return Resolution{}
}
