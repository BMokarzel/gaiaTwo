package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunExtractCodeowners_RequiresRepo(t *testing.T) {
	err := runExtractCodeowners(context.Background(), []string{"--tenant=acme"})
	if err == nil || !strings.Contains(err.Error(), "--repo") {
		t.Errorf("want --repo error, got %v", err)
	}
}

func TestRunExtractCodeowners_RequiresTenant(t *testing.T) {
	err := runExtractCodeowners(context.Background(), []string{"--repo=/tmp/x"})
	if err == nil || !strings.Contains(err.Error(), "--tenant") {
		t.Errorf("want --tenant error, got %v", err)
	}
}

func TestRunExtractCodeowners_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	err := runExtractCodeowners(context.Background(), []string{
		"--repo=" + dir, "--tenant=acme",
	})
	if err == nil || !strings.Contains(err.Error(), "file not found") {
		t.Errorf("want not-found error, got %v", err)
	}
}

func TestRunExtractCodeowners_NoGlobalRule(t *testing.T) {
	dir := t.TempDir()
	mustWriteCLI(t, filepath.Join(dir, "CODEOWNERS"), "# only path rules\nsrc/ @alice\n")
	err := runExtractCodeowners(context.Background(), []string{
		"--repo=" + dir, "--tenant=acme", "--dry-run",
	})
	if err == nil || !strings.Contains(err.Error(), "no global") {
		t.Errorf("want no-global error, got %v", err)
	}
}

// O backend memory in-process inicia vazio; o repo→Service não casa,
// devolvendo ErrServiceNotFound. O CLI converte em errPartial + JSON
// no stdout (não duplica como erro fatal).
func TestRunExtractCodeowners_ServiceNotFound(t *testing.T) {
	dir := t.TempDir()
	mustWriteCLI(t, filepath.Join(dir, "CODEOWNERS"), "* @alice @org/payments\n")
	err := runExtractCodeowners(context.Background(), []string{
		"--repo=" + dir, "--tenant=acme", "--dry-run",
	})
	if !errors.Is(err, errPartial) {
		t.Errorf("want errPartial, got %v", err)
	}
}
