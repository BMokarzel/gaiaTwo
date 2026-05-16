package service_compute

import (
	"sort"
	"strings"

	"costEngine/internal/entity/node"
)

// Apply é a engine pura de F-009: dado Computes + lookup de Services +
// edges correntes, devolve o diff (Opens/Closes/Orphans/Ambiguous).
//
// Determinismo:
//   - Computes são processados em ordem por URN.
//   - Opens e Closes são emitidos na ordem do processamento.
//   - Mesmo Input ⇒ mesmo Output (independente da ordem original do
//     repo). Caller pode rodar Apply 2x e comparar diffs.
//
// Bitemporal:
//   - Match (edge corrente aponta para o mesmo Service): noop.
//   - Mismatch (Service novo): close no edge antigo + open no novo.
//     ValidTo do close == ValidFrom do open == Input.Now.
//   - Sem edge corrente + sinal: open.
//   - Sem edge corrente + sem sinal: orphan (não emite, registra).
//   - Edge corrente + sem sinal: close com Reason="orphaned" + orphan.
//
// Ambiguidade vence sobre tudo: se o Compute aponta para um nome que
// virou ambíguo no lookup, NÃO emitimos open (não dá pra escolher
// determinístico) e NÃO fechamos edge corrente (vazaria info do dono
// real). Operador resolve via tag explícita (service.urn).
func Apply(in Input) Output {
	out := Output{
		Stats: Stats{ComputesScanned: len(in.Computes)},
	}

	// Index edges correntes por Compute URN para lookup O(1).
	current := make(map[node.URN]ExistingEdge, len(in.ExistingEdges))
	for _, e := range in.ExistingEdges {
		current[e.ComputeURN] = e
	}

	// Ordem determinística: por URN.
	computes := make([]node.Compute, len(in.Computes))
	copy(computes, in.Computes)
	sort.Slice(computes, func(i, j int) bool {
		return computes[i].URN() < computes[j].URN()
	})

	for _, c := range computes {
		cur, hasCurrent := current[c.URN()]

		// Detecta ambiguidade antes de resolver: se o Compute aponta
		// por nome (tag Service ou Name=svc-X-...) e esse nome está
		// em AmbiguousNames, marca como ambíguo e segue.
		if ambig, name := detectAmbiguity(c, in.AmbiguousNames); ambig != nil {
			out.Ambiguous = append(out.Ambiguous, Ambiguity{
				ComputeURN: c.URN(),
				Name:       name,
				Candidates: ambig,
			})
			out.Stats.AmbiguousCount++
			// Não fechamos edge corrente em ambíguo (ver doc Apply).
			continue
		}

		r := resolveOne(c, in.ServicesByName)
		switch r.Source {
		case SourceTag:
			out.Stats.ResolvedTag++
		case SourceNameConvention:
			out.Stats.ResolvedName++
		}

		switch {
		case !r.HasMatch() && !hasCurrent:
			// Sem sinal e sem edge — órfão.
			out.Orphans = append(out.Orphans, c.URN())
			out.Stats.NoSignal++

		case !r.HasMatch() && hasCurrent:
			// Edge corrente perdeu sinal: fecha como órfão.
			out.Closes = append(out.Closes, EdgeClose{
				EdgeID:     cur.EdgeID,
				ServiceURN: cur.ServiceURN,
				ComputeURN: cur.ComputeURN,
				ValidTo:    in.Now,
				Reason:     "orphaned",
			})
			out.Orphans = append(out.Orphans, c.URN())
			out.Stats.NoSignal++
			out.Stats.Closes++

		case r.HasMatch() && !hasCurrent:
			// Novo edge.
			out.Opens = append(out.Opens, EdgeOpen{
				ServiceURN: r.ServiceURN,
				ComputeURN: c.URN(),
				ValidFrom:  in.Now,
				Confidence: r.Confidence,
				Source:     r.Source,
			})
			out.Stats.Opens++

		case r.HasMatch() && hasCurrent:
			if cur.ServiceURN == r.ServiceURN &&
				cur.Source == r.Source &&
				cur.Confidence == r.Confidence {
				// Match exato — noop. Mudança só de confidence/source
				// (mesmo Service) também emite re-open: caller pode
				// querer registrar a evolução da evidência.
				continue
			}
			// Owner ou evidência mudou: close antigo + open novo.
			out.Closes = append(out.Closes, EdgeClose{
				EdgeID:     cur.EdgeID,
				ServiceURN: cur.ServiceURN,
				ComputeURN: cur.ComputeURN,
				ValidTo:    in.Now,
				Reason:     "owner_changed",
			})
			out.Opens = append(out.Opens, EdgeOpen{
				ServiceURN: r.ServiceURN,
				ComputeURN: c.URN(),
				ValidFrom:  in.Now,
				Confidence: r.Confidence,
				Source:     r.Source,
			})
			out.Stats.Opens++
			out.Stats.Closes++
		}
	}

	return out
}

// detectAmbiguity inspeciona as fontes potenciais de nome no Compute e
// retorna a lista de candidatos + o nome se ele estiver em
// ambiguousNames. service.urn (URN absoluta) ignora ambiguidade — caso
// de escape consagrado em ADR-004.
//
// Retorna (nil, "") quando não há ambiguidade aplicável.
func detectAmbiguity(c node.Compute, ambiguousNames map[string][]node.URN) ([]node.URN, string) {
	if c.Tags == nil || len(ambiguousNames) == 0 {
		return nil, ""
	}
	// URN absoluta na tag → não passa por nome, não ambíguo.
	if urn := c.Tags["service.urn"]; urn != "" {
		return nil, ""
	}
	if name := c.Tags["Service"]; name != "" {
		if cands, ok := ambiguousNames[name]; ok {
			return cands, name
		}
		// Service nomeado sem ambiguidade declarada → ok.
		return nil, ""
	}
	// Convention: olha sufixo Name=svc-<x>-... e tenta longest-match
	// contra os nomes ambíguos (mesma lógica de resolveByNameConvention).
	if raw := strings.TrimSpace(c.Tags["Name"]); raw != "" {
		m := nameConventionRegex.FindStringSubmatch(strings.ToLower(raw))
		if len(m) >= 2 {
			segs := strings.Split(m[1], "-")
			for i := len(segs); i >= 1; i-- {
				cand := strings.Join(segs[:i], "-")
				if cs, ok := ambiguousNames[cand]; ok {
					return cs, cand
				}
			}
		}
	}
	return nil, ""
}
