package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
	"costEngine/internal/modules/code/collector/golang"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := makeAllAndWrite(path, body); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestApply_WritesNodesAndEdges(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module ex\n")
	writeFile(t, filepath.Join(root, "service", "s.go"), `package service
func DoIt() {}
`)
	writeFile(t, filepath.Join(root, "api", "r.go"), `package api
func reg(r interface{}) { r.Get("/x", h) }
func h() {}
`)

	res, err := golang.Collect(root, golang.Config{
		Repo:       "exrepo",
		ObservedAt: time.Unix(1700000000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	repo := memory.New()
	w := Writer{Nodes: repo, Edges: repo.AsEdgeRepo()}
	st, err := w.Apply(context.Background(), res)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if st.Services != 1 || st.Functions != 1 || st.Endpoints != 1 || st.Edges != 2 {
		t.Errorf("stats=%+v", st)
	}

	// Roundtrip: node listing finds them.
	ctx := context.Background()
	list, err := repo.List(ctx, repository.NodeFilter{Kind: node.KindFunction})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("functions list=%d", len(list))
	}

	// Edges out of the function point to the service.
	svcURN := res.Services[0].URN()
	fnURN := res.Functions[0].URN()
	edges, err := repo.Neighbors(ctx, fnURN, repository.DirOut, repository.EdgeFilter{Types: []edge.Type{edge.TypeDefinedIn}})
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if len(edges) != 1 || edges[0].To() != svcURN {
		t.Errorf("edges=%+v", edges)
	}
}

func TestApply_Idempotent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module ex\n")
	writeFile(t, filepath.Join(root, "service", "s.go"), `package service
func DoIt() {}
`)
	res, err := golang.Collect(root, golang.Config{Repo: "ex", ObservedAt: time.Unix(0, 0).UTC()})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	repo := memory.New()
	w := Writer{Nodes: repo, Edges: repo.AsEdgeRepo()}
	if _, err := w.Apply(context.Background(), res); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Apply(context.Background(), res); err != nil {
		t.Fatal(err)
	}
	// Idempotência F-007: URN determinística + GetByURN encontra a versão
	// corrente após múltiplos applies. Memory versiona em cada Upsert
	// (contrato bitemporal); o que garantimos aqui é estabilidade do
	// identificador, não dedup de conteúdo.
	urn := res.Services[0].URN()
	got, err := repo.GetByURN(context.Background(), urn, repository.AsOf{})
	if err != nil || got.URN() != urn {
		t.Errorf("URN missing after re-apply: %v", err)
	}
}
