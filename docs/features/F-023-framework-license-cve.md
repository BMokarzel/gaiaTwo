---
id: F-023
title: Framework / License / SecurityAdvisory como nós globais
status: entity + coletores go.mod/package.json/requirements.txt done; pyproject/Cargo + License/SPDX + CVE pendentes
modules: [code]
depends_on: [F-022]
modeling_impact: yes
adrs: [ADR-006]
epic: E-007
updated: 2026-05-17
---

# F-023 — Framework, License, SecurityAdvisory

## Problema

Dependências externas (chi, gorm, react) influenciam custo, risco e
licença. Modelar como nó permite query do tipo "quais Services usam
chi v4 e estão afetados por CVE-2024-xxxx?".

## Mudanças

- `node.Framework` (global, tenant-agnóstico): `urn:ce:code:_global:framework/<ecosystem>!<name>`.
- `node.License` (SPDX): `urn:ce:code:_global:license/<spdx>`.
- `node.SecurityAdvisory` (CVE/GHSA): `urn:ce:code:_global:security_advisory/<id>`.
- Arestas: `LICENSED_UNDER` (Framework→License), `AFFECTED_BY`
  (Framework→Advisory), `PATCHED_IN` (Advisory→Framework).
- `DEPENDS_ON` (já existente) extendido para Module/Service→Framework.

## DoD

- [x] 3 kinds + URNs + ContentHash.
- [x] Adjacency LICENSED_UNDER / AFFECTED_BY / PATCHED_IN / DEPENDS_ON
      (Service/Module → Framework) validadas.
- [x] Coletor `go.mod` emite `Framework` + `DEPENDS_ON` Service→Framework;
      `Hard` reflete direct/indirect.
- [x] Coletor `package.json` em `internal/modules/code/collector/npm`:
      lê `dependencies`/`devDependencies`/`peerDependencies`/
      `optionalDependencies`, emite `Framework` (ecosystem=npm) +
      `DEPENDS_ON` Service→Framework. `IsDevOnly` cobre dev/peer/optional.
      Dedup cross-manifest por URN; `node_modules` skipado.
- [x] Coletor `requirements.txt` em `internal/modules/code/collector/python`:
      parseia `pkg[op]ver` (PEP 440 ops `==`/`>=`/`~=`/…), normaliza nome
      PEP 503 (lowercase, `_.` → `-`), descarta extras `[security]`,
      ignora `-r`/`-e`/URLs/`git+`. `DevFiles` configurável marca
      `IsDevOnly`. Ecosystem=pypi.
- [ ] Coletor `pyproject.toml` / `Cargo.toml` — backlog (requer parser TOML).
- [ ] Lookup SPDX para popular `License` + `LICENSED_UNDER` — backlog.
- [ ] Sync OSV / GitHub Advisory — backlog.
