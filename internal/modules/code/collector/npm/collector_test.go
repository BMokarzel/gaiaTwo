package npm

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"costEngine/internal/entity/node"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCollect_F023Npm cobre o caminho feliz do collector npm:
// dependencies vs devDependencies → IsDevOnly + Hard correto;
// `node_modules` é skipado; dedup cross-file por URN.
func TestCollect_F023Npm(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{
  "name": "web",
  "dependencies": {
    "react": "^18.2.0",
    "@types/node": "20.0.0"
  },
  "devDependencies": {
    "jest": "^29.0.0"
  }
}`)
	// Sub-package duplicando react (deve dedup) e adicionando vue.
	writeFile(t, filepath.Join(root, "apps", "admin", "package.json"), `{
  "name": "admin",
  "dependencies": {
    "react": "^18.2.0",
    "vue": "^3.4.0"
  }
}`)
	// node_modules tem package.json reais que devem ser ignorados.
	writeFile(t, filepath.Join(root, "node_modules", "lodash", "package.json"), `{
  "name": "lodash",
  "dependencies": {"should-be-ignored": "1.0.0"}
}`)

	svc := node.URN("urn:ce:code:ex:service/web")
	res, err := Collect(root, Config{
		Repo:       "ex/web",
		ServiceURN: svc,
		ObservedAt: time.Unix(0, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// Frameworks únicos: react, @types/node, jest, vue = 4.
	if len(res.Frameworks) != 4 {
		names := []string{}
		for _, f := range res.Frameworks {
			names = append(names, f.Name)
		}
		t.Fatalf("Frameworks=%d want 4: %v", len(res.Frameworks), names)
	}

	byName := map[string]node.Framework{}
	for _, f := range res.Frameworks {
		byName[f.Name] = f
		if f.Ecosystem != "npm" {
			t.Errorf("%s.Ecosystem=%q want npm", f.Name, f.Ecosystem)
		}
	}

	if !byName["jest"].IsDevOnly {
		t.Errorf("jest deveria IsDevOnly=true")
	}
	if byName["react"].IsDevOnly {
		t.Errorf("react não deveria IsDevOnly")
	}
	if byName["should-be-ignored"].Name != "" {
		t.Errorf("dep dentro de node_modules vazou: %v", byName)
	}

	// DEPENDS_ON: 5 (react x2 emite só 1 edge dedup'ada por DeterministicID?
	// Não — cada manifest emite uma edge separada com mesmo ID; o caller
	// resolve dedup no writer. Aqui só conferimos que 5 entradas saíram.
	// react aparece em 2 manifests → 2 edges; outras = 1 cada (jest, @types/node, vue).
	if len(res.DependsOn) != 5 {
		t.Fatalf("DependsOn=%d want 5", len(res.DependsOn))
	}
	for _, d := range res.DependsOn {
		if d.From() != svc {
			t.Errorf("DependsOn.From=%q want %q", d.From(), svc)
		}
	}
}

// TestCollect_NoServiceURN: quando ServiceURN é zero, só Frameworks
// são emitidos; nenhuma edge é gerada.
func TestCollect_NoServiceURN(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{
  "dependencies": {"react": "^18.0.0"}
}`)
	res, err := Collect(root, Config{Repo: "ex"})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(res.Frameworks) != 1 {
		t.Errorf("Frameworks=%d want 1", len(res.Frameworks))
	}
	if len(res.DependsOn) != 0 {
		t.Errorf("DependsOn=%d want 0 (ServiceURN zero)", len(res.DependsOn))
	}
}
