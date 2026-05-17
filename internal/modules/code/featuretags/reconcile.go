// Package featuretags faz a sanity-check de `feature_tags` denormalizadas
// (ADR-009 / F-028) contra o catálogo de Features cadastradas no plane
// `gov`.
//
// Política: tag órfã (não bate com nenhum `Feature.ShortID`) é um
// WARNING — não erro fatal. A coleta de código tem que sobreviver à
// drift entre catálogo e código; o relatório serve ao operador para
// decidir entre (a) cadastrar a Feature, (b) renomear a tag, ou
// (c) remover a tag.
//
// Não há decisão automática de qual ação tomar. O ingest segue.
package featuretags

import (
	"sort"

	"costEngine/internal/entity/node"
)

// Catalog é o conjunto de `Feature.ShortID` válidos por tenant. Lookup
// é O(1) — preencha uma vez por tenant e reutilize entre ingests.
type Catalog map[string]struct{}

// BuildCatalog converte um snapshot de Features em Catalog.
// Idempotente, ignora ShortID vazio.
func BuildCatalog(features []node.Feature) Catalog {
	c := Catalog{}
	for _, f := range features {
		if f.ShortID == "" {
			continue
		}
		c[f.ShortID] = struct{}{}
	}
	return c
}

// Has retorna true se a tag existe no catálogo.
func (c Catalog) Has(tag string) bool {
	_, ok := c[tag]
	return ok
}

// Tagged é a forma comum dos nós que carregam `FeatureTags`. Adapter
// para evitar reflect — chame `From*` helpers para converter slices
// concretas (Functions, Endpoints, etc.).
type Tagged struct {
	URN  node.URN
	Kind node.Kind
	Tags []string
}

// Orphan é uma ocorrência de tag sem Feature correspondente. O caller
// agrupa por tag se quiser dedupar warnings.
type Orphan struct {
	NodeURN  node.URN
	NodeKind node.Kind
	Tag      string
}

// Report agrega o resultado de uma reconciliação.
type Report struct {
	Orphans      []Orphan
	UniqueTags   []string // todas as tags encontradas (ordenadas)
	UniqueOrphan []string // subset de UniqueTags que NÃO está no catálogo
}

// Reconcile percorre `tagged` e separa as tags em conhecidas vs órfãs.
// Idempotente; ordem de Orphans segue a ordem de `tagged` × tag.
func Reconcile(tagged []Tagged, cat Catalog) Report {
	var rep Report
	tagSeen := map[string]struct{}{}
	orphanSeen := map[string]struct{}{}
	for _, t := range tagged {
		for _, tag := range t.Tags {
			if tag == "" {
				continue
			}
			tagSeen[tag] = struct{}{}
			if !cat.Has(tag) {
				rep.Orphans = append(rep.Orphans, Orphan{
					NodeURN:  t.URN,
					NodeKind: t.Kind,
					Tag:      tag,
				})
				orphanSeen[tag] = struct{}{}
			}
		}
	}
	rep.UniqueTags = sortedKeys(tagSeen)
	rep.UniqueOrphan = sortedKeys(orphanSeen)
	return rep
}

func sortedKeys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// --- Adapters por kind: evitam que cada caller refaça boilerplate ----

func FromServices(xs []node.Service) []Tagged {
	out := make([]Tagged, 0, len(xs))
	for _, x := range xs {
		out = append(out, Tagged{URN: x.NodeURN, Kind: node.KindService, Tags: x.FeatureTags})
	}
	return out
}

func FromModules(xs []node.Module) []Tagged {
	out := make([]Tagged, 0, len(xs))
	for _, x := range xs {
		out = append(out, Tagged{URN: x.NodeURN, Kind: node.KindModule, Tags: x.FeatureTags})
	}
	return out
}

func FromEndpoints(xs []node.Endpoint) []Tagged {
	out := make([]Tagged, 0, len(xs))
	for _, x := range xs {
		out = append(out, Tagged{URN: x.NodeURN, Kind: node.KindEndpoint, Tags: x.FeatureTags})
	}
	return out
}

func FromFunctions(xs []node.Function) []Tagged {
	out := make([]Tagged, 0, len(xs))
	for _, x := range xs {
		out = append(out, Tagged{URN: x.NodeURN, Kind: node.KindFunction, Tags: x.FeatureTags})
	}
	return out
}

func FromTypes(xs []node.Type) []Tagged {
	out := make([]Tagged, 0, len(xs))
	for _, x := range xs {
		out = append(out, Tagged{URN: x.NodeURN, Kind: node.KindType, Tags: x.FeatureTags})
	}
	return out
}

func FromVariables(xs []node.Variable) []Tagged {
	out := make([]Tagged, 0, len(xs))
	for _, x := range xs {
		out = append(out, Tagged{URN: x.NodeURN, Kind: node.KindVariable, Tags: x.FeatureTags})
	}
	return out
}

func FromCalls(xs []node.Call) []Tagged {
	out := make([]Tagged, 0, len(xs))
	for _, x := range xs {
		out = append(out, Tagged{URN: x.NodeURN, Kind: node.KindCall, Tags: x.FeatureTags})
	}
	return out
}

func FromSchemas(xs []node.Schema) []Tagged {
	out := make([]Tagged, 0, len(xs))
	for _, x := range xs {
		out = append(out, Tagged{URN: x.NodeURN, Kind: node.KindSchema, Tags: x.FeatureTags})
	}
	return out
}
