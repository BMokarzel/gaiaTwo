// Package bridge resolve identificadores nativos de provedor (`resource_id`
// do AWS CUR, por exemplo) para URNs canônicas do grafo.
//
// É o ponto único de entrada do ingestor de custo (F-004) e de qualquer
// allocation engine que precisa amarrar uma linha de custo a um nó do
// grafo. O contrato é estável; a implementação delega a
// `repository.NodeRepository.GetByExternalID`.
//
// Política (F-003):
//
//  1. Tenta match estrito (provider, account, externalID).
//  2. Se ErrNotFound, tenta match wildcard cross-account
//     (provider, externalID). Cobre recursos "globais" (S3, IAM) que o
//     CUR atribui a conta A enquanto o grafo guarda o dono em conta B.
//  3. Se o wildcard matchear múltiplas URNs em contas diferentes,
//     retorna ErrAmbiguous — caller decide (fila unresolved, log).
//
// AsOf opcional permite resolver no estado histórico do grafo
// (útil para reprocessar CUR antigo: ver F-003/S-003).
package bridge

import (
	"context"
	"errors"
	"fmt"

	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// Resolver é o contrato que F-004 (CUR parser) consome.
type Resolver interface {
	// ResolveURN resolve um externalID (ARN ou short ID) para a URN
	// canônica. account != "" → tenta estrito primeiro com fallback
	// cross-account; account == "" → vai direto pelo wildcard.
	ResolveURN(ctx context.Context, p node.ProviderID, account, externalID string, as repository.AsOf) (node.URN, error)

	// ResolveURNBatch resolve N externalIDs sob o mesmo (provider, account, asOf).
	// Retorna mapa parcial (externalID → URN) com os que resolveram, e
	// uma lista de erros não-fatais para os que falharam (ErrNotFound,
	// ErrAmbiguous). Erros fatais (ctx cancelado, etc.) abortam e
	// retornam o que tiver até ali + o erro.
	ResolveURNBatch(ctx context.Context, p node.ProviderID, account string, externalIDs []string, as repository.AsOf) (map[string]node.URN, []ResolveError, error)
}

// ResolveError descreve a falha de um externalID específico no batch.
type ResolveError struct {
	ExternalID string
	Err        error
}

func (e ResolveError) Error() string { return fmt.Sprintf("%s: %v", e.ExternalID, e.Err) }
func (e ResolveError) Unwrap() error { return e.Err }

// repoResolver implementa Resolver sobre um NodeRepository.
type repoResolver struct {
	repo repository.NodeRepository
}

// New cria um Resolver lendo de repo.
func New(repo repository.NodeRepository) Resolver {
	return &repoResolver{repo: repo}
}

// ResolveURN aplica a política estrito → wildcard.
func (b *repoResolver) ResolveURN(ctx context.Context, p node.ProviderID, account, externalID string, as repository.AsOf) (node.URN, error) {
	// Passo 1: match estrito (se account foi fornecido).
	if account != "" {
		urn, err := b.repo.GetByExternalID(ctx, p, account, externalID, as)
		if err == nil {
			return urn, nil
		}
		if !errors.Is(err, repository.ErrNotFound) {
			// Ambiguidade ou erro fatal — não fallback.
			return "", err
		}
		// fallthrough: tenta wildcard.
	}

	// Passo 2: wildcard cross-account.
	urn, err := b.repo.GetByExternalID(ctx, p, "", externalID, as)
	if err != nil {
		return "", err
	}
	return urn, nil
}

// ResolveURNBatch faz N chamadas sequenciais. Otimizações (UNWIND no n4j)
// vivem na impl do repository — bridge mantém a API simples.
func (b *repoResolver) ResolveURNBatch(ctx context.Context, p node.ProviderID, account string, externalIDs []string, as repository.AsOf) (map[string]node.URN, []ResolveError, error) {
	out := make(map[string]node.URN, len(externalIDs))
	var failures []ResolveError

	for _, eid := range externalIDs {
		if err := ctx.Err(); err != nil {
			return out, failures, err
		}
		urn, err := b.ResolveURN(ctx, p, account, eid, as)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) || errors.Is(err, repository.ErrAmbiguous) {
				failures = append(failures, ResolveError{ExternalID: eid, Err: err})
				continue
			}
			// Erro fatal (ex.: contexto): aborta.
			return out, failures, err
		}
		out[eid] = urn
	}
	return out, failures, nil
}
