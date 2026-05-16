package rules

import (
	"fmt"
	"regexp"
	"time"
)

// RuleType enumera os tipos de regra suportados no MVP (F-006).
//
// Cada tipo tem seu próprio formato de `Target` — validado em Load.
type RuleType string

const (
	// TypeProportional rateia o valor `Match`-ado proporcionalmente
	// ao já alocado em `fct_cost_by_urn` pela dimensão alvo. Ex.: AWS
	// Support rateado por `team` na proporção do que cada team já
	// recebeu como direct.
	TypeProportional RuleType = "proportional_to_allocated"

	// TypeByDestination atribui a recursos cuja região é a região
	// destino da linha unallocated. Ex.: data transfer inter-region.
	TypeByDestination RuleType = "by_destination"

	// TypeStaticOverride distribui percentuais fixos para URNs
	// explícitas. Ex.: "rateio manual de contrato anual entre 3 times".
	TypeStaticOverride RuleType = "static_override"
)

// Match seleciona quais linhas de `fct_unallocated_cost` a regra
// captura. Todos os filtros são AND; campos vazios não filtram.
//
// `ResourceIDEmpty=true` matcha apenas linhas sem resource_id (caso
// mais comum — taxa/suporte/egress agregado).
type Match struct {
	Service         string            `yaml:"service,omitempty"`
	AccountID       string            `yaml:"account_id,omitempty"`
	Reasons         []string          `yaml:"reasons,omitempty"` // unallocated reason
	ResourceIDEmpty bool              `yaml:"resource_id_empty,omitempty"`
	Tags            map[string]string `yaml:"tags,omitempty"` // futuros — não implementado MVP
}

// Target descreve como distribuir o valor matched. Significa coisas
// diferentes por `RuleType`:
//
//   - Proportional: `Dimension` = eixo de rateio (service|account|region|team).
//   - ByDestination: `Region` filtra resource pool destino.
//   - StaticOverride: `Shares` = mapa URN → fração (sum ≈ 1.0).
type Target struct {
	Dimension string             `yaml:"dimension,omitempty"`
	Region    string             `yaml:"region,omitempty"`
	Shares    map[string]float64 `yaml:"shares,omitempty"`
}

// Rule é a unidade de configuração. Equivalente conceitual de um
// `SharedCostRule` Kind (decisão de não-promover em ADR-003).
type Rule struct {
	ID          string     `yaml:"id"`
	Version     uint32     `yaml:"version"`
	Type        RuleType   `yaml:"type"`
	Description string     `yaml:"description,omitempty"`
	ValidFrom   *time.Time `yaml:"valid_from,omitempty"`
	ValidTo     *time.Time `yaml:"valid_to,omitempty"`
	Match       Match      `yaml:"match"`
	Target      Target     `yaml:"target"`
}

// CoversPeriod retorna true se a regra é aplicável ao mês `period`.
// `period` é sempre o primeiro dia UTC do mês.
func (r Rule) CoversPeriod(period time.Time) bool {
	if r.ValidFrom != nil && period.Before(*r.ValidFrom) {
		return false
	}
	if r.ValidTo != nil && !period.Before(*r.ValidTo) {
		// valid_to é exclusivo: período == valid_to já é "depois".
		return false
	}
	return true
}

// idPattern é o slug aceito para `id` — lowercase, kebab-case.
// Renomear quebra audit trail em fct_cost_by_urn → restritivo de propósito.
var idPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Validate aplica validação estática. Erros são fatais no load
// (relatórios FinOps não toleram regra mal-formada passando silente).
func (r Rule) Validate() error {
	if !idPattern.MatchString(r.ID) {
		return fmt.Errorf("rule id %q: must match %s", r.ID, idPattern)
	}
	if r.Version == 0 {
		return fmt.Errorf("rule %q: version must be ≥ 1", r.ID)
	}
	switch r.Type {
	case TypeProportional:
		switch r.Target.Dimension {
		case "service", "account", "region", "team":
			// ok
		case "":
			return fmt.Errorf("rule %q (%s): target.dimension required", r.ID, r.Type)
		default:
			return fmt.Errorf("rule %q (%s): target.dimension %q not supported (service|account|region|team)",
				r.ID, r.Type, r.Target.Dimension)
		}
	case TypeByDestination:
		if r.Target.Region == "" {
			return fmt.Errorf("rule %q (%s): target.region required", r.ID, r.Type)
		}
	case TypeStaticOverride:
		if len(r.Target.Shares) == 0 {
			return fmt.Errorf("rule %q (%s): target.shares required (URN → fraction)", r.ID, r.Type)
		}
		var sum float64
		for urn, frac := range r.Target.Shares {
			if frac <= 0 || frac > 1 {
				return fmt.Errorf("rule %q: shares[%s]=%v out of (0,1]", r.ID, urn, frac)
			}
			sum += frac
		}
		// Tolera 1e-6 de drift de ponto-flutuante na soma das frações.
		if sum < 1-1e-6 || sum > 1+1e-6 {
			return fmt.Errorf("rule %q: shares sum to %v, want ≈ 1.0", r.ID, sum)
		}
	default:
		return fmt.Errorf("rule %q: unknown type %q", r.ID, r.Type)
	}
	if r.ValidFrom != nil && r.ValidTo != nil && !r.ValidFrom.Before(*r.ValidTo) {
		return fmt.Errorf("rule %q: valid_from %v must be < valid_to %v",
			r.ID, *r.ValidFrom, *r.ValidTo)
	}
	return nil
}
