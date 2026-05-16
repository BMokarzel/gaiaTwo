package edge

import "costEngine/internal/entity/node"

// Routes modela conectividade direcional entre componentes de rede.
//
// Subnet ──▶ VPC (containment routing)
// LB     ──▶ TargetGroup
// Gateway──▶ VPC
type Routes struct {
	Base
	CIDRTarget string   `json:"cidr_target,omitempty"`
	NextHop    node.URN `json:"next_hop,omitempty"`
}

// Peers modela relação SIMÉTRICA entre redes (VPC peering, transit gateway).
// EdgeMeta.Directional deve ser false.
type Peers struct {
	Base
	AllowedCIDRs []string `json:"allowed_cidrs,omitempty"`
}
