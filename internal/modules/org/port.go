// Package org é o módulo do **org plane** (F-010+).
//
// Este arquivo declara a inbound port (org.Service) — a única interface
// que controllers e features cross-plane consomem. A implementação
// concreta vive em `internal/modules/org/service/`. Erros tipados
// (errs.go) e value types (types.go) compõem o contrato; entidades de
// domínio reusam `internal/entity/node` (F-016 D2: entidades são
// structs, não interfaces).
//
// Princípios (F-016 ADR-005):
//   - Uma Service por módulo: a segregação vem dos *métodos*, não de
//     múltiplas interfaces. Operações cross-entidade (que tocam Team,
//     Squad e Person juntas) têm lugar natural aqui.
//   - Service não conhece HTTP: retorna `org.ErrXxx` tipados; o
//     controller delega a renderização a `platform/httpserver`.
//   - Outbound ports são owned pelo *caller* (ex.: `app/search` declara
//     `OrgSearcher`); este módulo apenas expõe métodos suficientes para
//     que o adapter externo satisfaça structural typing.
package org

import (
	"context"

	"costEngine/internal/entity/node"
)

// Service é a inbound port do módulo org. Implementada por
// `modules/org/service.New(...)`.
//
// Convenções:
//   - Todo método aceita context.Context na primeira posição (F-016 D10).
//   - Métodos de read são puros sobre o estado bitemporal vigente em
//     AsOfOptions.AsOf (zero = corrente).
//   - Errors retornados são `org.ErrXxx` tipados ou erros do repositório
//     wrapped via fmt.Errorf — o controller usa errors.As para mapear.
type Service interface {
	// ListTeams devolve teams paginados. Em cada item, SquadCount e
	// PersonCount são derivados — não confiar em ordem além de URN asc.
	ListTeams(ctx context.Context, q ListTeamsQuery) (Page[TeamSummary], error)

	// GetTeam devolve o detalhe de um team, incluindo a lista de URNs
	// de squads dependentes. ErrTeamNotFound se URN não resolve.
	// ErrInvalidURN se o URN existe mas referencia outro Kind.
	GetTeam(ctx context.Context, urn node.URN, opts AsOfOptions) (TeamDetail, error)

	// ListSquadsOfTeam paginado. ErrTeamNotFound se o team não existe.
	ListSquadsOfTeam(ctx context.Context, team node.URN, q ListSquadsQuery) (Page[SquadDetail], error)

	// GetSquad devolve detalhe + count de membros. ErrSquadNotFound /
	// ErrInvalidURN como acima.
	GetSquad(ctx context.Context, urn node.URN, opts AsOfOptions) (SquadDetail, error)

	// ListSquadMembers paginado. ErrSquadNotFound se squad não existe.
	// PersonProfile.TeamURN fica vazio aqui (contexto squad-local).
	ListSquadMembers(ctx context.Context, squad node.URN, q ListMembersQuery) (Page[PersonProfile], error)

	// GetPerson devolve perfil com TeamURN derivado via SquadURN.
	// ErrPersonNotFound / ErrInvalidURN.
	GetPerson(ctx context.Context, urn node.URN, opts AsOfOptions) (PersonProfile, error)

	// ListReports devolve a árvore de subordinados a partir de root,
	// até Depth. ErrPersonNotFound se root não existe. Cycles marcam
	// Truncated=true sem erro.
	ListReports(ctx context.Context, root node.URN, q ListReportsQuery) (ReportsTree, error)

	// Search procura nós org (Team/Squad/Person) que casam com Q.
	// Sem filtro de Kind = todos os Kinds do plano org. ErrInvalidArgument
	// se Q vazia.
	Search(ctx context.Context, q SearchQuery) (SearchResults, error)
}
