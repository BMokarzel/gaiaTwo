package aws

import (
	"context"
	"errors"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/infra/collector"
)

// fakeSTS é uma implementação mínima de STSClient para testes.
type fakeSTS struct {
	account string
	err     error
}

func (f fakeSTS) GetCallerIdentity(ctx context.Context, in *sts.GetCallerIdentityInput, opts ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	acc := f.account
	return &sts.GetCallerIdentityOutput{Account: &acc}, nil
}

func TestCollector_Provider(t *testing.T) {
	c := NewWithSTS(fakeSTS{account: "1"}, awssdk.Config{})
	if got := c.Provider(); got != node.ProviderAWS {
		t.Fatalf("Provider() = %q, want %q", got, node.ProviderAWS)
	}
}

func TestValidate_OK(t *testing.T) {
	c := NewWithSTS(fakeSTS{account: "123456789012"}, awssdk.Config{})
	err := c.Validate(context.Background(), collector.Scope{Account: "123456789012"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_NoAccountConstraint(t *testing.T) {
	// Scope sem Account: qualquer identidade autenticada deve passar.
	c := NewWithSTS(fakeSTS{account: "999"}, awssdk.Config{})
	if err := c.Validate(context.Background(), collector.Scope{}); err != nil {
		t.Fatalf("expected nil error when Scope.Account empty, got %v", err)
	}
}

func TestValidate_AuthError(t *testing.T) {
	c := NewWithSTS(fakeSTS{err: errors.New("InvalidClientTokenId")}, awssdk.Config{})
	err := c.Validate(context.Background(), collector.Scope{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("expected ErrAuth, got %v", err)
	}
}

func TestValidate_AccountMismatch(t *testing.T) {
	c := NewWithSTS(fakeSTS{account: "111"}, awssdk.Config{})
	err := c.Validate(context.Background(), collector.Scope{Account: "222"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrAccount) {
		t.Fatalf("expected ErrAccount, got %v", err)
	}
}

func TestDiscover_StubClosesChannels(t *testing.T) {
	// Garante o contrato do stub: ambos os canais fecham sem emitir.
	c := NewWithSTS(fakeSTS{account: "1"}, awssdk.Config{})
	res, errs := c.Discover(context.Background(), collector.Scope{})

	for r := range res {
		t.Fatalf("expected no resources, got %+v", r)
	}
	for e := range errs {
		t.Fatalf("expected no errors, got %v", e)
	}
}

// Garantia em compile-time de que Collector satisfaz as interfaces.
var (
	_ collector.Collector = (*Collector)(nil)
	_ collector.Validator = (*Collector)(nil)
)
