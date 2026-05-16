package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunIngestHRIS_RequiresCSV(t *testing.T) {
	err := runIngestHRIS(context.Background(), []string{"--tenant=acme"})
	if err == nil || !strings.Contains(err.Error(), "--csv") {
		t.Errorf("want --csv error, got %v", err)
	}
}

func TestRunIngestHRIS_RequiresTenant(t *testing.T) {
	err := runIngestHRIS(context.Background(), []string{"--csv=foo.csv"})
	if err == nil || !strings.Contains(err.Error(), "--tenant") {
		t.Errorf("want --tenant error, got %v", err)
	}
}

func TestRunIngestHRIS_DryRunMemorySmoke(t *testing.T) {
	dir := t.TempDir()
	csv := filepath.Join(dir, "p.csv")
	body := `email,name,role,team,squad,manager_email,start_date,end_date_or_blank
alice@x.com,Alice,Eng,Platform,Checkout,,2024-01-15,
bob@x.com,Bob,Eng,Platform,Growth,,2024-01-15,
`
	if err := os.WriteFile(csv, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runIngestHRIS(context.Background(), []string{
		"--csv=" + csv, "--tenant=acme", "--dry-run",
	})
	if err != nil {
		t.Errorf("dry-run: %v", err)
	}
}
