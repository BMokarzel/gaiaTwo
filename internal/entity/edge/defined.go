package edge

// DefinedIn modela a relação code plane (F-007):
//
//	Endpoint ──DEFINED_IN──▶ Service
//	Function ──DEFINED_IN──▶ Service
//
// Direcional (do símbolo para o serviço dono). É a aresta que permite
// queries do tipo "quais endpoints o serviço X expõe?" e prepara o
// terreno para F-009 (Service ↔ Compute) sem precisar duplicar lookups.
type DefinedIn struct {
	Base
}
