package golang

import (
	"time"

	"costEngine/internal/entity/node"
)

// EmitOptions agrupa metadados de coleta aplicados a todo Service.
type EmitOptions struct {
	Repo       string    // nome canônico do repositório (vai pro slot <account> da URN)
	RunID      string    // identificador da execução do coletor
	ObservedAt time.Time // transaction time (defaults a now se zero)
}

// EmitServices converte uma lista de `Discovered` em `node.Service`.
// URN é determinística — reprocessar o mesmo repo produz o mesmo
// conjunto de URNs (idempotência F-007 D6).
func EmitServices(found []Discovered, opts EmitOptions) []node.Service {
	now := opts.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	out := make([]node.Service, 0, len(found))
	for _, d := range found {
		urn := node.NewServiceURN(opts.Repo, d.RelPath)
		out = append(out, node.Service{
			Base: node.Base{
				NodeURN:  urn,
				NodeKind: node.KindService,
				NodeMeta: node.Meta{
					Version:    1,
					ValidFrom:  now,
					ObservedAt: now,
					Source: node.Source{
						Collector: "code/golang",
						RunID:     opts.RunID,
						Method:    node.MethodDeclared,
					},
					Confidence: 1.0,
				},
			},
			Repo:       opts.Repo,
			ModulePath: d.RelPath,
			Language:   "go",
			GoModule:   d.GoModule,
		})
	}
	return out
}
