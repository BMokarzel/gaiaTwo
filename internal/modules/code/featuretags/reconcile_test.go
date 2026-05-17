package featuretags

import (
	"reflect"
	"testing"

	"costEngine/internal/entity/node"
)

func TestBuildCatalog_IgnoresEmpty(t *testing.T) {
	cat := BuildCatalog([]node.Feature{
		{ShortID: "checkout"},
		{ShortID: ""},
		{ShortID: "pix"},
		{ShortID: "checkout"}, // duplicate idempotent
	})
	if !cat.Has("checkout") || !cat.Has("pix") {
		t.Errorf("missing expected tag")
	}
	if cat.Has("") {
		t.Errorf("empty ShortID não deveria entrar no catálogo")
	}
	if len(cat) != 2 {
		t.Errorf("catalog size=%d want 2", len(cat))
	}
}

// TestReconcile_F028OrphanWarning é o caminho feliz da reconciliação:
// tags válidas atravessam silenciosamente, órfãs viram Orphan + UniqueOrphan.
func TestReconcile_F028OrphanWarning(t *testing.T) {
	cat := BuildCatalog([]node.Feature{
		{ShortID: "checkout"},
		{ShortID: "pix"},
	})

	fns := []node.Function{
		{Base: node.Base{NodeURN: "urn:ce:code:r:function/a", NodeKind: node.KindFunction}, FeatureTags: []string{"checkout"}},
		{Base: node.Base{NodeURN: "urn:ce:code:r:function/b", NodeKind: node.KindFunction}, FeatureTags: []string{"checkout", "unknown1"}},
		{Base: node.Base{NodeURN: "urn:ce:code:r:function/c", NodeKind: node.KindFunction}, FeatureTags: []string{"unknown2"}},
		{Base: node.Base{NodeURN: "urn:ce:code:r:function/d", NodeKind: node.KindFunction}, FeatureTags: nil},
	}
	calls := []node.Call{
		{Base: node.Base{NodeURN: "urn:ce:code:r:call/x", NodeKind: node.KindCall}, FeatureTags: []string{"pix"}},
		{Base: node.Base{NodeURN: "urn:ce:code:r:call/y", NodeKind: node.KindCall}, FeatureTags: []string{"unknown1"}}, // dup orphan
	}

	tagged := append(FromFunctions(fns), FromCalls(calls)...)
	rep := Reconcile(tagged, cat)

	wantUnique := []string{"checkout", "pix", "unknown1", "unknown2"}
	if !reflect.DeepEqual(rep.UniqueTags, wantUnique) {
		t.Errorf("UniqueTags=%v want %v", rep.UniqueTags, wantUnique)
	}
	wantOrphan := []string{"unknown1", "unknown2"}
	if !reflect.DeepEqual(rep.UniqueOrphan, wantOrphan) {
		t.Errorf("UniqueOrphan=%v want %v", rep.UniqueOrphan, wantOrphan)
	}
	// 3 ocorrências: function/b com unknown1, function/c com unknown2, call/y com unknown1.
	if len(rep.Orphans) != 3 {
		t.Errorf("Orphans=%d want 3", len(rep.Orphans))
	}
	// Confere shape de um Orphan.
	for _, o := range rep.Orphans {
		if o.NodeURN == "" || o.Tag == "" || o.NodeKind == "" {
			t.Errorf("Orphan incompleto: %+v", o)
		}
	}
}

// TestReconcile_EmptyCatalogTreatsAllAsOrphan: ausência de catálogo
// (ex: pre-cadastro) marca tudo como órfão — opera como linter mas
// sem filtrar.
func TestReconcile_EmptyCatalogTreatsAllAsOrphan(t *testing.T) {
	cat := BuildCatalog(nil)
	fns := []node.Function{
		{Base: node.Base{NodeURN: "urn:ce:code:r:function/a", NodeKind: node.KindFunction}, FeatureTags: []string{"x", "y"}},
	}
	rep := Reconcile(FromFunctions(fns), cat)
	if len(rep.Orphans) != 2 || len(rep.UniqueOrphan) != 2 {
		t.Errorf("Orphans=%d UniqueOrphan=%v want 2 each", len(rep.Orphans), rep.UniqueOrphan)
	}
}
