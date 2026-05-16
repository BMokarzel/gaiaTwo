package org

import (
	"costEngine/internal/entity/node"
	"costEngine/internal/repository"
)

// Page é a página genérica de resultados paginados. NextOffset = 0
// indica "fim da lista" (convenção do cursor opaco em platform/httpserver).
//
// Limit é eco do request — útil em respostas JSON. Items é nil em
// listas vazias para permitir distinguir "página vazia" de "não havia
// resultado".
type Page[T any] struct {
	Items      []T
	Limit      int
	NextOffset int
}

// AsOfOptions agrupa as opções bitemporais reusadas em todos os reads
// do módulo. Centraliza para evitar 7 campos idênticos espalhados
// pelos queries.
type AsOfOptions struct {
	AsOf            repository.AsOf
	IncludeInactive bool
}

// ListTeamsQuery parametriza ListTeams.
type ListTeamsQuery struct {
	AsOfOptions
	Limit  int
	Offset int
}

// ListSquadsQuery parametriza ListSquadsOfTeam.
type ListSquadsQuery struct {
	AsOfOptions
	Limit  int
	Offset int
}

// ListMembersQuery parametriza ListSquadMembers.
type ListMembersQuery struct {
	AsOfOptions
	Limit  int
	Offset int
}

// ListReportsQuery parametriza ListReports. Depth ≥ 1 e ≤ cap do módulo
// (controller valida; service trata Depth ≤ 0 como 0 — árvore vazia).
type ListReportsQuery struct {
	AsOfOptions
	Depth int
}

// SearchQuery parametriza Search (módulo só ou via app/search outbound port).
type SearchQuery struct {
	Q      string
	Kind   node.Kind
	Limit  int
	Offset int
}

// TeamSummary é a projeção de Team usada em listagens.
//
// `Squads` ausente (nil) em listas; preenchido apenas no detail
// (TeamDetail). Counts são derivados via traversal — o service
// encapsula esse custo para que o controller seja dumb.
type TeamSummary struct {
	Team        node.Team
	SquadCount  int
	PersonCount int
}

// TeamDetail estende TeamSummary com a lista de URNs de squads do team.
type TeamDetail struct {
	TeamSummary
	Squads []node.URN
}

// SquadDetail é a projeção de Squad com count de membros.
type SquadDetail struct {
	Squad       node.Squad
	MemberCount int
}

// PersonProfile é a projeção de Person com o TeamURN derivado via
// Squad.TeamURN (F-010 modela como propriedade, evita traversal).
//
// Em endpoints de lista (members de squad), TeamURN fica vazio — o
// caller já está navegando dentro de um squad e a transitividade não
// agrega.
type PersonProfile struct {
	Person  node.Person
	TeamURN node.URN
}

// ReportsTree é a árvore de subordinados de uma pessoa. Truncated
// indica que algum ramo foi cortado por ciclo detectado.
type ReportsTree struct {
	Root      node.Person
	Reports   []ReportNode
	Depth     int
	Truncated bool
}

// ReportNode é um nó da árvore de reports.
type ReportNode struct {
	Person  node.Person
	Reports []ReportNode
}

// SearchResults é o retorno paginado de Search. NextOffset = 0 → fim.
type SearchResults struct {
	Items      []node.Node
	Limit      int
	NextOffset int
}
