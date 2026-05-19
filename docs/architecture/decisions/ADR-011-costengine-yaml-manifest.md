---
id: ADR-011
title: Manifesto `costengine.yaml` — arquivo único, opcional, autoritativo
status: accepted
date: 2026-05-18
related_features: [F-009, F-011, F-013, F-017]
supersedes: []
superseded_by: []
---

# ADR-011 — `costengine.yaml` como manifesto único do repo

## Contexto

Três features pedem, hoje, "um arquivo na raiz do repo que descreva o
mapeamento entre infra/código/governo e o CostEngine":

- **F-009** (Bridge Service→Compute): estratégia `manifest` para
  declarar que `serviceURN=X` roda em `computeURN=Y`, com
  `confidence=1.0`. Hoje só implementa `tag` + `name_convention`.
- **F-011** (CODEOWNERS → Owns): mono-repos quebram a convenção
  "1 repo = 1 Service". O destino é declarar a lista de Services do
  repo num manifesto.
- **F-013** (Feature→Service via PR): resolução de `path → Service` por
  longest-prefix-match exige saber o `ModulePath` autoritativo de cada
  Service do mono-repo. Path heurístico falha quando o repo tem
  múltiplos Services compartilhando prefixos.

ADR-009 adicionou um quarto: `features.yaml` standalone na raiz do repo
para popular `feature_tags` quando anotação inline `@feature:` não basta.

Estado atual: 4 fontes diferentes na cabeça do operador (manifest
implícito de linguagem `go.mod`/`package.json` lido em F-017, mais 3
arquivos potenciais hipotéticos), com risco real de cada feature
decidir por conta — formato divergente, paths diferentes, lint
duplicado.

A pergunta a fechar: **um arquivo só guarda-chuva ou um por finalidade?**

## Decisão

**Adotar `costengine.yaml` na raiz do repo como manifesto único,
opcional e autoritativo.** Concretamente:

1. **Localização:** `./costengine.yaml` na raiz do repo. Não procurar
   em subdirs. Mono-repos com layouts especiais podem usar override
   via env var (ver §Consequências), não convenção implícita.

2. **Status:** opcional. Sua ausência não bloqueia ingest — heurísticas
   continuam (tag, name_convention, CODEOWNERS direto, anotação
   inline). Sua presença sobrescreve heurísticas conflitantes.

3. **Schema versionado:**

   ```yaml
   version: 1                # obrigatório; bump em break.
   services:                 # mono-repo: lista; mono-Service: omitir.
     - urn: urn:ce:code:acme-monorepo:service/checkout
       slug: checkout
       module_path: services/checkout
       manifest:
         path: services/checkout/go.mod
         type: go.mod        # alinha com Service.ManifestType (F-017).
       compute:              # opcional; alimenta F-009 manifest strategy.
         - urn: urn:ce:aws:111:compute/i-abc
         - urn: urn:ce:aws:111:compute/eks-checkout-pool
   owners:                   # opcional; override de CODEOWNERS.
     - service: checkout     # ref por slug local ou URN.
       team: urn:ce:org:team:payments
       people:
         - urn:ce:org:person:alice
   features:                 # opcional; absorve `features.yaml` de ADR-009.
     - short_id: home-banner
       owners:
         - urn:ce:code:.../function/.../home.RenderHomeBanner
         - urn:ce:code:.../call/http!.../handler.GetHome#3
   ```

   Top-level keys são todas opcionais exceto `version`. Schema completo
   vive em `docs/api/manifests/costengine.schema.json` (a criar em
   feature dedicada).

4. **Precedência por domínio:**

   | Sinal | Ordem (vence o primeiro) |
   |---|---|
   | Service em mono-repo | `costengine.yaml#/services` → inferência via build-manifest (`go.mod`/`package.json`, F-017) → 1 repo = 1 Service (fallback) |
   | Service→Compute | `costengine.yaml#/services[].compute` (`confidence=1.0`, `source=manifest`) → tag (1.0) → name_convention (0.7) |
   | Owners | `costengine.yaml#/owners` → `CODEOWNERS` (F-011) |
   | feature_tags | `@feature:` inline → `costengine.yaml#/features` → PR label (extensão F-013) |

   Empate dentro de uma mesma fonte é diagnóstico (ingestor reporta
   ambíguo, não escolhe).

5. **`features.yaml` standalone (ADR-009):** continua aceito durante
   uma janela de compatibilidade, mas marcado deprecated. Novos repos
   devem usar `costengine.yaml#/features`. O coletor aceita ambos;
   conflito é reportado como ambíguo.

6. **Validação:**
   - Comando `ce manifest validate [--repo=<path>]` que parseia o
     YAML, checa schema, resolve URNs declaradas contra o grafo
     (`as_of=now`) e reporta drift.
   - Schema strict no parser: chave desconhecida → erro com sugestão
     (não silencia — manifestos crescem em escopo).

7. **Versionamento do manifesto:** campo `version: 1` obrigatório.
   Mudanças aditivas (novas keys) não bumpam. Quebras de semântica
   incrementam `version`; parser rejeita versões desconhecidas com
   mensagem clara.

8. **Não-objetivos explícitos:**
   - **Não substitui** `go.mod` / `package.json` / `pyproject.toml`
     — esses continuam sendo a fonte primária de identidade de
     Service (F-017). `costengine.yaml#/services[].manifest` só
     *aponta* para eles.
   - **Não declara recursos de infra.** Compute/Persistence vêm da
     discovery (F-001). O manifest só os referencia por URN para
     amarrar à Service.
   - **Não substitui CODEOWNERS** — quem só usa GitHub-native
     continua valendo. O manifest é override quando precisa.

## Alternativas consideradas

### A — Manter 3+ arquivos separados (status quo implícito)
Descartada. Cada feature decidir o próprio formato leva a divergência
de nomes (`costengine.yaml` vs `features.yaml` vs hipotético
`owners.yaml`), lint duplicado, ferramenta diferente para validar
cada um. Custo operacional alto para vidas operadoras com pouco
ganho técnico.

### B — Manifesto único `costengine.yaml` (escolhida)
Pró: um arquivo, um schema versionado, uma validação. Pega carona
no padrão setorial (`renovate.json`, `dependabot.yml` na raiz). Permite
seções opcionais — repos só usam o que precisam. Centraliza diagnóstico
de drift.

Contra: arquivo cresce e vira "god config" se não disciplinado.
Mitigação: schema strict + lint que avisa quando uma seção fica vazia
(remover é melhor que manter placeholder).

### C — Discovery distribuída (`costengine/` dir com vários files)
Descartada. Reduz acoplamento mas multiplica arquivos sem benefício
real para o tamanho atual do manifest (~30 linhas em repo médio).
Reabrir quando alguma seção crescer para >200 linhas e justificar split.

### D — Schema embedded no `package.json` / `go.mod`
Descartada. Mistura ferramentas, complica parsing por linguagem,
exige convenções por manifest-type. Manifesto próprio é simples e
agnóstico de linguagem.

## Consequências

### Positivas
- **Um lugar canônico** para todas as overrides do CostEngine.
  Reduz "onde declaro X?" a uma resposta.
- **F-009 destravado** para implementar a estratégia `manifest`
  (hoje placeholder em `bridge/service_compute`).
- **F-011 destravado** para mono-repos sem heurística frágil.
- **F-013 destravado** para resolução determinística de
  `path → Service` em mono-repo.
- **ADR-009 simplificada** — `features.yaml` vira ponteiro para
  uma seção do `costengine.yaml`.
- **Diagnóstico unificado:** `ce manifest validate` cobre todas as
  intenções declaradas no repo.

### Negativas / custo
- **Novo schema a manter.** Versão `1` é o ponto de partida; bumps
  precisam de ADR adicional ou nota neste mesmo doc.
- **Migração de quem já escreveu `features.yaml`** (se houver).
  Mitigado pelo aceite de ambos durante janela de compatibilidade.
- **Override por env var (`CE_MANIFEST_PATH`)** vira necessidade em
  setups exóticos. Adicionar apenas quando alguém pedir; não
  proativo.
- **Documentação adicional** — schema, exemplos por seção,
  changelog do manifest. Aceitável.

### Quando reabrir
- Se o manifesto crescer para >5 seções top-level com semânticas
  ortogonais (alternativa C fica atrativa).
- Se mais de um repo precisar declarar manifesto em local não-raiz
  por motivo legítimo (alternativa "discovery distribuída").
- Se duas features divergirem sobre como interpretar a mesma key
  (sinal de que o schema está sobrecarregado).

## Implementação esperada

Esta ADR define o contrato. A implementação concreta entra como
features quando houver dor real:

- **Parser + validador (`ce manifest validate`)** — feature nova
  quando primeira estratégia `manifest` em `service_compute` for
  promovida.
- **Estratégia `manifest` em F-009** — slice dentro de F-009,
  consome a seção `services[].compute`.
- **Override de CODEOWNERS em F-011** — slice quando primeiro
  mono-repo entrar.
- **Path → Service em F-013** — usar `services[].module_path` no
  longest-prefix-match.
- **Migração de `features.yaml`** — coletor de feature_tags aceita
  ambos; deprecation warning quando `features.yaml` é encontrado.

Backlog de promoção fica em `docs/backlog/pendencias.md`.

## Referências

- F-009: docs/features/F-009-bridge-service-to-compute.md
- F-011: docs/features/F-011-codeowners-to-owns.md
- F-013: docs/features/F-013-feature-service-link-via-pr.md
- F-017: docs/features/F-017-cross-language-identity.md
- ADR-004: edge `RUNS_ON` (menciona `manifest` como source futuro)
- ADR-009: feature-tag denormalizada (cita `features.yaml`)
- pendencias §5: motivação registrada antes deste ADR
