// Command ce-api expõe a REST /v1/architecture/* (F-014).
//
// Uso:
//
//	ce-api --addr=:8080 [--neo4j=<uri>] [--neo4j-user=<u>]
//	       [--neo4j-pass=<p>] [--neo4j-db=<d>]
//	       [--tenant-required]
//
// Sem --neo4j → backend memory (vazio; útil pra smoke local).
// Com --neo4j → conecta e roda migrations idempotentes (constraints).
//
// Sinal: respeita SIGINT/SIGTERM e faz shutdown graceful em até 10s.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	graphctrl "costEngine/internal/app/graph/controller"
	"costEngine/internal/app/search"
	searchctrl "costEngine/internal/app/search/controller"
	"costEngine/internal/entity/node"
	orgctrl "costEngine/internal/modules/org/controller"
	orgservice "costEngine/internal/modules/org/service"
	"costEngine/internal/platform/httpserver"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
	"costEngine/internal/repository/n4j"
)

// searchAdapter satisfaz `search.NodeSearcher` traduzindo entre o
// SearchQuery do port (consumer-owned) e o do repository. É um
// wrapper trivial — vive aqui no assembler para que o package
// `app/search` não importe `repository`.
type searchAdapter struct{ r repository.NodeRepository }

func (a searchAdapter) Search(ctx context.Context, q search.SearchQuery) ([]node.Node, error) {
	return a.r.Search(ctx, repository.SearchQuery{
		Q: q.Q, Kind: q.Kind, Limit: q.Limit, Offset: q.Offset,
	})
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("ce-api", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "endereço HTTP (host:port)")
	neoURI := fs.String("neo4j", "", "URI Neo4j (ex.: bolt://localhost:7687); vazio → memory")
	neoUser := fs.String("neo4j-user", "neo4j", "usuário Neo4j")
	neoPass := fs.String("neo4j-pass", "", "senha Neo4j")
	neoDB := fs.String("neo4j-db", "neo4j", "database Neo4j")
	tenantRequired := fs.Bool("tenant-required", false, "se true, exige X-Tenant-ID em toda request (401 sem ele)")
	readTimeout := fs.Duration("read-timeout", 10*time.Second, "http.Server.ReadHeaderTimeout")
	writeTimeout := fs.Duration("write-timeout", 30*time.Second, "http.Server.WriteTimeout")
	idleTimeout := fs.Duration("idle-timeout", 120*time.Second, "http.Server.IdleTimeout")
	shutdownGrace := fs.Duration("shutdown-grace", 10*time.Second, "tempo de graceful shutdown após sinal")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	nodes, edges, closeRepo, err := buildRepos(ctx, *neoURI, *neoUser, *neoPass, *neoDB, logger)
	if err != nil {
		return fmt.Errorf("repo: %w", err)
	}
	defer closeRepo()

	orgSvc := orgservice.New(nodes, edges)
	orgCtrl := orgctrl.New(orgSvc)
	graphCtrl := graphctrl.New(nodes, edges)
	searchCtrl := searchctrl.New(searchAdapter{r: nodes})

	apiSrv := httpserver.New(httpserver.Config{
		TenantRequired: *tenantRequired,
		Logger:         logger,
	}, orgCtrl, graphCtrl, searchCtrl)

	hs := &http.Server{
		Addr:              *addr,
		Handler:           apiSrv.Routes(),
		ReadHeaderTimeout: *readTimeout,
		WriteTimeout:      *writeTimeout,
		IdleTimeout:       *idleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening",
			"addr", *addr,
			"tenant_required", *tenantRequired,
			"backend", repoBackendName(*neoURI),
		)
		if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	}

	shutCtx, shutCancel := context.WithTimeout(context.Background(), *shutdownGrace)
	defer shutCancel()
	if err := hs.Shutdown(shutCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("shutdown complete")
	return nil
}

// buildRepos escolhe memory vs n4j com base em --neo4j. Devolve um
// closer no-op para memory e Close(ctx) do client para n4j.
func buildRepos(
	ctx context.Context,
	uri, user, pass, db string,
	logger *slog.Logger,
) (repository.NodeRepository, repository.EdgeRepository, func(), error) {
	if uri == "" {
		logger.Info("backend: memory (in-process; data is ephemeral)")
		m := memory.New()
		return m, m.AsEdgeRepo(), func() {}, nil
	}
	nc, err := n4j.Connect(ctx, n4j.Config{
		URI: uri, Username: user, Password: pass, Database: db,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("neo4j connect: %w", err)
	}
	if err := nc.Migrate(ctx); err != nil {
		_ = nc.Close(ctx)
		return nil, nil, nil, fmt.Errorf("neo4j migrate: %w", err)
	}
	logger.Info("backend: neo4j", "uri", uri, "database", db)
	return n4j.NewNodeRepo(nc), n4j.NewEdgeRepo(nc), func() { _ = nc.Close(ctx) }, nil
}

func repoBackendName(uri string) string {
	if uri == "" {
		return "memory"
	}
	return "neo4j"
}
