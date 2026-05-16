package codeowners

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Rule é uma regra do arquivo CODEOWNERS após o parse.
//
// `Owners` preserva os handles na forma original (com prefixo `@`) — o
// resolver downstream normaliza para lookup no grafo. Isso mantém o
// parser puro: sem depender do estado do grafo.
type Rule struct {
	LineNum int      // número da linha original (1-indexado)
	Pattern string   // ex.: "*", "/docs/", "*.go"
	Owners  []string // ex.: ["@alice", "@org/payments"]
}

// IsGlobal indica se a regra é a global (`*`), única suportada no MVP
// para emissão de `Owns`. Outras regras passam pelo parser para futura
// extensão mas o engine atual ignora.
func (r Rule) IsGlobal() bool { return r.Pattern == "*" }

// RowError sinaliza falha em uma linha. Não-fatal — o parse continua e
// devolve o erro acumulado para o caller relatar.
type RowError struct {
	LineNum int
	Field   string
	Message string
}

func (e RowError) Error() string {
	return fmt.Sprintf("line %d: %s: %s", e.LineNum, e.Field, e.Message)
}

// Parse lê um CODEOWNERS de `r` e devolve (regras válidas, erros por
// linha, erro fatal). Erro fatal só em I/O — sintaxe inválida vira
// RowError.
//
// Limites: arquivos absurdamente grandes (>1 MiB de linha) estouram via
// bufio.Scanner por padrão; consumidores podem ajustar com bufio.NewReader
// + Scanner.Buffer caso necessário (não esperado para CODEOWNERS reais).
func Parse(r io.Reader) ([]Rule, []RowError, error) {
	sc := bufio.NewScanner(r)

	var rules []Rule
	var errs []RowError
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Text()
		clean := stripComment(raw)
		fields := strings.Fields(clean)
		if len(fields) == 0 {
			continue // linha em branco ou só comentário
		}
		if len(fields) == 1 {
			errs = append(errs, RowError{
				LineNum: line, Field: "owners",
				Message: fmt.Sprintf("rule %q has no owners", fields[0]),
			})
			continue
		}
		// Valida que os tokens após o pattern são handles plausíveis.
		// Tolerância: aceitamos qualquer token começando com `@` — a
		// validação semântica (handle existe? team existe?) é do
		// resolver no S-003.
		pattern := fields[0]
		owners := fields[1:]
		invalid := false
		for _, o := range owners {
			if !strings.HasPrefix(o, "@") {
				errs = append(errs, RowError{
					LineNum: line, Field: "owner",
					Message: fmt.Sprintf("owner %q must start with @", o),
				})
				invalid = true
				break
			}
		}
		if invalid {
			continue
		}
		rules = append(rules, Rule{LineNum: line, Pattern: pattern, Owners: owners})
	}
	if err := sc.Err(); err != nil {
		return rules, errs, fmt.Errorf("codeowners: read: %w", err)
	}
	return rules, errs, nil
}

// stripComment remove o trecho `# ...` da linha. Não suporta `#`
// dentro de strings ou paths — CODEOWNERS não tem strings.
func stripComment(s string) string {
	if before, _, ok := strings.Cut(s, "#"); ok {
		return before
	}
	return s
}

// GlobalRule devolve a regra `*` se existir (a última prevalece, como
// no GitHub para o mesmo pattern), ou (Rule{}, false).
func GlobalRule(rules []Rule) (Rule, bool) {
	var found Rule
	ok := false
	for _, r := range rules {
		if r.IsGlobal() {
			found = r
			ok = true
		}
	}
	return found, ok
}
