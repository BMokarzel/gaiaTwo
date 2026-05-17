// Package proto coleta `node.Schema` a partir de arquivos `.proto`
// (Protobuf v2/v3). Sintaxe é parseada por linha — não é um compilador,
// mas cobre o suficiente para extrair:
//
//   - `package` directive  → Schema.Namespace
//   - `message Foo { ... }` (com aninhamento)  → Schema (Format=proto)
//   - Campos `[modifier] <type> <name> = <num>;` → Schema.Fields
//
// Limitações conhecidas (MVP, F-022):
//   - `oneof` é planificado (fields do oneof entram no parent).
//   - `enum`, `service`, `rpc` ignorados.
//   - `map<K,V>` representado como `map<K,V>` literal no TypeRef.
//   - `imports` ignorados (não resolvemos refs cross-file ainda).
//
// Confidence: 1.0 — a sintaxe Protobuf é declarativa e determinística.
package proto

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"costEngine/internal/entity/node"
)

// Config controla a execução.
type Config struct {
	Repo              string    // ex: github.com/acme/payments
	ServiceModulePath string    // "." ou subpath; "" → "."
	ServiceURN        node.URN  // pode ser vazia para schemas globais
	RunID             string
	ObservedAt        time.Time
}

// Result agrega o que `Collect` produz.
type Result struct {
	Schemas []node.Schema
}

// Collect percorre `root` à procura de `*.proto`. Idempotente.
func Collect(root string, cfg Config) (Result, error) {
	now := cfg.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if cfg.ServiceModulePath == "" {
		cfg.ServiceModulePath = "."
	}

	var out Result
	werr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "node_modules" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".proto") {
			return nil
		}
		f, oerr := os.Open(path)
		if oerr != nil {
			return nil
		}
		defer f.Close()
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		schemas, perr := parseFile(f, rel, cfg, now)
		if perr != nil {
			return fmt.Errorf("parse %s: %w", path, perr)
		}
		out.Schemas = append(out.Schemas, schemas...)
		return nil
	})
	if werr != nil {
		return Result{}, werr
	}
	return out, nil
}

// parseFile lê o conteúdo de um `.proto` linha-a-linha mantendo um
// stack de `message` para suportar aninhamento. Devolve um Schema por
// message (com fields planos).
func parseFile(r io.Reader, relFile string, cfg Config, now time.Time) ([]node.Schema, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var (
		pkgName string
		out     []node.Schema
		stack   []*node.Schema // pilha de messages abertas (top é current)
	)

	for scanner.Scan() {
		line := stripComment(scanner.Text())
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// `package <name>;`
		if strings.HasPrefix(line, "package") {
			rest := strings.TrimPrefix(line, "package")
			rest = strings.TrimSpace(rest)
			rest = strings.TrimSuffix(rest, ";")
			pkgName = strings.TrimSpace(rest)
			continue
		}

		// `}` fecha o message corrente.
		if line == "}" {
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				out = append(out, *top)
				stack = stack[:len(stack)-1]
			}
			continue
		}

		// `message Foo {` (com chave na mesma linha ou na próxima).
		if strings.HasPrefix(line, "message ") {
			name := messageName(line)
			if name == "" {
				continue
			}
			// Para suportar nested: prefixa com parent.
			qualified := name
			if len(stack) > 0 {
				qualified = stack[len(stack)-1].Symbol + "." + name
			}
			urn := node.NewSchemaURN(cfg.Repo, cfg.ServiceModulePath, qualified)
			s := node.Schema{
				Base: node.Base{
					NodeURN:  urn,
					NodeKind: node.KindSchema,
					NodeMeta: node.Meta{
						Version:    1,
						ValidFrom:  now,
						ObservedAt: now,
						Source: node.Source{
							Collector: "code/proto",
							RunID:     cfg.RunID,
							Method:    node.MethodDeclared,
						},
						Confidence: 1.0,
					},
				},
				ServiceURN: cfg.ServiceURN,
				Format:     node.SchemaProto,
				Namespace:  pkgName,
				Symbol:     qualified,
				SourceFile: relFile,
			}
			stack = append(stack, &s)
			continue
		}

		// Field line dentro de um message: `[mod] <type> <name> = <num>;`
		if len(stack) > 0 {
			if f, ok := parseField(line); ok {
				top := stack[len(stack)-1]
				f.Position = len(top.Fields)
				top.Fields = append(top.Fields, f)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	// Mensagens abertas sem `}` — proto malformado; ignora para não
	// abortar a coleta. Resto do repo segue.
	return out, nil
}

// messageName extrai "Foo" de `message Foo {` ou `message Foo`.
func messageName(line string) string {
	rest := strings.TrimPrefix(line, "message ")
	rest = strings.TrimSpace(rest)
	if i := strings.IndexAny(rest, " \t{"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// parseField tenta extrair `[mod] <type> <name> = <num>;`. Devolve
// false se a linha não bater (e.g., `option (foo) = bar;` ou
// `reserved 5, 6;`).
func parseField(line string) (node.FieldSlot, bool) {
	if !strings.HasSuffix(line, ";") {
		return node.FieldSlot{}, false
	}
	// Ignora directives que não são fields. Sufixo " " garante
	// word-boundary (evita que "option" engula "optional").
	for _, kw := range []string{"option ", "option(", "reserved ", "import ", "syntax ", "extensions ", "enum ", "service ", "rpc "} {
		if strings.HasPrefix(line, kw) {
			return node.FieldSlot{}, false
		}
	}
	body := strings.TrimSuffix(line, ";")
	// Divide na "=" (separador field-number).
	eq := strings.LastIndex(body, "=")
	if eq < 0 {
		return node.FieldSlot{}, false
	}
	head := strings.TrimSpace(body[:eq])
	if head == "" {
		return node.FieldSlot{}, false
	}
	toks := splitFields(head)
	if len(toks) < 2 {
		return node.FieldSlot{}, false
	}
	// Modifier opcional ("repeated" / "optional" / "required").
	switch toks[0] {
	case "repeated", "optional", "required":
		toks = toks[1:]
	}
	if len(toks) < 2 {
		return node.FieldSlot{}, false
	}
	typeRef := strings.Join(toks[:len(toks)-1], " ")
	name := toks[len(toks)-1]
	return node.FieldSlot{Name: name, TypeRef: typeRef}, true
}

// splitFields divide preservando `map<K,V>` como um token único.
func splitFields(s string) []string {
	var toks []string
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '<':
			depth++
			b.WriteRune(r)
		case '>':
			depth--
			b.WriteRune(r)
		case ' ', '\t':
			if depth == 0 {
				if b.Len() > 0 {
					toks = append(toks, b.String())
					b.Reset()
				}
			} else {
				b.WriteRune(r)
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		toks = append(toks, b.String())
	}
	return toks
}

// stripComment remove `//...` no fim da linha. Comentários `/* ... */`
// multi-line não são suportados pelo MVP (raríssimos em .proto).
func stripComment(line string) string {
	if i := strings.Index(line, "//"); i >= 0 {
		return line[:i]
	}
	return line
}
