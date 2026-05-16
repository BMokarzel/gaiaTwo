package main

import (
	"context"
	"strings"
	"testing"

	"costEngine/internal/modules/cost/allocate"
)

func TestRunAllocate_RequiresPeriod(t *testing.T) {
	err := runAllocate(context.Background(), []string{"--clickhouse=localhost:9000"})
	if err == nil || !strings.Contains(err.Error(), "period") {
		t.Fatalf("expected period error, got %v", err)
	}
}

func TestRunAllocate_BadPeriod(t *testing.T) {
	err := runAllocate(context.Background(), []string{"--period=2026-13", "--clickhouse=localhost:9000"})
	if err == nil || !strings.Contains(err.Error(), "period") {
		t.Fatalf("expected period parse error, got %v", err)
	}
}

func TestRunAllocate_RequiresClickHouse(t *testing.T) {
	err := runAllocate(context.Background(), []string{"--period=2026-05"})
	if err == nil || !strings.Contains(err.Error(), "clickhouse") {
		t.Fatalf("expected clickhouse error, got %v", err)
	}
}

func TestParseDimensions(t *testing.T) {
	dims, err := parseDimensions("service,account,region")
	if err != nil {
		t.Fatal(err)
	}
	if len(dims) != 3 {
		t.Errorf("len=%d want 3", len(dims))
	}
	if dims[0].Dimension() != allocate.DimensionService {
		t.Errorf("dims[0] = %v", dims[0].Dimension())
	}
}

func TestParseDimensions_Bad(t *testing.T) {
	if _, err := parseDimensions("service,team"); err == nil {
		t.Fatal("expected error for unknown dimension")
	}
}
