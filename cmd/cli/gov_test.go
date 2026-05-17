package main

import (
	"context"
	"strings"
	"testing"
)

// O backend memory é in-process e zera por invocação (cada chamada de
// `buildGovBackend` instancia um repo novo). Logo os testes que cobrem
// round-trip precisam encadear create+get dentro do *mesmo* processo —
// algo que o CLI real não faz. Aqui validamos apenas:
//
//   - parsing de flags (uso inválido devolve errBadUsage);
//   - despacho de kinds/ops (kinds inválidos retornam erro);
//   - happy path de create (não explode em memory).
//
// Round-trip de persistência fica nos testes do service (gov/service)
// e do controller (gov/controller), que já cobrem todos os kinds.

func TestRunGov_RequiresOp(t *testing.T) {
	err := runGov(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "uso inválido") {
		t.Errorf("want bad-usage, got %v", err)
	}
}

func TestRunGov_RequiresKind(t *testing.T) {
	err := runGov(context.Background(), []string{"create"})
	if err == nil || !strings.Contains(err.Error(), "uso inválido") {
		t.Errorf("want bad-usage, got %v", err)
	}
}

func TestRunGov_UnknownOp(t *testing.T) {
	err := runGov(context.Background(), []string{"explode", "company"})
	if err == nil || !strings.Contains(err.Error(), "operação desconhecida") {
		t.Errorf("want unknown-op, got %v", err)
	}
}

func TestRunGov_UnknownKind(t *testing.T) {
	err := runGov(context.Background(), []string{
		"create", "fairy", "--tenant=acme", "--short-id=x", "--name=X",
	})
	if err == nil || !strings.Contains(err.Error(), "kind desconhecido") {
		t.Errorf("want unknown-kind, got %v", err)
	}
}

func TestRunGov_Create_RequiresTenant(t *testing.T) {
	err := runGov(context.Background(), []string{
		"create", "company", "--short-id=ce", "--name=CostEngine",
	})
	if err == nil || !strings.Contains(err.Error(), "--tenant") {
		t.Errorf("want --tenant required, got %v", err)
	}
}

func TestRunGov_Create_RequiresShortIDAndName(t *testing.T) {
	err := runGov(context.Background(), []string{
		"create", "company", "--tenant=acme",
	})
	if err == nil || !strings.Contains(err.Error(), "--short-id") {
		t.Errorf("want missing-flags error, got %v", err)
	}
}

func TestRunGov_CreateCompany_Happy(t *testing.T) {
	err := runGov(context.Background(), []string{
		"create", "company",
		"--tenant=acme",
		"--short-id=cost-engine",
		"--name=CostEngine",
		"--domain=costengine.io",
	})
	if err != nil {
		t.Fatalf("create company falhou: %v", err)
	}
}

func TestRunGov_List_RequiresTenant(t *testing.T) {
	err := runGov(context.Background(), []string{"list", "company"})
	if err == nil || !strings.Contains(err.Error(), "--tenant") {
		t.Errorf("want --tenant required, got %v", err)
	}
}

func TestRunGov_ListCompany_EmptyOK(t *testing.T) {
	// Memory backend zera por processo → list retorna página vazia,
	// não erro. Suficiente para validar wiring.
	err := runGov(context.Background(), []string{
		"list", "company", "--tenant=acme",
	})
	if err != nil {
		t.Errorf("list company vazio: want nil, got %v", err)
	}
}

func TestRunGov_Get_RequiresURN(t *testing.T) {
	err := runGov(context.Background(), []string{"get", "company"})
	if err == nil || !strings.Contains(err.Error(), "<urn>") {
		t.Errorf("want urn required, got %v", err)
	}
}

func TestRunGov_Patch_RequiresURN(t *testing.T) {
	err := runGov(context.Background(), []string{"patch", "feature"})
	if err == nil || !strings.Contains(err.Error(), "<urn>") {
		t.Errorf("want urn required, got %v", err)
	}
}

func TestRunGov_Delete_RequiresURN(t *testing.T) {
	err := runGov(context.Background(), []string{"delete", "domain"})
	if err == nil || !strings.Contains(err.Error(), "<urn>") {
		t.Errorf("want urn required, got %v", err)
	}
}

func TestRunGov_Help(t *testing.T) {
	if err := runGov(context.Background(), []string{"help"}); err != nil {
		t.Errorf("help: want nil, got %v", err)
	}
}
