package typescript

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// Config controla a coleta sobre um repositório TypeScript.
type Config struct {
	Repo       string    // nome canônico do repo (vai pro slot <account> da URN)
	RunID      string    // identificador da execução
	ObservedAt time.Time // defaults a now se zero

	// SkipDirs adicionais ao default (`node_modules`, `dist`, `build`,
	// `.next`, `.git`, dirs começando com `.`).
	SkipDirs []string

	// Verbose ativa logs textuais detalhados em stderr do sidecar.
	Verbose bool

	// NodePath sobrescreve o binário Node a ser usado (default: "node"
	// no PATH).
	NodePath string

	// EventTimeout é o tempo máximo sem novos eventos antes de abortar.
	// Default: 5min.
	EventTimeout time.Duration
}

// ErrNodeMissing é devolvido quando o sidecar não pôde ser executado
// porque Node.js não está disponível no host.
var ErrNodeMissing = errors.New(
	"typescript collector: Node.js >=18 not found in PATH; install from https://nodejs.org",
)

// ErrSidecarSchema é devolvido quando o sidecar reporta versão de
// schema incompatível.
var ErrSidecarSchema = errors.New("typescript collector: sidecar reported incompatible $schema")

// Collect roda o pipeline completo sobre `repoRoot`:
//
//  1. Garante que o sidecar bundle está extraído (cache em tmpdir).
//  2. Spawn `node <bundle>` com pipes stdin/stdout/stderr.
//  3. Envia ConfigEnvelope no stdin (uma linha JSON).
//  4. Lê NDJSON do stdout até evento `done` ou `error`.
//  5. Decodifica eventos em entidades tipadas.
//
// É idempotente — URNs e IDs de aresta são determinísticos no sidecar.
func Collect(ctx context.Context, repoRoot string, cfg Config) (Result, error) {
	if cfg.Repo == "" {
		return Result{}, fmt.Errorf("typescript.Collect: Repo is required")
	}
	if cfg.ObservedAt.IsZero() {
		cfg.ObservedAt = time.Now().UTC()
	}
	if cfg.EventTimeout == 0 {
		cfg.EventTimeout = 5 * time.Minute
	}

	bundlePath, err := materializeSidecar()
	if err != nil {
		return Result{}, fmt.Errorf("materialize sidecar: %w", err)
	}

	nodeBin := cfg.NodePath
	if nodeBin == "" {
		nodeBin = "node"
	}
	if _, lookupErr := exec.LookPath(nodeBin); lookupErr != nil {
		return Result{}, ErrNodeMissing
	}

	cmd := exec.CommandContext(ctx, nodeBin, bundlePath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Result{}, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Result{}, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start sidecar: %w", err)
	}

	// Drain stderr para evitar deadlock — descartado por default, mas
	// callers em modo `Verbose` podem passar logger no futuro.
	go drainStderr(stderr, cfg.Verbose)

	envelope := ConfigEnvelope{
		Schema:     SchemaVersion,
		Root:       repoRoot,
		Repo:       cfg.Repo,
		RunID:      cfg.RunID,
		ObservedAt: cfg.ObservedAt.UTC().Format(time.RFC3339),
		SkipDirs:   cfg.SkipDirs,
		Verbose:    cfg.Verbose,
	}
	if err := writeConfigLine(stdin, envelope); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return Result{}, fmt.Errorf("send config: %w", err)
	}

	res, decodeErr := decodeStream(stdout, cfg)

	waitErr := cmd.Wait()
	if decodeErr != nil {
		return Result{}, decodeErr
	}
	if waitErr != nil {
		return Result{}, fmt.Errorf("sidecar exited with error: %w", waitErr)
	}
	return res, nil
}

func writeConfigLine(w io.WriteCloser, env ConfigEnvelope) error {
	defer w.Close()
	enc := json.NewEncoder(w)
	return enc.Encode(env)
}

// drainStderr consome stderr do sidecar para evitar bloqueio. Verbose
// não está wireado a logger ainda — captura silenciosamente no MVP.
// Slice futura: expor `Config.Logger`.
func drainStderr(r io.ReadCloser, _ bool) {
	defer r.Close()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1024), 1024*1024)
	for sc.Scan() {
		_ = sc.Text() // descartado
	}
}
