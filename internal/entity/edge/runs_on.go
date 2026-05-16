package edge

// ServiceRunsOn modela o vínculo code→infra (F-009, [ADR-004]):
//
//	Service ──RUNS_ON──▶ Compute
//
// Distinto de DeployedOn (Compute→Compute, p.ex. Pod→Node). A
// resolução (qual estratégia produziu o vínculo: tag, name_convention,
// manifest) e a confiança ficam em Base.EdgeMeta (Source / Confidence /
// Properties) — o struct concreto não duplica esses campos.
//
// [ADR-004]: docs/architecture/decisions/ADR-004-runs-on-edge-for-service-compute.md
type ServiceRunsOn struct {
	Base
}
