package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunExtractCode_RequiresRepo(t *testing.T) {
	err := runExtractCode(context.Background(), []string{"--path=."})
	if err == nil || !strings.Contains(err.Error(), "--repo") {
		t.Errorf("expected --repo error, got %v", err)
	}
}

func TestRunExtractCode_DryRunMemorySmoke(t *testing.T) {
	dir := t.TempDir()
	mustWriteCLI(t, filepath.Join(dir, "go.mod"), "module ex\n")
	mustWriteCLI(t, filepath.Join(dir, "service", "s.go"), `package service
func DoX() {}
`)
	err := runExtractCode(context.Background(), []string{
		"--repo=ex", "--path=" + dir, "--dry-run",
	})
	if err != nil {
		t.Errorf("dry-run: %v", err)
	}
}

func mustWriteCLI(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
