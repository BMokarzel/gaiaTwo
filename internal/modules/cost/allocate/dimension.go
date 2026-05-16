package allocate

import (
	"costEngine/internal/entity/node"
)

// DimensionResolver projeta um (bucket → URN) em zero ou mais valores
// para uma única Dimension. É plugin-style para que dimensões futuras
// (`team`, `capability`, `environment`) entrem sem mexer no agregador.
//
// Contrato:
//   - Retornar "" significa "essa dimensão não se aplica a este bucket"
//     (ex.: DimensionTeam quando o URN não está mapeado em HRIS) — o
//     allocator dropa silenciosamente (não vai para unallocated).
//   - O resolver é stateless e thread-safe. Receber o URN permite a
//     dimensões que precisam de info do grafo (team, capability) fazer
//     lookup; as do MVP só usam a key.
type DimensionResolver interface {
	Dimension() Dimension
	Resolve(key resourceKey, urn node.URN) string
}

// serviceResolver tira o `Service` direto do bucket. Equivale ao
// `lineItem/ProductCode` na origem (CUR).
type serviceResolver struct{}

func (serviceResolver) Dimension() Dimension                  { return DimensionService }
func (serviceResolver) Resolve(k resourceKey, _ node.URN) string { return k.Service }

// accountResolver usa o `AccountID` do bucket. Em F-010 (HRIS) será
// substituído por um resolver que mapeia account → team, mas a
// dimensão DimensionAccount permanece — multi-tenancy interna ainda
// vai querer rollup por conta.
type accountResolver struct{}

func (accountResolver) Dimension() Dimension                  { return DimensionAccount }
func (accountResolver) Resolve(k resourceKey, _ node.URN) string { return k.AccountID }

// regionResolver tira o `Region` direto do bucket.
type regionResolver struct{}

func (regionResolver) Dimension() Dimension                  { return DimensionRegion }
func (regionResolver) Resolve(k resourceKey, _ node.URN) string { return k.Region }

// DefaultResolvers retorna os 3 resolvers do MVP. Futuro: aceitar config
// para incluir team/capability quando HRIS estiver online.
func DefaultResolvers() []DimensionResolver {
	return []DimensionResolver{
		serviceResolver{},
		accountResolver{},
		regionResolver{},
	}
}

// ResolversFor filtra DefaultResolvers pelo set requested. Ordem do
// resultado segue a ordem de `requested` — facilita testes
// determinísticos.
func ResolversFor(requested []Dimension) []DimensionResolver {
	defs := DefaultResolvers()
	idx := make(map[Dimension]DimensionResolver, len(defs))
	for _, d := range defs {
		idx[d.Dimension()] = d
	}
	out := make([]DimensionResolver, 0, len(requested))
	for _, d := range requested {
		if r, ok := idx[d]; ok {
			out = append(out, r)
		}
	}
	return out
}
