package golang

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Discovered descreve um módulo Go encontrado durante o walk.
//
// `RelPath` é o caminho do diretório do `go.mod` relativo à raiz do
// repositório — "." se for a raiz. Esse valor alimenta `ModulePath`
// no `node.Service` (e portanto a URN).
type Discovered struct {
	RelPath  string // "." ou subpath (ex.: "cmd/cli")
	AbsPath  string // caminho absoluto do diretório
	GoModule string // valor da diretiva `module`
}

// WalkOptions controla filtros do walker. Os defaults (zero value) já
// aplicam exclusões padrão: `vendor/`, diretórios começando com `.`.
type WalkOptions struct {
	// ExtraSkipDirs adiciona nomes de diretório a ignorar (match por
	// basename). Ex.: "third_party", "node_modules".
	ExtraSkipDirs []string
}

// Walk percorre `repoRoot` e devolve um `Discovered` por `go.mod`
// encontrado. Ordem é determinística (filepath.WalkDir caminha em ordem
// lexicográfica).
//
// Erros de leitura em arquivos individuais não abortam — o walker
// acumula apenas erros de I/O da raiz; falhas pontuais em parsear um
// `go.mod` são reportadas como erro retornado (decisão MVP: melhor
// falhar cedo do que emitir Service inconsistente).
func Walk(repoRoot string, opts WalkOptions) ([]Discovered, error) {
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("walker: abs repo root: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("walker: stat repo root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("walker: repo root %q is not a directory", absRoot)
	}

	skip := defaultSkipDirs()
	for _, d := range opts.ExtraSkipDirs {
		skip[d] = struct{}{}
	}

	var out []Discovered
	werr := filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			// Nunca pular a própria raiz mesmo se bater o filtro.
			if path == absRoot {
				return nil
			}
			if _, hit := skip[name]; hit {
				return filepath.SkipDir
			}
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" {
			return nil
		}
		mod, perr := parseModuleDirective(path)
		if perr != nil {
			return fmt.Errorf("walker: parse %s: %w", path, perr)
		}
		dir := filepath.Dir(path)
		rel, rerr := filepath.Rel(absRoot, dir)
		if rerr != nil {
			return fmt.Errorf("walker: rel %s: %w", dir, rerr)
		}
		rel = filepath.ToSlash(rel)
		if rel == "" {
			rel = "."
		}
		out = append(out, Discovered{RelPath: rel, AbsPath: dir, GoModule: mod})
		return nil
	})
	if werr != nil {
		return nil, werr
	}
	return out, nil
}

// defaultSkipDirs são os diretórios ignorados sem necessidade de opt-in.
// Vendor está aqui porque cópias de dependências não contam como
// services do repo. `.git`/`.idea`/etc. caem na regra de prefixo ".".
func defaultSkipDirs() map[string]struct{} {
	return map[string]struct{}{
		"vendor":       {},
		"node_modules": {},
	}
}

// parseModuleDirective lê apenas a primeira linha `module <path>` do
// `go.mod`. Não usa `golang.org/x/mod/modfile` para manter o módulo
// sem dependências externas além do stdlib (decisão F-007).
func parseModuleDirective(goModPath string) (string, error) {
	f, err := os.Open(goModPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if !strings.HasPrefix(line, "module") {
			// `go.mod` válido começa com `module <path>`. Se a primeira
			// linha não-vazia/não-comentário não é `module`, considera-se
			// malformado.
			return "", fmt.Errorf("first non-comment line is not `module`: %q", line)
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "module"))
		// Permite forma `module "path"` (raríssimo mas válido).
		rest = strings.Trim(rest, "\"")
		if rest == "" {
			return "", fmt.Errorf("empty module path")
		}
		return rest, nil
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no module directive found")
}
