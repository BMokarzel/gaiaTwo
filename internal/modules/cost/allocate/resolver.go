package allocate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/bridge"
	"costEngine/internal/repository"
)

// resourceLookup é a chave canônica (account, resource_id) usada para
// indexar tanto resoluções de URN quanto falhas. É um superset do que
// o `resourceKey` agrega (que inclui region+service também) — vários
// buckets podem compartilhar a mesma URN se o operador rodou o mesmo
// recurso em regiões/usage_types diferentes.
type resourceLookup struct {
	AccountID  string
	ResourceID string
}

// Resolution agrupa o output de `Resolve` num formato consumível pelo
// dimensionador.
type Resolution struct {
	// URNByResource: chave (account, resource_id) → URN canônica.
	URNByResource map[resourceLookup]node.URN

	// Failures: chave (account, resource_id) → motivo (ReasonNotFound ou
	// ReasonAmbiguous). Linhas faltando entram em fct_unallocated_cost.
	Failures map[resourceLookup]UnallocatedReason
}

// AsOfForPeriod calcula o instante de resolução para um BillingPeriod.
// Por F-005 D6 usamos o último instante do mês — pegando o estado do
// grafo "no fim do período" — para que recursos descomissionados
// **dentro** do mês ainda apareçam (eles tinham custo) e recursos
// criados **depois** do fim do mês não vazem.
//
// period precisa ser dia 1 UTC. Retorna last_day 23:59:59.999 UTC.
func AsOfForPeriod(period time.Time) repository.AsOf {
	period = period.UTC()
	first := time.Date(period.Year(), period.Month(), 1, 0, 0, 0, 0, time.UTC)
	nextMonth := first.AddDate(0, 1, 0)
	lastInstant := nextMonth.Add(-time.Millisecond)
	return repository.AsOf(lastInstant)
}

// Resolve executa N batches no Resolver — um por account — e materializa
// uma Resolution. AsOf é derivada de `period` (`AsOfForPeriod`).
//
// Erros fatais (ctx cancel, repo errors não-sentinela) abortam e
// retornam o que foi resolvido até ali + o erro.
func Resolve(
	ctx context.Context,
	res bridge.Resolver,
	provider node.ProviderID,
	idsByAccount map[string][]string,
	period time.Time,
) (Resolution, error) {
	out := Resolution{
		URNByResource: make(map[resourceLookup]node.URN, 1024),
		Failures:      make(map[resourceLookup]UnallocatedReason, 64),
	}
	asOf := AsOfForPeriod(period)

	for account, ids := range idsByAccount {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		urns, failures, err := res.ResolveURNBatch(ctx, provider, account, ids, asOf)
		if err != nil {
			return out, fmt.Errorf("resolve account=%s: %w", account, err)
		}
		for id, urn := range urns {
			out.URNByResource[resourceLookup{AccountID: account, ResourceID: id}] = urn
		}
		for _, f := range failures {
			lk := resourceLookup{AccountID: account, ResourceID: f.ExternalID}
			switch {
			case errors.Is(f.Err, repository.ErrAmbiguous):
				out.Failures[lk] = ReasonAmbiguous
			case errors.Is(f.Err, repository.ErrNotFound):
				out.Failures[lk] = ReasonNotFound
			default:
				// Erro não-sentinela no batch: tratamos como NotFound
				// para não bloquear a alocação, mas registramos.
				out.Failures[lk] = ReasonNotFound
			}
		}
	}
	return out, nil
}
