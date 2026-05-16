// Package aws implementa o Collector de infraestrutura AWS.
//
// Na S-001 (scaffold) só existe Validate, que confirma identidade via
// STS GetCallerIdentity. Discover é stub e será preenchido em S-002+
// (Account/Region/Zone, EC2, EBS, S3, RDS, Network).
package aws

import (
	"context"
	"errors"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/infra/collector"
)

// Erros tipados para que CLI/serviços possam discriminar com errors.Is.
var (
	// ErrAuth indica falha na autenticação (credenciais ausentes,
	// expiradas, ou recusadas pelo STS).
	ErrAuth = errors.New("aws: authentication failed")

	// ErrAccount indica que a credencial é válida mas a identidade
	// pertence a uma conta diferente da requisitada pelo Scope.
	ErrAccount = errors.New("aws: account mismatch")
)

// STSClient é a sub-API do AWS SDK que utilizamos para validação.
// Declarado como interface para permitir injeção de fake em testes.
type STSClient interface {
	GetCallerIdentity(ctx context.Context, in *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// Collector é a implementação AWS de collector.Collector + Validator.
type Collector struct {
	sts STSClient
	cfg awssdk.Config
}

// New carrega a configuração default do AWS SDK v2 (env, ~/.aws/config,
// IAM role, etc.) para a região indicada e instancia o Collector.
func New(ctx context.Context, region string) (*Collector, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	return &Collector{sts: sts.NewFromConfig(cfg), cfg: cfg}, nil
}

// NewWithSTS constrói um Collector com STSClient explícito. Reservado
// para testes; produção usa New.
func NewWithSTS(c STSClient, cfg awssdk.Config) *Collector {
	return &Collector{sts: c, cfg: cfg}
}

// Provider retorna node.ProviderAWS.
func (c *Collector) Provider() node.ProviderID { return node.ProviderAWS }

// Config expõe a awssdk.Config carregada para que sub-coletores
// (Topology, EC2, S3, ...) compartilhem credenciais e região.
func (c *Collector) Config() awssdk.Config { return c.cfg }

// Validate confirma identidade via STS e, se scope.Account != "",
// verifica que a conta autenticada coincide com a requisitada.
func (c *Collector) Validate(ctx context.Context, scope collector.Scope) error {
	out, err := c.sts.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAuth, err)
	}
	if scope.Account != "" && out.Account != nil && *out.Account != scope.Account {
		return fmt.Errorf("%w: requested %q, authenticated as %q",
			ErrAccount, scope.Account, *out.Account)
	}
	return nil
}

// Discover é stub na S-001. Implementações reais em S-002..S-007.
// Fecha os canais imediatamente para preservar o contrato do Collector.
func (c *Collector) Discover(ctx context.Context, scope collector.Scope) (<-chan node.Resource, <-chan error) {
	res := make(chan node.Resource)
	errs := make(chan error)
	close(res)
	close(errs)
	return res, errs
}
