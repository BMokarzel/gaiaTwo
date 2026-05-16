package edge

// MemberOf modela a relação org plane (F-010):
//
//	Person ──MEMBER_OF──▶ Squad
//
// Direcional. Fechar (`valid_to`) quando a pessoa muda de squad ou
// sai da empresa (end_date preenchido no CSV de HRIS).
type MemberOf struct{ Base }

// PartOf modela a relação:
//
//	Squad ──PART_OF──▶ Team
type PartOf struct{ Base }

// ReportsTo modela a relação hierárquica:
//
//	Person ──REPORTS_TO──▶ Person (manager)
//
// O parser HRIS rejeita ciclos (F-010 D6).
type ReportsTo struct{ Base }

// Owns modela ownership de código (F-011):
//
//	Person ──OWNS──▶ Service
//	Team   ──OWNS──▶ Service
//
// Direcional. Fonte da verdade: arquivo CODEOWNERS do repo. Trocar o
// owner no CODEOWNERS fecha o edge antigo e cria um novo (bitemporal).
// Tanto Person quanto Team podem ser owners — owner duplicado (handle
// pessoal + handle de team) gera dois edges.
type Owns struct{ Base }
