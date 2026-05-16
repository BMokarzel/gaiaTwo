package rules

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Load lê todos os `.yaml`/`.yml` em `dir` (não-recursivo), valida cada
// um, e retorna `[]Rule` ordenado por `id`. Determinístico — mesma
// pasta produz mesma ordem em runs distintos.
//
// Falha fatal:
//   - dir não existe / não-legível.
//   - YAML mal-formado.
//   - Schema inválido (Validate retorna erro).
//   - Dois arquivos com mesmo `id` (ambiguidade de audit trail).
//
// `dir` vazio retorna `nil, nil` (ok rodar sem regras — gera 0 linhas
// shared, todo `fct_unallocated_cost` continua "remaining").
func Load(dir string) ([]Rule, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("rules: read dir %s: %w", dir, err)
	}

	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	sort.Strings(paths)

	out := make([]Rule, 0, len(paths))
	seen := make(map[string]string, len(paths)) // id → path (para mensagem clara em duplicata)

	for _, p := range paths {
		r, err := loadOne(p)
		if err != nil {
			return nil, err
		}
		if prev, ok := seen[r.ID]; ok {
			return nil, fmt.Errorf("rules: duplicate id %q in %s and %s", r.ID, prev, p)
		}
		seen[r.ID] = p
		out = append(out, r)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func loadOne(path string) (Rule, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Rule{}, fmt.Errorf("rules: read %s: %w", path, err)
	}
	var r Rule
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true) // rejeita campos extra → typo em yaml falha barulhento
	if err := dec.Decode(&r); err != nil {
		return Rule{}, fmt.Errorf("rules: parse %s: %w", path, err)
	}
	if err := r.Validate(); err != nil {
		return Rule{}, fmt.Errorf("rules: %s: %w", path, err)
	}
	return r, nil
}
