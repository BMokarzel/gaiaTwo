package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunBridge_NoSubcommand(t *testing.T) {
	err := runBridge(context.Background(), []string{})
	if err == nil || !strings.Contains(err.Error(), "ce bridge") {
		t.Errorf("want bridge usage err, got %v", err)
	}
}

func TestRunBridge_UnknownSubcommand(t *testing.T) {
	err := runBridge(context.Background(), []string{"compute-service"})
	if err == nil || !strings.Contains(err.Error(), "service-compute") {
		t.Errorf("want hint err, got %v", err)
	}
}

func TestRunBridgeServiceCompute_RequiresAccount(t *testing.T) {
	err := runBridgeServiceCompute(context.Background(), []string{})
	if err == nil || !strings.Contains(err.Error(), "--account") {
		t.Errorf("want --account err, got %v", err)
	}
}

func TestRunBridgeServiceCompute_MemoryDryRunSmoke(t *testing.T) {
	// Sem --neo4j → backend memory vazio: nada para resolver, mas
	// também não deve falhar.
	err := runBridgeServiceCompute(context.Background(), []string{
		"--account=urn:ce:aws:111:account/111",
		"--dry-run",
	})
	if err != nil {
		t.Errorf("smoke dry-run: %v", err)
	}
}
