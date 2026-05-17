package edge

// =============================================================================
// Code plane edges (E-007) — ADR-007/008.
// =============================================================================

// Invokes modela a relação Function → Call (ADR-007).
//
//	Function ──INVOKES──▶ Call
//
// Toda Call origina-se de uma Function declarante. Ordinal preservado
// no nó Call (`Ordinal`), não na edge.
type Invokes struct{ Base }

// Targets modela o alvo resolvido de um Call (ADR-007).
//
//	Call ──TARGETS──▶ Function | Endpoint | Persistence | Messaging | Schema
//
// Opcional — Call sem TARGETS é alvo dinâmico/externo não-resolvível
// (ex.: HttpCall para URL literal externa).
type Targets struct{ Base }

// Uses modela referência de Call/Module/Service a Framework.
//
//	Call    ──USES──▶ Framework  (cliente library detectado)
//	Module  ──USES──▶ Framework  (import declarado)
//	Service ──USES──▶ Framework
type Uses struct{ Base }

// Implements modela satisfação estrutural de interface entre Types
// (F-021):
//
//	Type ──IMPLEMENTS──▶ Type
//
// Em Go: type struct satisfaz interface se tem todos os métodos.
// Em TS/Java: relação explícita via `implements`.
type Implements struct{ Base }

// Extends modela herança/embedding entre Types (F-021):
//
//	Type ──EXTENDS──▶ Type
type Extends struct{ Base }

// Aliases modela `type X = Y` semântico entre Types (F-021).
type Aliases struct{ Base }

// SerializesAs modela a ponte Type → Schema (F-022):
//
//	Type ──SERIALIZES_AS──▶ Schema
//
// Type gerado a partir de contrato externo (proto/openapi).
type SerializesAs struct{ Base }

// Imports modela referência entre Schemas (F-022):
//
//	Schema ──IMPORTS──▶ Schema
//
// Suporta proto imports e $ref de OpenAPI.
type Imports struct{ Base }

// LicensedUnder modela Framework → License (F-023).
type LicensedUnder struct{ Base }

// AffectedBy modela Framework → SecurityAdvisory (F-023).
type AffectedBy struct{ Base }

// PatchedIn modela SecurityAdvisory → Framework (F-023). `since_version`
// preenche `Meta.Properties`.
type PatchedIn struct{ Base }

// =============================================================================
// Governance plane edges (E-008).
// =============================================================================

// Delivers modela Feature → UserStory (F-025).
type Delivers struct{ Base }

// AssignedTo modela Person → UserStory (F-025). Cardinalidade 1.
type AssignedTo struct{ Base }

// Serves modela UserStory → Persona (F-026). ADR-009 — Persona NÃO
// é denormalizada em código; só chega via UserStory.
type Serves struct{ Base }

// =============================================================================
// Org plane edges (F-027).
// =============================================================================

// HasRole modela Person → Role (F-027). Cardinalidade 1 (uma role
// corrente; bitemporal cobre promoções).
type HasRole struct{ Base }

// LedBy é a aresta opcional Team → Person (F-027): lideranças de time.
type LedBy struct{ Base }

// =============================================================================
// Bridge gov→code edges (F-013).
// =============================================================================

// Realizes modela Feature → Service (F-013).
//
//	Feature ──REALIZES──▶ Service
//
// Capturada via webhook GitHub: label `feature:<feature_urn>` em PR
// mergeado + arquivos tocados resolvidos em Services. `Source.Properties`
// guarda `pr_url` e `merged_at` para auditoria.
type Realizes struct{ Base }
