package node

// Location é a posição estruturada de um nó do code plane dentro do
// repositório. Carregada como atributo (não influencia URN/ContentHash):
// mover o código sem mudar a identidade lógica não rebumpa versão
// (ADR-006).
//
// `File` é relativo à raiz do repo. Linhas/colunas são 1-based como nos
// fileset do `go/token`. Zero-value indica "desconhecido" — usado quando
// o produtor (coletor de IaC, ferramenta externa) não preserva posição.
type Location struct {
	File     string `json:"file,omitempty"`
	LineInit int    `json:"line_init,omitempty"`
	LineEnd  int    `json:"line_end,omitempty"`
	ColInit  int    `json:"col_init,omitempty"`
	ColEnd   int    `json:"col_end,omitempty"`
}

// IsZero retorna true quando Location não carrega informação útil.
func (l Location) IsZero() bool {
	return l == Location{}
}
