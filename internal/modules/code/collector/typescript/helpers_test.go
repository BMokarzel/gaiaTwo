package typescript

import (
	"context"
	"testing"
)

// testContext devolve um context.Context que é cancelado quando o
// teste termina. Mantido helper local para evitar `t.Context()` que só
// existe no Go ≥1.24 e mantém este pacote compilável em toolchains
// mais antigas.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx
}
