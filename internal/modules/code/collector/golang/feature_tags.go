package golang

import (
	"go/ast"
	"sort"
	"strings"
)

// featureTagPrefix é o token reconhecido em comentários Go que
// pontua um símbolo com uma Feature (ADR-009 / F-028).
//
// Formato suportado:
//
//	// @feature: checkout
//	// @feature: pix, qr-code
//	// @feature(pix)
//
// Tags são `Feature.ShortID` (kebab-case). Não validamos contra o
// catálogo aqui — quem ingere o Result faz reconciliação e emite
// warning para órfãs.
const featureTagPrefix = "@feature"

// extractFeatureTagsFromDoc lê o CommentGroup de doc de uma declaração
// e devolve as tags ordenadas e únicas. Vazio se ausente.
func extractFeatureTagsFromDoc(doc *ast.CommentGroup) []string {
	if doc == nil {
		return nil
	}
	set := map[string]struct{}{}
	for _, c := range doc.List {
		line := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		line = strings.TrimSpace(strings.TrimPrefix(line, "/*"))
		line = strings.TrimSpace(strings.TrimSuffix(line, "*/"))
		idx := strings.Index(line, featureTagPrefix)
		if idx < 0 {
			continue
		}
		rest := line[idx+len(featureTagPrefix):]
		// Suporta `@feature: foo, bar` e `@feature(foo)`.
		rest = strings.TrimSpace(rest)
		switch {
		case strings.HasPrefix(rest, ":"):
			rest = rest[1:]
		case strings.HasPrefix(rest, "("):
			if end := strings.Index(rest, ")"); end >= 0 {
				rest = rest[1:end]
			}
		default:
			continue
		}
		for _, p := range strings.Split(rest, ",") {
			tag := strings.TrimSpace(p)
			if tag == "" {
				continue
			}
			set[tag] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
