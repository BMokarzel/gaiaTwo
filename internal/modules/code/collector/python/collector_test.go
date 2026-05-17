package python

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

// TestParseRequirementLine cobre as variantes mais comuns.
func TestParseRequirementLine(t *testing.T) {
	cases := []struct {
		in        string
		wantName  string
		wantVer   string
		wantOK    bool
	}{
		{"django==4.2.0", "django", "==4.2.0", true},
		{"Django>=4,<5", "django", ">=4,<5", true},
		{"requests", "requests", "", true},
		{"requests[security]==2.31.0", "requests", "==2.31.0", true},
		{"  numpy~=1.24  ", "numpy", "~=1.24", true},
		{"flask; python_version >= '3.8'", "flask", "", true},
		{"some_pkg.name==1.0", "some-pkg-name", "==1.0", true},
		{"", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			d, ok := parseRequirementLine(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v", ok, tc.wantOK)
			}
			if ok && (d.Name != tc.wantName || d.Version != tc.wantVer) {
				t.Errorf("(%q,%q) want (%q,%q)", d.Name, d.Version, tc.wantName, tc.wantVer)
			}
		})
	}
}

// TestCollect_F023Python valida o caminho feliz: dedup, DevFiles,
// directives ignoradas (`-r`, `-e`, URLs), edges DEPENDS_ON.
func TestCollect_F023Python(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "requirements.txt"), `
# core deps
django==4.2.0
requests>=2.28
numpy

-r dev-requirements.txt
-e .
git+https://github.com/foo/bar.git
`)
	writeFile(t, filepath.Join(root, "requirements-dev.txt"), `
pytest==8.0.0
django==4.2.0  # já em runtime — dedup
`)
	// .venv deve ser ignorado.
	writeFile(t, filepath.Join(root, ".venv", "lib", "requirements.txt"), "should-be-ignored==1.0\n")

	svc := node.URN("urn:ce:code:ex:service/api")
	res, err := Collect(root, Config{
		Repo:       "ex/api",
		ServiceURN: svc,
		ObservedAt: time.Unix(0, 0).UTC(),
		DevFiles:   []string{"requirements-dev.txt"},
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// Frameworks únicos: django, requests, numpy, pytest = 4.
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
		if f.Ecosystem != "pypi" {
			t.Errorf("%s.Ecosystem=%q want pypi", f.Name, f.Ecosystem)
		}
	}
	if byName["should-be-ignored"].Name != "" {
		t.Errorf(".venv vazou: %v", byName)
	}
	if !byName["pytest"].IsDevOnly {
		t.Errorf("pytest deveria IsDevOnly=true (DevFiles)")
	}
	// WalkDir é alfabético → `requirements-dev.txt` é lido ANTES de
	// `requirements.txt`. django aparece primeiro como dev, depois é
	// dedup'do em runtime. Documenta a semântica "first-seen wins".
	if !byName["django"].IsDevOnly {
		t.Errorf("django foi visto primeiro em dev file; dedup deveria preservar IsDevOnly=true")
	}

	// DependsOn: pytest, django(dev), django(runtime), requests, numpy = 5
	// (django emite uma edge por aparição — dedup é responsabilidade do writer).
	if len(res.DependsOn) != 5 {
		t.Fatalf("DependsOn=%d want 5", len(res.DependsOn))
	}
}
