---
id: F-019
title: Call boundary nodes (Http, Rpc, Event, Queue, DataAccess, Job)
status: entity + coletor Go (HttpCall, DataAccess, RpcCall, QueueSend/Receive, EventPublish, JobSchedule) done; EventSubscribe backlog
modules: [code]
depends_on: [F-018]
modeling_impact: yes
adrs: [ADR-007, ADR-008]
epic: E-007
updated: 2026-05-17
---

# F-019 — Call boundary nodes

## Problema

Toda comunicação que cruza fronteira de processo é um custo de
arquitetura (latência, falha cascata, escolha de protocolo) e precisa
ser nó de primeira classe para simulação.

## Mudanças

- Novo kind `Call` em `internal/entity/node/call.go` com discriminador
  `CallKind`: `http`, `rpc`, `event_publish`, `event_subscribe`,
  `queue_send`, `queue_receive`, `data_access`, `job_schedule`.
- URN: `urn:ce:code:<repo>:call/<kind>!<caller-function-id>#<ordinal>`.
  `ordinal` é AST-stable (ADR-007).
- Args/Returns como `[]ArgSlot` com `Source` discriminado
  (`literal:`, `variable:`, `param:`, `call:`, `unknown`).
- Arestas: `INVOKES` (Function→Call), `TARGETS` (Call→Function/
  Endpoint/Persistence/Messaging/Schema/Service/Type), `USES`
  (Call→Framework).

## DoD

- [x] `node.Call` + `NewCallURN` + `ContentHash`.
- [x] Adjacency INVOKES/TARGETS/USES validadas.
- [x] Coletor Go detecta `net/http.Get/Post/PostForm/Head/Do`
      (HttpCall) e `Query*/Exec*` (DataAccess) com `OperationKind` /
      `TargetURL` / `TargetMethod`.
- [x] Fixture: handler com `http.Get` + `DB.QueryContext` gera
      1 HttpCall + 1 DataAccess + INVOKES + USES.
- [x] Coletores adicionais: gRPC (`Invoke`/`NewStream`),
      AWS SQS (`SendMessage`/`ReceiveMessage`/batch),
      AWS SNS (`Publish`/`PublishBatch`),
      AWS Kinesis (`PutRecord`/`PutRecords` → EventPublish),
      schedulers robfig/cron (`AddFunc`/`AddJob` → JobSchedule com
      spec capturada em `Topic`). Fixture em
      `call_subkinds_test.go::TestExtractCalls_F019Subkinds`.
- [ ] EventSubscribe (consumer kafka/pubsub) — backlog.
      Pattern menos óbvio sintaticamente: consumidores são loops sobre
      iteradores/canais. Provavelmente exige type-resolve (F-021).
