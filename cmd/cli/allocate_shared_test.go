package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunAllocateShared_RequiresPeriod(t *testing.T) {
	err := runAllocateShared(context.Background(), []string{"--clickhouse=tcp://localhost:9000"})
	if err == nil || !strings.Contains(err.Error(), "--period") {
		t.Errorf("want --period err, got %v", err)
	}
}

func TestRunAllocateShared_RequiresClickhouse(t *testing.T) {
	err := runAllocateShared(context.Background(), []string{"--period=2026-05"})
	if err == nil || !strings.Contains(err.Error(), "--clickhouse") {
		t.Errorf("want --clickhouse err, got %v", err)
	}
}

func TestRunAllocateShared_BadPeriod(t *testing.T) {
	err := runAllocateShared(context.Background(), []string{
		"--period=notamonth", "--clickhouse=tcp://localhost:9000",
	})
	if err == nil || !strings.Contains(err.Error(), "--period") {
		t.Errorf("want --period parse err, got %v", err)
	}
}

func TestRunAllocateShared_EmptyRulesDir(t *testing.T) {
	// Pasta de regras vazia → 0 regras → retorna sem tentar conectar em CH.
	dir := t.TempDir()
	err := runAllocateShared(context.Background(), []string{
		"--period=2026-05", "--clickhouse=tcp://localhost:9000", "--rules=" + dir,
	})
	if err != nil {
		t.Errorf("want nil (zero rules early-exit), got %v", err)
	}
}
