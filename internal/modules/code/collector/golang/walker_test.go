package golang

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestWalk_FindsRootAndSubmodules(t *testing.T) {
	root := t.TempDir()

	mustWrite(t, filepath.Join(root, "go.mod"), "module example.com/repo\n\ngo 1.22\n")
	mustWrite(t, filepath.Join(root, "cmd", "cli", "go.mod"), "module example.com/repo/cmd/cli\n")
	mustWrite(t, filepath.Join(root, "internal", "api", "go.mod"), "module example.com/repo/internal/api\n")

	// Ruídos que devem ser ignorados.
	mustWrite(t, filepath.Join(root, "vendor", "x", "go.mod"), "module vendored/x\n")
	mustWrite(t, filepath.Join(root, ".cache", "go.mod"), "module cached/x\n")
	mustWrite(t, filepath.Join(root, "node_modules", "y", "go.mod"), "module node/y\n")

	got, err := Walk(root, WalkOptions{})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	paths := make([]string, 0, len(got))
	mods := map[string]string{}
	for _, d := range got {
		paths = append(paths, d.RelPath)
		mods[d.RelPath] = d.GoModule
	}
	sort.Strings(paths)

	want := []string{".", "cmd/cli", "internal/api"}
	if len(paths) != len(want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
	for i, p := range want {
		if paths[i] != p {
			t.Errorf("paths[%d]=%q want %q", i, paths[i], p)
		}
	}
	if mods["."] != "example.com/repo" {
		t.Errorf("root module=%q", mods["."])
	}
	if mods["cmd/cli"] != "example.com/repo/cmd/cli" {
		t.Errorf("cmd/cli module=%q", mods["cmd/cli"])
	}
}

func TestWalk_ExtraSkipDirs(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module a\n")
	mustWrite(t, filepath.Join(root, "third_party", "x", "go.mod"), "module x\n")

	got, err := Walk(root, WalkOptions{ExtraSkipDirs: []string{"third_party"}})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(got) != 1 || got[0].RelPath != "." {
		t.Errorf("got %+v", got)
	}
}

func TestWalk_NotADirectory(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "file.txt")
	mustWrite(t, f, "x")
	if _, err := Walk(f, WalkOptions{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseModuleDirective_Variants(t *testing.T) {
	cases := []struct {
		name, body, want string
		wantErr          bool
	}{
		{"plain", "module foo\n", "foo", false},
		{"with-comment", "// hello\nmodule foo/bar\n", "foo/bar", false},
		{"quoted", "module \"foo\"\n", "foo", false},
		{"empty", "// only comment\n", "", true},
		{"no-module-first", "go 1.22\nmodule foo\n", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "go.mod")
			mustWrite(t, p, tc.body)
			got, err := parseModuleDirective(p)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want err, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
