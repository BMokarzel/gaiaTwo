---
id: F-031
title: Web — visualizador de arquitetura em 3 níveis (services → service → endpoint steps)
status: done
modules: [web]
depends_on: [F-014, F-030]
modeling_impact: no
adrs: []
epic: E-006
updated: 2026-05-20
---

# F-031 — Web architecture viewer

## Problema

A web app costEngine (`web/`) hoje expõe duas listas planas
(`/services`, `/endpoints`) e uma view de detalhe de endpoint
(`/endpoints/:urn`) com layout Dagre. Não há um nível intermediário
"service" — o operador precisa pular direto de lista global para
endpoint, perdendo o contexto da arquitetura do serviço e sem
conseguir explorar "a partir deste serviço, com o que ele conversa".

A view de endpoint também é estática (Dagre fixo, sem interação) e os
nós são caixas uniformes; não há diferenciação visual entre um service
e um type, nem feedback de quais nós estão conectados.

**Para quem:** arquiteto/engenheiro sênior usando o produto Arquitetura
para inspecionar como um repo TS está estruturado e quais frameworks
externos cada endpoint exercita.

**Dor sem ela:** view chapada que não convida exploração; impossível
compartilhar URL de uma sub-view; não dá pra arrastar nós para
clarificar leitura.

## Escopo

**Inclui:**

- **3 níveis de navegação URL-driven** (React Router):
  - `/services` — lista (já existe)
  - `/services/:repo/architecture` — arquitetura do sistema com root no
    service (depth ∞ no MVP)
  - `/services/:repo/endpoints` — lista de endpoints **deste service**
    (substitui a lista global `/endpoints`)
  - `/services/:repo/endpoints/:urn/*` — steps do endpoint (rename de
    `EndpointDetailPage`)
- **Shapes por kind**:
  - círculo: `service`, `endpoint`
  - losango: `flow_control` (placeholder — coletor TS ainda não emite)
  - retângulo: demais kinds (`module`, `function`, `call`, `type`,
    `variable`, `framework`)
- **Canvas movível com containers**:
  - posições iniciais via Dagre, depois nós livremente arrastáveis
  - containers via `parentId` do React Flow: arquitetura usa `service`
    como container raiz e `module` como sub-container; steps usa
    `module` como container e `function`/`call` dentro
  - containers se expandem automaticamente (`expandParent: true`)
- **Sem zoom/pan**: desabilita `panOnDrag`, `panOnScroll`, `zoomOn*`,
  remove `<Controls/>` e `<MiniMap/>`. Wrapper CSS `overflow: scroll`
  dá navegação espacial via scrollbar nativa.
- **Highlight on hover**: hover em nó destaca vizinhos (in/out edges +
  nós conectados); resto fica dim (opacity 0.25); edges incidentes
  ganham cor de destaque.
- **Detail panel**: click em nó abre painel lateral fixo (direita) com
  `urn`, `kind`, campos de `data` (renderização por kind),
  `valid_from`, `confidence` e `location.file:line` quando houver.
  Sem source snippet (collector não persiste conteúdo).

**NÃO inclui:**

- Modo colapsado/compacto de densidade (mantém uma única densidade =
  "tudo").
- Zoom/pan (decisão explícita).
- Edição/mutação do grafo.
- Visualização de flow control (if/loop/switch) — depende do coletor
  evoluir; shape losango fica disponível mas vazio.
- Source snippet inline (coletor não tem conteúdo).
- Visão global do sistema sem service-root (futuro).

**Precondições:**

- F-014 (REST `/v1/architecture/*`) operacional — usado pelo
  `getFlow`.
- F-030 (TS collector) emitindo `Service / Module / Endpoint /
  Function / Call / Framework` + edges (Contains, DependsOn, Invokes,
  Targets, Uses, DefinedIn).

## Toque no grafo

- **Lê:** todos os Kinds (`service`, `module`, `endpoint`, `function`,
  `call`, `type`, `variable`, `framework`).
- **Escreve:** nada (web é read-only).
- **Novos Kinds/edges:** nenhum.
- **Bitemporal:** consome `valid_from`/`observed_at` por nó; exibe
  apenas o cabeçalho informativo no painel.

## API consumida

- `GET /v1/architecture/nodes?kind=service` — lista services
- `GET /v1/architecture/nodes?kind=endpoint` — lista endpoints
  (filtrada client-side por `service_urn` enquanto backend não tiver
  filtro — verificar)
- `GET /v1/architecture/flow?root=<urn>&depth=N` (formato atual do
  `getFlow`) para arquitetura (root=serviceURN) e steps
  (root=endpointURN)

## Plano de slices (fases)

1. **Fase 1 — Roteamento**
   - `App.tsx`: 5 rotas (lista, redirect repo, architecture, endpoints
     do service, steps).
   - `ServiceArchitecturePage` e `ServiceEndpointsPage` novos.
   - `EndpointDetailPage` → `EndpointStepsPage` (rename, lógica
     preservada).
   - `LeftRail`/`TopBar` apontam pra `/services` (entrada única).
   - Remover `/endpoints` global; helper `buildServiceURN(repo)`.

2. **Fase 2 — Shapes por kind**
   - `web/src/flow/nodes/RoundNode.tsx`, `DiamondNode.tsx`,
     `RectNode.tsx` (substitui `CeNode`).
   - Map `nodeTypes` passado ao `<ReactFlow>`.
   - `flow/layout.ts` seta `type` por kind.

3. **Fase 3 — Canvas movível + containers**
   - Dagre só uma vez no mount (posições iniciais).
   - `parentId` + `extent: 'parent'` + `expandParent: true` para
     containers.
   - Desabilita zoom/pan; remove `Controls`/`MiniMap`.
   - Wrapper `overflow: scroll`; calcula width/height do canvas
     post-layout.

4. **Fase 4 — Hover highlight**
   - Estado local `hoveredId`.
   - `useMemo` para Set de vizinhos.
   - Classes/inline-styles para dim + edge highlight.

5. **Fase 5 — Detail panel**
   - `flow/DetailPanel.tsx` lateral direito.
   - Render por kind dos campos de `data`.
   - Toggle via state local; `Esc` fecha.

## Critérios de aceitação

- Navegar pelas 5 rotas com F5 funciona em qualquer profundidade.
- Service e endpoint nodes aparecem como círculos; demais como
  retângulos; losango disponível (sem dado por enquanto).
- Arrastar um nó dentro de um container expande o container se
  necessário; sair do container não é permitido.
- Scroll do mouse não dá zoom; drag no canvas não dá pan. Scroll move
  o viewport CSS.
- Hover em nó destaca vizinhança; click abre painel lateral.

## Gaps conhecidos

- Coletor TS não emite `flow_control` — losango fica sem população.
- `getFlow` com `depth=∞` num grafo maior pode ficar pesado; revisar
  paginação/lazy depois das primeiras observações.
- Backend pode não filtrar `nodes?kind=endpoint&service_urn=...`;
  fallback client-side por substring na URN no MVP.
