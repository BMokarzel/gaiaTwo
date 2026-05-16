package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/org/ingest/codeowners"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
	"costEngine/internal/repository/n4j"
)

// runExtractCodeowners é o handler de `ce extract codeowners` (F-011).
//
// Pipeline:
//  1. Localiza o CODEOWNERS dentro do `--repo` (CODEOWNERS, .github/,
//     docs/).
//  2. Parse → Rules + RowErrors (não-fatal).
//  3. Pega a regra global (`*`) — única suportada no MVP.
//  4. Resolve owners contra o grafo (Person via github_handle, Team via
//     nome).
//  5. Mapeia o repo para a Service URN alvo.
//  6. Build → Result; Writer.Apply → grafo.
//  7. Imprime sumário JSON.
//
// `--dry-run` pula a fase de write.
func runExtractCodeowners(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("extract codeowners", flag.ContinueOnError)
	repoPath := fs.String("repo", "", "caminho local do repo — obrigatório")
	tenant := fs.String("tenant", "", "tenant id (slot <account> da URN) — obrigatório")
	neoURI := fs.String("neo4j", "", "URI bolt:// do Neo4j (vazio → memory in-process)")
	neoUser := fs.String("neo4j-user", "", "usuário Neo4j")
	neoPass := fs.String("neo4j-pass", "", "senha Neo4j")
	neoDB := fs.String("neo4j-db", "", "database Neo4j")
	dryRun := fs.Bool("dry-run", false, "parseia + resolve mas não escreve")
	runID := fs.String("run-id", "", "identificador da execução (default: timestamp)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repoPath == "" {
		return fmt.Errorf("%w: --repo obrigatório", errBadUsage)
	}
	if *tenant == "" {
		return fmt.Errorf("%w: --tenant obrigatório", errBadUsage)
	}

	ownersPath, err := locateCodeowners(*repoPath)
	if err != nil {
		return err
	}
	f, err := os.Open(ownersPath)
	if err != nil {
		return fmt.Errorf("open codeowners: %w", err)
	}
	defer f.Close()

	rules, rowErrs, err := codeowners.Parse(f)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	if len(rowErrs) > 0 {
		fmt.Fprintf(os.Stderr, "row errors (%d):\n", len(rowErrs))
		for _, e := range rowErrs {
			fmt.Fprintf(os.Stderr, "  %s\n", e.Error())
		}
	}
	global, ok := codeowners.GlobalRule(rules)
	if !ok {
		return fmt.Errorf("codeowners: no global `*` rule found in %s", ownersPath)
	}

	nodeRepo, edgeRepo, closeRepo, err := buildRepos(ctx, *neoURI, *neoUser, *neoPass, *neoDB)
	if err != nil {
		return err
	}
	defer closeRepo()

	res := &codeowners.Resolver{Nodes: nodeRepo, Tenant: *tenant}
	resolved, unresolved, err := res.Resolve(ctx, global.Owners)
	if err != nil {
		return fmt.Errorf("resolve: %w", err)
	}

	lookup := &codeowners.ServiceLookup{Nodes: nodeRepo}
	resolution, err := lookup.ResolveForRepo(ctx, *repoPath)
	if err != nil && !errors.Is(err, codeowners.ErrServiceNotFound) {
		return err
	}
	summary := map[string]any{
		"repo":        resolution.Repo,
		"owners_path": ownersPath,
		"rules_total": len(rules),
		"resolved":    ownersJSON(resolved),
		"unresolved":  unresolved,
		"row_errors":  len(rowErrs),
	}
	if errors.Is(err, codeowners.ErrServiceNotFound) {
		summary["service"] = "not_found"
		return emitSummary(summary, errPartial)
	}
	if len(resolution.Ambiguous) > 0 {
		summary["service"] = "ambiguous"
		summary["ambiguous_urns"] = stringifyURNs(resolution.Ambiguous)
		return emitSummary(summary, errPartial)
	}
	summary["service"] = string(resolution.Found)

	rid := *runID
	if rid == "" {
		rid = fmt.Sprintf("codeowners-%d", time.Now().Unix())
	}
	emit := codeowners.Build(resolution.Found, resolved, unresolved,
		codeowners.EmitOptions{Tenant: *tenant, RunID: rid, ObservedAt: time.Now().UTC()})

	if *dryRun {
		summary["dry_run"] = true
		summary["candidate_edges"] = len(emit.Owns)
		return emitSummary(summary, nil)
	}

	w := &codeowners.Writer{Nodes: nodeRepo, Edges: edgeRepo}
	st, err := w.Apply(ctx, emit)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	summary["opened"] = st.Opened
	summary["closed"] = st.Closed
	summary["unchanged"] = st.Unchanged
	return emitSummary(summary, nil)
}

// locateCodeowners busca o arquivo nos paths comuns. Devolve erro se
// nenhum existe.
func locateCodeowners(repoPath string) (string, error) {
	candidates := []string{
		filepath.Join(repoPath, "CODEOWNERS"),
		filepath.Join(repoPath, ".github", "CODEOWNERS"),
		filepath.Join(repoPath, "docs", "CODEOWNERS"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("codeowners: file not found in %s (looked in CODEOWNERS, .github/, docs/)", repoPath)
}

func buildRepos(
	ctx context.Context, neoURI, neoUser, neoPass, neoDB string,
) (repository.NodeRepository, repository.EdgeRepository, func(), error) {
	if neoURI == "" {
		m := memory.New()
		return m, m.AsEdgeRepo(), func() {}, nil
	}
	nc, err := n4j.Connect(ctx, n4j.Config{
		URI: neoURI, Username: neoUser, Password: neoPass, Database: neoDB,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("neo4j connect: %w", err)
	}
	if err := nc.Migrate(ctx); err != nil {
		_ = nc.Close(ctx)
		return nil, nil, nil, fmt.Errorf("neo4j migrate: %w", err)
	}
	return n4j.NewNodeRepo(nc), n4j.NewEdgeRepo(nc), func() { _ = nc.Close(ctx) }, nil
}

func ownersJSON(rs []codeowners.ResolvedOwner) []map[string]string {
	out := make([]map[string]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, map[string]string{
			"handle": r.Handle,
			"urn":    string(r.URN),
			"kind":   string(r.Kind),
		})
	}
	return out
}

func stringifyURNs(urns []node.URN) []string {
	out := make([]string, 0, len(urns))
	for _, u := range urns {
		out = append(out, string(u))
	}
	return out
}

// emitSummary serializa `summary` como JSON pretty-printed em stdout e
// devolve `passthrough` (permite a função-handler combinar relatório
// com errPartial / nil sem duplicar o print).
func emitSummary(summary map[string]any, passthrough error) error {
	b, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal summary: %w", err)
	}
	fmt.Println(string(b))
	return passthrough
}
