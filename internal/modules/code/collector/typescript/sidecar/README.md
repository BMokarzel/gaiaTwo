# `@costengine/typescript-sidecar`

Sidecar Node.js usado pelo coletor TypeScript do CostEngine (F-030).

## Como rodar manualmente

Para depurar fora do binário Go:

```bash
cd internal/modules/code/collector/typescript/sidecar
npm install
npm run build
echo '{"$schema":"v1","root":"/path/to/repo-ts","repo":"my-repo","run_id":"dev-1","observed_at":"2026-05-18T12:00:00Z"}' \
  | node dist/sidecar.bundle.js
```

## Build

```bash
npm install
npm run build   # produz dist/sidecar.bundle.js (CommonJS, minified, single-file)
```

O bundle é embedded no binário Go via `//go:embed` (ver
`internal/modules/code/collector/typescript/embed.go`).

**Sem rodar o build**, o stub commitado em `dist/sidecar.bundle.js`
faz o coletor falhar cedo com `ErrSidecarStub` — proteção contra
distribuir um binário que tenta executar TS extraction sem o bundle
real.

## Protocolo NDJSON

- **stdin**: 1 linha JSON com `ConfigEnvelope` (campos canônicos
  em `proto.go` no Go-side).
- **stdout**: N+2 linhas. Sempre `init` primeiro e `done` (ou `error`)
  último; entre eles, eventos das entidades extraídas.
- **stderr**: logs textuais (consumidos só em `--verbose`).

Schema versionado via `$schema`. MVP = `v1`. Mudanças incompatíveis
exigem ADR + bump de versão.

## Slices

Implementação progressiva (ver F-030):

- S-030.1 (este PR) — esqueleto: lê config, emite `init` + `done`
  vazio.
- S-030.2 — Service + Module + URNs.
- S-030.3 — Functions + Types + Variables.
- S-030.4 — Endpoints Express.
- S-030.5 — Endpoints NestJS.
- S-030.6 — Calls (http!, db!, mq!, in-process).
- S-030.7 — Frameworks (delegado para `npm.Collect` no Go-side).
