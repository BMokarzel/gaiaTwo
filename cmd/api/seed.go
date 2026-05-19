// Seed opcional do ce-api: na inicialização, roda o coletor TypeScript
// sobre um path local e grava no repository configurado. Útil em modo
// `memory` (sem Neo4j) para popular o grafo e iterar com a web UI.
//
// Não é caminho de produção — para isso há `ce extract code --neo4j`
// + `ce-api --neo4j` apontando para o mesmo cluster.

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"costEngine/internal/modules/code/collector/golang"
	"costEngine/internal/modules/code/collector/typescript"
	codesvc "costEngine/internal/modules/code/service"
	"costEngine/internal/repository"
)

// seedFromTypeScript roda typescript.Collect + codesvc.Writer.Apply.
// Por enquanto o Writer só persiste Services/Endpoints/Functions e
// edges DefinedIn (vide internal/modules/code/service/service.go).
// Modules/Types/Variables/Calls/Frameworks e demais edges são
// extraídos mas ainda não gravados — gap consciente do MVP.
func seedFromTypeScript(
	ctx context.Context,
	path, repo string,
	nodes repository.NodeRepository,
	edges repository.EdgeRepository,
	logger *slog.Logger,
) error {
	tcfg := typescript.Config{
		Repo:       repo,
		RunID:      fmt.Sprintf("seed-%d", time.Now().Unix()),
		ObservedAt: time.Now().UTC(),
	}
	tres, err := typescript.Collect(ctx, path, tcfg)
	if err != nil {
		if errors.Is(err, typescript.ErrNodeMissing) {
			return fmt.Errorf("%w; install Node.js >= 18", err)
		}
		return fmt.Errorf("typescript.Collect: %w", err)
	}

	res := golang.Result{
		Services:   tres.Services,
		Modules:    tres.Modules,
		Endpoints:  tres.Endpoints,
		Functions:  tres.Functions,
		Frameworks: tres.Frameworks,
		Calls:      tres.Calls,
		Types:      tres.Types,
		Variables:  tres.Variables,
		Contains:   tres.Contains,
		DependsOn:  tres.DependsOn,
		Invokes:    tres.Invokes,
		Targets:    tres.Targets,
		Uses:       tres.Uses,
		Extends:    tres.Extends,
		Aliases:    tres.Aliases,
		DefinedIn:  tres.DefinedIn,
		Edges:      tres.DefinedIn,
	}

	w := codesvc.Writer{Nodes: nodes, Edges: edges}
	st, err := w.Apply(ctx, res)
	if err != nil {
		return fmt.Errorf("writer.Apply: %w", err)
	}
	logger.Info("seed: applied",
		"path", path,
		"repo", repo,
		"services", st.Services,
		"endpoints", st.Endpoints,
		"functions", st.Functions,
		"edges", st.Edges,
		"extracted_modules", len(tres.Modules),
		"extracted_types", len(tres.Types),
		"extracted_calls", len(tres.Calls),
	)
	return nil
}
