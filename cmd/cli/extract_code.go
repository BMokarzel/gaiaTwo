package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"costEngine/internal/modules/code/collector/golang"
	"costEngine/internal/modules/code/collector/typescript"
	codesvc "costEngine/internal/modules/code/service"
	"costEngine/internal/repository"
	"costEngine/internal/repository/memory"
	"costEngine/internal/repository/n4j"
)

// languageMode controla o dispatcher por linguagem do collector.
type languageMode string

const (
	langAuto       languageMode = "auto"
	langGo         languageMode = "go"
	langTypeScript languageMode = "typescript"
)

// runExtractCode é o handler de `ce extract code` (F-007 S-007).
//
// Pipeline:
//  1. golang.Collect(repoRoot) → walk + extract.
//  2. codesvc.Writer.Apply → grava Services/Endpoints/Functions/Edges.
//
// Backend:
//   - Sem `--neo4j`: usa `repository.memory` (visualizar apenas) — útil
//     em CI e desenvolvimento local. Equivalente a `--dry-run` se você
//     descarta o processo no fim.
//   - Com `--neo4j`: grava no grafo persistente.
//
// `--dry-run` roda Collect mas não chama Writer.Apply.
func runExtractCode(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("extract code", flag.ContinueOnError)
	repoName := fs.String("repo", "", "nome canônico do repositório (vai pro slot <account> da URN) — obrigatório")
	path := fs.String("path", ".", "caminho local do repositório")
	neoURI := fs.String("neo4j", "", "URI bolt:// do Neo4j (opcional; vazio → backend memory in-process)")
	neoUser := fs.String("neo4j-user", "", "usuário Neo4j")
	neoPass := fs.String("neo4j-pass", "", "senha Neo4j")
	neoDB := fs.String("neo4j-db", "", "database Neo4j")
	dryRun := fs.Bool("dry-run", false, "extrai mas não escreve no repository")
	runID := fs.String("run-id", "", "identificador da execução (default: timestamp)")
	lang := fs.String("lang", "auto", "linguagem do collector: auto|go|typescript")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repoName == "" {
		return fmt.Errorf("%w: --repo obrigatório", errBadUsage)
	}

	rid := *runID
	if rid == "" {
		rid = fmt.Sprintf("code-%d", time.Now().Unix())
	}

	mode, err := resolveLanguage(languageMode(*lang), *path)
	if err != nil {
		return err
	}

	var res golang.Result
	switch mode {
	case langGo:
		cfg := golang.Config{Repo: *repoName, RunID: rid, ObservedAt: time.Now().UTC()}
		res, err = golang.Collect(*path, cfg)
		if err != nil {
			return fmt.Errorf("collect (go): %w", err)
		}
	case langTypeScript:
		tcfg := typescript.Config{Repo: *repoName, RunID: rid, ObservedAt: time.Now().UTC()}
		tres, terr := typescript.Collect(ctx, *path, tcfg)
		if terr != nil {
			if errors.Is(terr, typescript.ErrNodeMissing) {
				return fmt.Errorf("collect (typescript): %w; instale Node.js >= 18 e exponha no PATH", terr)
			}
			return fmt.Errorf("collect (typescript): %w", terr)
		}
		res = typescriptToGolang(tres)
	default:
		return fmt.Errorf("%w: --lang inválido (%q); use auto|go|typescript", errBadUsage, *lang)
	}

	fmt.Printf("extract code: repo=%s path=%s lang=%s run=%s\n", *repoName, *path, mode, rid)
	fmt.Printf("  Services:   %d\n", len(res.Services))
	fmt.Printf("  Modules:    %d\n", len(res.Modules))
	fmt.Printf("  Endpoints:  %d\n", len(res.Endpoints))
	fmt.Printf("  Functions:  %d\n", len(res.Functions))
	fmt.Printf("  Types:      %d\n", len(res.Types))
	fmt.Printf("  Variables:  %d\n", len(res.Variables))
	fmt.Printf("  Frameworks: %d\n", len(res.Frameworks))
	fmt.Printf("  Calls:      %d\n", len(res.Calls))
	fmt.Printf("  Edges:      %d (DEFINED_IN legado)\n", len(res.Edges))

	if *dryRun {
		fmt.Println("dry-run: nada gravado")
		return nil
	}

	var (
		nodeRepo repository.NodeRepository
		edgeRepo repository.EdgeRepository
	)
	if *neoURI == "" {
		m := memory.New()
		nodeRepo, edgeRepo = m, m.AsEdgeRepo()
		fmt.Println("backend: memory (in-process — nada persiste após o processo)")
	} else {
		nc, err := n4j.Connect(ctx, n4j.Config{
			URI: *neoURI, Username: *neoUser, Password: *neoPass, Database: *neoDB,
		})
		if err != nil {
			return fmt.Errorf("neo4j connect: %w", err)
		}
		defer nc.Close(ctx)
		if err := nc.Migrate(ctx); err != nil {
			return fmt.Errorf("neo4j migrate: %w", err)
		}
		nodeRepo = n4j.NewNodeRepo(nc)
		edgeRepo = n4j.NewEdgeRepo(nc)
	}

	w := codesvc.Writer{Nodes: nodeRepo, Edges: edgeRepo}
	st, err := w.Apply(ctx, res)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	fmt.Printf("ok: gravado services=%d endpoints=%d functions=%d edges=%d\n",
		st.Services, st.Endpoints, st.Functions, st.Edges)
	return nil
}

// resolveLanguage decide qual collector usar. Em `auto`, inspeciona
// manifests no root:
//   - go.mod                  → Go
//   - tsconfig.json | package.json (qualquer subdir até 2 níveis) → TypeScript
//   - ambos → Go (precedência histórica; rodar TS exige --lang=typescript explícito)
//   - nenhum → erro
func resolveLanguage(requested languageMode, root string) (languageMode, error) {
	switch requested {
	case langGo, langTypeScript:
		return requested, nil
	case langAuto, "":
		hasGo := fileExists(filepath.Join(root, "go.mod"))
		hasTS := manifestExists(root, "tsconfig.json") || manifestExists(root, "package.json")
		switch {
		case hasGo:
			return langGo, nil
		case hasTS:
			return langTypeScript, nil
		default:
			return "", fmt.Errorf("%w: não consegui detectar a linguagem em %s; passe --lang=go|typescript", errBadUsage, root)
		}
	default:
		return "", fmt.Errorf("%w: --lang inválido (%q)", errBadUsage, requested)
	}
}

// manifestExists checa root e até 2 níveis de subdirs (cobre monorepos
// com `apps/<svc>/package.json` ou `packages/<x>/tsconfig.json`).
func manifestExists(root, name string) bool {
	if fileExists(filepath.Join(root, name)) {
		return true
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "node_modules" || e.Name() == ".git" {
			continue
		}
		sub := filepath.Join(root, e.Name())
		if fileExists(filepath.Join(sub, name)) {
			return true
		}
		// 2º nível
		subs, err := os.ReadDir(sub)
		if err != nil {
			continue
		}
		for _, s := range subs {
			if !s.IsDir() {
				continue
			}
			if fileExists(filepath.Join(sub, s.Name(), name)) {
				return true
			}
		}
	}
	return false
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// typescriptToGolang converte o subset suportado pelo Writer atual
// (Services/Endpoints/Functions/Edges). Modules, Frameworks, Calls,
// Types, Variables e demais edges ainda não são persistidos — slice
// futura no Writer (S-030.x). O dispatcher mantém o formato para que o
// usuário veja os contadores no console; quando o Writer for estendido,
// basta encaminhar os campos extras.
func typescriptToGolang(r typescript.Result) golang.Result {
	return golang.Result{
		Services:   r.Services,
		Modules:    r.Modules,
		Endpoints:  r.Endpoints,
		Functions:  r.Functions,
		Frameworks: r.Frameworks,
		Calls:      r.Calls,
		Types:      r.Types,
		Variables:  r.Variables,
		Contains:   r.Contains,
		DependsOn:  r.DependsOn,
		Invokes:    r.Invokes,
		Targets:    r.Targets,
		Uses:       r.Uses,
		Extends:    r.Extends,
		Aliases:    r.Aliases,
		DefinedIn:  r.DefinedIn,
		Edges:      r.DefinedIn,
	}
}
