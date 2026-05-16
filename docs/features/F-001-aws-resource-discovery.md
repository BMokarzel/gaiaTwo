---
id: F-001
title: AWS Resource Discovery
status: done
modules: [infra]
depends_on: []
modeling_impact: no
adrs: [ADR-002]
epic: E-001
updated: 2026-05-14
---

# F-001 — AWS Resource Discovery

## Problema
Para que o grafo represente a realidade, alguém precisa ler o que existe
nas contas AWS dos tenants e produzir nós `Compute`, `Persistence`,
`Network`, `Messaging` + a hierarquia `Account → Region → Zone`. Sem
isso, todo o resto (custo, code linking, simulação) opera no vazio.

**Para quem:** operador de plataforma (rodando `ce extract aws ...`)
e, futuramente, o produto Arquitetura que consome o grafo.

**Dor sem ela:** o grafo está vazio; nenhum produto tem dado.

## Escopo

**Inclui:**
- Coleta de EC2 (Compute), EBS (Persistence/block), S3 (Persistence/object),
  RDS (Persistence/rdbms), VPC/Subnet/SG/LB (Network) numa conta AWS.
- Mapeamento de cada recurso AWS → URN canônica.
- Upsert bitemporal no `NodeRepository`.
- Criação automática dos nós ancestrais (`Account`, `Region`, `Zone`).
- Edge `Contains` da hierarquia.
- CLI: `ce extract aws --account=<id> --region=<region>`.

**NÃO inclui:**
- GCP, Azure, K8s (features próprias).
- Tags como `Labels` semânticos (apenas guarda em `tags_json`).
- Linkagem com código ou custo (vive em outras features).
- Reconciliação inteligente de drift (recursos deletados — Fase posterior).
- Multi-conta em paralelo (uma conta por invocação).

**Precondições:**
- Repository layer pronto (✅).
- Credencial AWS disponível para a CLI (assumirole ou keys).

## Toque no grafo

- **Lê:** `Account`, `Region`, `Zone`, `Compute`, `Persistence`,
  `Network`, `Messaging` existentes (para detectar atualizações vs
  criações).
- **Escreve:** todos os Kinds de `infra` + edge `Contains`.
- **Novos Kinds/edges:** nenhum.
- **Bitemporal:** primeira execução cria versão 1 com `valid_from=now`,
  `valid_to=null`. Execuções seguintes: se hash dos campos
  significativos mudou, fecha versão atual e cria nova; se igual,
  apenas atualiza `observed_at`.

## Critérios de aceite

- [ ] Dado uma conta AWS com 5 EC2 + 2 EBS + 1 RDS, quando rodar
      `ce extract aws --account=X --region=us-east-1`, então o grafo
      contém 1 Account + 1 Region + 1+ Zones + 5 Compute + 2 Persistence
      + 1 Persistence, todos versão 1.
- [ ] Dado uma execução prévia, quando re-executar sem mudanças no AWS,
      então nenhum nó ganha versão nova (só `observed_at` atualiza).
- [ ] Dado uma execução prévia e uma EC2 que teve `instance_type`
      alterado, quando re-executar, então o `Compute` correspondente
      tem versão 2 e versão 1 tem `valid_to` preenchido.
- [ ] Dado credenciais inválidas, quando rodar o comando, então sai
      com erro claro identificando que é problema de auth.
- [ ] Dado erro intermitente de API AWS num recurso específico, quando
      rodar, então os outros recursos são processados e o erro é
      reportado no fim (não derruba a execução).
- [ ] Comando reporta totais por Kind: "5 Compute (3 new, 2 updated)".

## Riscos / incerteza

- **Paginação e rate limit do AWS SDK.** SDK Go cuida da maior parte;
  validar com conta grande.
- **External_id consistente.** Decisão: usar ARN para tudo que tem,
  ID nativo (`i-...`, `vol-...`) como fallback. Testar idempotência.
- **Mapeamento Zone.** EC2 retorna `availability_zone` por instância;
  criar nó `Zone` derivado. Não há entidade explícita de Zone no AWS.
- **EBS detached.** Volumes não atrelados existem; ainda devem virar
  `Persistence` (sem edge `AttachedTo`).

## Notas de implementação

- Implementação em `internal/modules/infra/collector/aws/` (a criar).
- Interface `Collector` definida em `internal/modules/infra/collector/`.
- Workers em goroutines no MVP; promover para `cmd/worker` quando
  Topologia A → B (ver `architecture/06-system.md`).
- Idempotência: hash determinístico dos campos significativos +
  comparação antes de fazer Upsert.

## Stories

### S-001 — Scaffold do coletor AWS + comando CLI vazio ✅
**Status:** done (2026-05-14)
**Comportamento:** Dado que o operador roda `ce extract aws --account=<id> --region=<region>` com credenciais válidas, quando o comando executa, então valida as credenciais via STS `GetCallerIdentity` e imprime "ok: conta=<id> região=<region>" sem tocar no grafo.
**Camadas tocadas:** [cli, modules/infra/collector]
**Entregue:**
- `internal/modules/infra/collector/collector.go` — interfaces `Collector` + `Validator` + `Scope`
- `internal/modules/infra/collector/aws/aws.go` — Collector AWS (Validate via STS; Discover stub); erros tipados `ErrAuth`, `ErrAccount`
- `internal/modules/infra/collector/aws/aws_test.go` — testes com fake STSClient (5 cenários)
- `cmd/cli/main.go` — binário `ce` com `extract aws --account/--region`, exit codes discriminados (2/3/4/1)
- Deps: `aws-sdk-go-v2/{config,service/sts}`

### S-002 — Hierarquia Account → Region → Zone com edge `Contains` ✅
**Status:** done (2026-05-14)
**Comportamento:** Dado credenciais válidas, quando rodar `ce extract aws --account=X --region=us-east-1`, então o grafo contém 1 nó `Account`, 1 nó `Region`, e 1+ nós `Zone` (descobertos via `DescribeAvailabilityZones`), todos versão 1, ligados por edges `Contains` (Account→Region→Zone).
**Camadas tocadas:** [cli, modules/infra/collector, repository]
**Entregue:**
- `collector/collector.go` — `ScopeTopology` + `ScopeDiscoverer` (contrato separado de `Collector` porque scopes não são Resource)
- `collector/aws/topology.go` — `Topology` que descobre AZs via EC2 `DescribeAvailabilityZones`; meta bitemporal com `valid_from=now`, `version=1`, `source.collector="aws"`
- `service/discover.go` — `DiscoverService.EnsureScopeAncestors` orquestra Discover→Upsert; helper `endpointKinds` extrai Kinds via `ParseURN` para `edge.Validate`
- Tests: 5 cenários em topology (happy, idempotência de edge IDs, scope vazio, EC2 erro, AZ name vazio); 3 em service (persiste+reporta, erro propagado, idempotência bitemporal cria 3 versões com 1 corrente)
- CLI: `ce extract aws` agora persiste em backend memory; saída `accounts=1 regions=1 zones=N edges=N+1`
- Dep: `aws-sdk-go-v2/service/ec2`
**Backlog técnico:** wiring de `n4j` como backend opcional (`--backend=neo4j`) fica para story dedicada após S-008 — basta trocar o repo no CLI.

### S-003 — Discovery de EC2 (Compute) com prova de idempotência bitemporal ✅
**Status:** done (2026-05-14)
**Comportamento:**
- Dado conta com N EC2, quando rodar o comando, então existem N nós `Compute` versão 1 com edge `Contains` partindo da `Zone` correta.
- Dado execução prévia sem mudanças no AWS, quando re-executar, então nenhum `Compute` ganha versão nova; só `observed_at` atualiza.
- Dado uma EC2 que teve `instance_type` alterado, quando re-executar, então o `Compute` correspondente tem versão 2 e versão 1 ganhou `valid_to`.
**Camadas tocadas:** [modules/infra/collector, repository]
**Blocked by:** [S-002]
**Entregue:**
- `entity/node/compute.go` — `Compute.ContentHash()` (SHA256 de provider/flavor/instance_type/vcpus/memory/state/tags ordenadas; exclui Meta+SpecRaw)
- `entity/edge/registry.go` — `Contains.From` estendido com `KindZone` (permite Zone→Compute)
- `repository/repository.go` — `NodeRepository.Touch(urn)` atualiza só ObservedAt sem versionar; `repository/memory` + `repository/n4j` implementam (memory via `touchNode` type-switch; n4j via `SET n.observed_at = $now WHERE valid_to IS NULL`)
- `collector/collector.go` — `ComputeBatch` + `ComputeDiscoverer` (separado de `Collector` pelo mesmo motivo de `ScopeDiscoverer`: caller precisa decidir bitemporal por nó)
- `collector/aws/ec2.go` — `EC2` usa `DescribeInstances` com paginação via `NextToken`; converte `ec2types.Instance` → `node.Compute` (Flavor=VM, mapeia `InstanceStateName` → `LifecycleState`); edges Zone→Compute via lookup por AZ no `topo.Zones`; instâncias em AZ desconhecida são puladas (sem edge)
- `collector/aws/ec2_test.go` — 7 cenários (happy, paginação, edge ID determinístico, instance-id em branco, AZ desconhecida, erro de API, scope vazio)
- `service/discover.go` — `DiscoverCompute` orquestra: para cada `Compute`, `GetByURN` corrente; se ausente, Upsert (new); se igual via `ContentHash`, `Touch` (unchanged); se diferente, Upsert com `Version++` (updated). Retorna `ComputeReport{New,Updated,Unchanged,Edges}`. `EnsureScopeAncestorsWithTopology` expõe a topologia descoberta para reaproveitar em `DiscoverCompute`.
- `service/discover_test.go` — 4 cenários (new, unchanged só toca, mudança bumpa versão, falha sem ComputeDiscoverer)
- CLI: imprime `compute: new=N updated=M unchanged=K edges=N+M`
**Notas:** Primeira coleta com dados reais. Hash determinístico em `Compute.ContentHash()` exclui propositadamente `Meta` (versão/tempo) e `SpecRaw` (ruído do provedor). Padrão replicado pelas próximas stories.

### S-004 — Discovery de EBS (Persistence/block) ✅
**Status:** done (2026-05-14)
**Comportamento:** Dado conta com M volumes EBS (incluindo detached), quando rodar o comando, então existem M nós `Persistence` (subkind block) versão 1 — volumes detached também são criados, sem edge para Compute.
**Camadas tocadas:** [modules/infra/collector, repository]
**Blocked by:** [S-002]
**Entregue:**
- `entity/node/persistence.go` — `Persistence.ContentHash()` (Provider/Flavor/Engine/SizeGiB/IOPS/Encrypted/Tags ordenadas; exclui Meta+SpecRaw)
- `collector/collector.go` — `PersistenceBatch` + `PersistenceDiscoverer` (slice no service: múltiplas implementações coexistem; S-005/S-006 acoplam S3/RDS aqui)
- `collector/aws/ebs.go` — `EBS` discoverer via `DescribeVolumes` paginado; converte `ec2types.Volume` → `node.Persistence` (Flavor=block, Engine=VolumeType "gp3"/"io2"); edges Zone→Persistence (Contains) + Persistence→Compute (AttachedTo) por attachment; detached volumes geram só Contains; AZ desconhecida → Persistence sem Contains
- `collector/aws/ebs_test.go` — 8 cenários (attached+detached, multi-attachment, paginação, blank id, AZ desconhecida, edge IDs determinísticos, erro de API, scope vazio)
- `service/discover.go` — `DiscoverPersistence` itera todos os discoverers registrados; aplica mesma lógica ContentHash + Touch/Upsert que `DiscoverCompute`. `WithPersistenceDiscoverer` é variadic-friendly (append em slice).
- `service/discover_test.go` — 4 cenários (new, unchanged Touch, size 100→200 bumpa versão, falha sem discoverer)
- CLI: `ce extract aws` imprime `persistence: new=N updated=M unchanged=K edges=...`
**Notas:** O contrato `PersistenceDiscoverer` foi desenhado já pensando em S-005/S-006 (S3, RDS): cada flavor é um discoverer independente plugado pelo mesmo setter; o service agrega.

### S-005 — Discovery de S3 (Persistence/object) ✅
**Status:** done (2026-05-14)
**Comportamento:** Dado conta com K buckets S3, quando rodar o comando, então existem K nós `Persistence` (subkind object) versão 1 sob `Account` (S3 é global, não tem `Region` no edge `Contains` — fica direto sob Account).
**Camadas tocadas:** [modules/infra/collector, repository]
**Blocked by:** [S-002]
**Entregue:**
- `collector/aws/s3.go` — `S3` discoverer via `ListBuckets`; prefere `Bucket.BucketRegion` (SDK v2 moderno) e cai para `GetBucketLocation` quando ausente; normaliza códigos legados (`""→us-east-1`, `"EU"→eu-west-1`); edge `Contains` vai direto `Account → Persistence` (sem Zone/Region intermediária, pois buckets não são AZ-scoped)
- `collector/aws/s3_test.go` — 8 cenários (happy path com BucketRegion, fallback para GetBucketLocation, normalização legada, blank skip, edge IDs determinísticos, erro list, erro location, account vazio)
- Reuso de `DiscoverPersistence`: S3 é registrado como mais um `PersistenceDiscoverer` ao lado do EBS; service itera ambos e agrega o relatório (validado pelo desenho do contrato em S-004)
- Dep: `aws-sdk-go-v2/service/s3 v1.101.0`
- CLI: `ce extract aws` continua imprimindo `persistence: new=N updated=M ...` somando EBS+S3
**Notas:** Decisão de modelagem documentada em comentário do `s3.go`: edge `Contains` direto de Account porque buckets não são AZ-scoped e o campo Region é apenas metadado por bucket, não estrutural.

### S-006 — Discovery de RDS (Persistence/rdbms) ✅
**Status:** done (2026-05-14)
**Comportamento:** Dado conta com P instâncias RDS, quando rodar o comando, então existem P nós `Persistence` (subkind rdbms) versão 1 ligados à `Zone` correspondente.
**Camadas tocadas:** [modules/infra/collector, repository]
**Blocked by:** [S-002]
**Entregue:**
- `collector/aws/rds.go` — `RDS` discoverer via `DescribeDBInstances` paginado por `Marker`; converte `rdstypes.DBInstance` → `node.Persistence` (Flavor=rdbms, Engine="<engine>-<version>" via `rdsEngineLabel`, SizeGiB=AllocatedStorage, IOPS, Encrypted=StorageEncrypted, Tags); edge Zone→Persistence (Contains) via AZ no `topo.Zones`; AZ desconhecida → Persistence sem Contains
- `collector/aws/rds_test.go` — 8 cenários (happy path 2 instâncias com tags, paginação Marker, blank id, AZ desconhecida, edge IDs determinísticos, engine sem versão (fallback), erro de API, scope vazio)
- Reuso de `DiscoverPersistence`: RDS é registrado como mais um `PersistenceDiscoverer` ao lado de EBS/S3; service itera todos e agrega no relatório
- Dep: `aws-sdk-go-v2/service/rds v1.99.0`
- CLI: `ce extract aws` continua imprimindo `persistence: new=N updated=M ...` somando EBS+S3+RDS
**Notas:** MultiAZ não modificou a topologia — instâncias MultiAZ continuam ancoradas na AZ primária reportada pela API; o aspecto pode virar atributo no `Persistence` se necessário, sem mudar edges.

### S-007a — Discovery de Network: VPC + Subnet ✅
**Status:** done (2026-05-14)
**Comportamento:** Dado conta com VPCs e subnets, quando rodar o comando, então cada um vira nó `Network` (subkind `vpc` ou `subnet`) versão 1, sob `Region` (VPC) ou `Zone` (Subnet), com edge `Contains` adicional VPC→Subnet quando a relação está disponível.
**Camadas tocadas:** [entity/node, modules/infra/collector, modules/infra/service]
**Blocked by:** [S-002]
**Entregue:**
- `entity/node/network.go` — `Network.ContentHash()` (Provider/Flavor/CIDR/Tags ordenadas; exclui Meta+SpecRaw), paralelo a Compute/Persistence
- `collector/collector.go` — `NetworkBatch` + `NetworkDiscoverer` (slice no service, múltiplas implementações coexistem; S-007b vai adicionar SG/LB sob o mesmo contrato)
- `collector/aws/vpc.go` — `VPC` discoverer faz `DescribeVpcs` + `DescribeSubnets` (ambos paginados); converte `ec2types.Vpc` → `Network(vpc, CIDR)` com edge Region→VPC; `ec2types.Subnet` → `Network(subnet, CIDR)` com edges Zone→Subnet + VPC→Subnet; subnets com AZ desconhecida ou VpcId órfão recebem apenas a(s) edge(s) disponíveis (sem aresta órfã)
- `collector/aws/vpc_test.go` — 9 cenários (happy path com 1 VPC + 2 subnets e 5 edges esperadas, paginação 2+2, AZ desconhecida não emite Zone-edge, VPC órfão não emite VPC-edge, blanks pulados, edge IDs determinísticos, erro VPCs, erro Subnets, scope vazio)
- `service/discover.go` — `WithNetworkDiscoverer` + `DiscoverNetwork` retornando `NetworkReport{New,Updated,Unchanged,Edges}`; mesma lógica ContentHash + Touch/Upsert dos casos Compute/Persistence
- `service/discover_test.go` — 4 cenários (new, unchanged Touch, CIDR change bumpa versão, falha sem discoverer)
- CLI: `ce extract aws` imprime nova linha `network: new=N updated=M unchanged=K edges=...`
**Notas:** Slice (a) do S-007 conforme planejado. (b) — SG + LB — segue como story independente. Edge `Contains` Network→Network (VPC→Subnet) já estava permitida na matriz de adjacência em `edge/registry.go`.

### S-007b — Discovery de Network: SG + LB ✅
**Status:** done (2026-05-14)
**Comportamento:** Dado conta com security groups e load balancers, quando rodar o comando, então cada um vira nó `Network` versão 1: SG (flavor `securitygroup`) referenciando a VPC quando disponível; LB (flavor `loadbalancer`) sob Region, com edges VPC→LB e Zone→LB quando aplicável.
**Camadas tocadas:** [entity/node, modules/infra/collector]
**Blocked by:** [S-007a]
**Entregue:**
- `entity/node/network.go` — novo `NetworkSecurityGroup = "securitygroup"` (flavor dedicado em vez de reusar Endpoint; SG é construto de filtro, não recurso de tráfego)
- `collector/aws/sg.go` — `SG` discoverer via `DescribeSecurityGroups` paginado por `NextToken`; converte `ec2types.SecurityGroup` → `Network(securitygroup)`; edge VPC→SG (`Contains`) emitida com URN canônica do `VpcId` mesmo sem garantir presença do VPC no batch corrente (service layer tolera por DeterministicID + upsert idempotente)
- `collector/aws/sg_test.go` — 7 cenários (happy 2 SGs + 2 edges, SG clássico sem VpcId não emite edge, paginação, blank skip, edge IDs determinísticos, erro de API, scope vazio)
- `collector/aws/lb.go` — `LB` discoverer via `DescribeLoadBalancers` (ELBv2) paginado por `Marker`; converte `elbtypes.LoadBalancer` → `Network(loadbalancer)` com tags `lb:type` (application/network/gateway) e `lb:scheme` (internet-facing/internal); edges Region→LB (sempre), VPC→LB (se VpcId), Zone→LB (uma por AZ presente em `topo.Zones`); ExtID prefere `LoadBalancerArn`, cai para `LoadBalancerName`
- `collector/aws/lb_test.go` — 9 cenários (happy path ALB com 4 edges esperadas, sem VpcId, AZ desconhecida pulada, paginação, fallback de ExtID para Name, blank skip, edge IDs determinísticos, erro de API, scope vazio)
- `collector/aws/lb.go` provê helper `containsEdge(from, to, now) edge.Contains` reusado por `sg.go` (evita duplicação)
- Reuso de `DiscoverNetwork`: SG e LB são registrados como mais dois `NetworkDiscoverer` ao lado do VPC; service itera todos e agrega no relatório
- Dep: `aws-sdk-go-v2/service/elasticloadbalancingv2 v1.54.12`
- CLI: linha `network:` agora soma VPC+Subnet+SG+LB
**Notas:** Subtype técnico do LB (application/network/gateway) e scheme (internet-facing/internal) vão como Tags em vez de novos flavors — evita inflar o domínio antes de haver uso analítico concreto. SG ganhou flavor próprio porque é semanticamente filtro/política, distinto de endpoint de tráfego.

### S-008 — Resiliência: erro parcial, auth inválida, relatório de totais ✅
**Status:** done (2026-05-14)
**Comportamento:**
- Dado credenciais inválidas, quando rodar o comando, então sai com exit code 3 e mensagem `aws: authentication failed: <detalhe>` (já entregue em S-001 via `awscol.ErrAuth`; só verificada/documentada aqui).
- Dado erro intermitente da API AWS em 1 discoverer, quando rodar, então os outros são processados e o erro é reportado no final em uma seção `warnings (N)` no stderr; exit code 5 (parcial).
- Em qualquer execução, comando imprime totais por Kind no formato `Compute: 5 total (3 new, 2 updated, 0 unchanged), 5 edges`.
**Camadas tocadas:** [cli, modules/infra/service]
**Blocked by:** [S-003, S-004, S-005, S-006, S-007a, S-007b]
**Entregue:**
- `service/discover.go` — novo tipo `DiscoveryError{Stage, Err}` (implementa `error` + `Unwrap`); `PersistenceReport.Errors` e `NetworkReport.Errors` capturam falhas por-discoverer sem abortar a execução; loops em `DiscoverPersistence`/`DiscoverNetwork` acumulam erros e seguem para o próximo
- Setters `WithPersistenceDiscoverer(name, disc)` / `WithNetworkDiscoverer(name, disc)` passaram a exigir um rótulo curto ("ebs"/"s3"/"rds"/"vpc"/"sg"/"lb") usado nas mensagens de warning (`persistence/ebs: ...`)
- `service/discover_test.go` — +2 cenários: erro em 1 discoverer do slice mantém o outro funcionando, com erro capturado em `rep.Errors` e stage rotulado
- `cmd/cli/main.go` — orquestração ganha buffer `warnings` que concatena erros top-level de cada estágio + `Errors` por-discoverer; quando vazio imprime `ok:`, caso contrário `parcial: ...`; formato de relatório virou `Compute: N total (X new, Y updated, Z unchanged), W edges`; nova flag de exit code `errPartial` (5); `main` suprime o prefixo "erro:" em saídas parciais (warnings já vão para stderr)
- Topologia continua fatal (sem topologia, demais estágios não fazem sentido)
**Notas:** Auth/account não precisaram de mudança — já tinham erros tipados (`ErrAuth`, `ErrAccount`) com exit codes discriminados (3/4) desde S-001. Story consolida o contrato de erro do CLI: exit codes 0 (ok), 2 (uso), 3 (auth), 4 (account), 5 (parcial), 1 (outro). F-001 fica `done` — próxima feature: F-002 produção (Neo4j wiring) ou avançar para F-003 (CUR bridge).
