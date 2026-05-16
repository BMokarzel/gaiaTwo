package node

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// NetworkFlavor classifica o tipo de componente de rede.
type NetworkFlavor string

const (
	NetworkVPC           NetworkFlavor = "vpc"
	NetworkSubnet        NetworkFlavor = "subnet"
	NetworkLB            NetworkFlavor = "loadbalancer"
	NetworkSecurityGroup NetworkFlavor = "securitygroup" // SG, firewall
	NetworkGateway       NetworkFlavor = "gateway"       // IGW, NAT, VPN
	NetworkPeering       NetworkFlavor = "peering"       // VPC peering, transit gateway
	NetworkEndpoint      NetworkFlavor = "endpoint"      // PrivateLink, Service Connect
	NetworkENI           NetworkFlavor = "eni"           // interfaces de rede
)

// Network representa qualquer componente de rede do provedor.
// Central para cálculos de custo de egress e topologia.
type Network struct {
	Base
	ProviderID ProviderID        `json:"provider_id"`
	Account    URN               `json:"account_urn"`
	Region     URN               `json:"region_urn"`
	ExtID      string            `json:"external_id"`
	Flavor     NetworkFlavor     `json:"flavor"`
	CIDR       string            `json:"cidr,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
	SpecRaw    map[string]any    `json:"spec_raw,omitempty"`
}

// Resource interface implementation.
func (n Network) Provider() ProviderID          { return n.ProviderID }
func (n Network) AccountURN() URN               { return n.Account }
func (n Network) RegionURN() URN                { return n.Region }
func (n Network) ExternalID() string            { return n.ExtID }
func (n Network) NativeTags() map[string]string { return n.Tags }
func (n Network) Spec() map[string]any          { return n.SpecRaw }

// ContentHash retorna um hash determinístico dos *campos significativos*
// de um Network — exclui Meta (versão/tempo) e SpecRaw (ruído). Mesma
// função que Compute.ContentHash / Persistence.ContentHash: coletores
// usam para decidir entre Upsert (mudança) e Touch (só ObservedAt).
//
// Inclui: Provider, Flavor, CIDR, Tags (ordenadas). NÃO inclui
// ExtID/Account/Region (parte da URN).
func (n Network) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(string(n.ProviderID))
	sb.WriteByte('|')
	sb.WriteString(string(n.Flavor))
	sb.WriteByte('|')
	sb.WriteString(n.CIDR)
	sb.WriteByte('|')

	keys := make([]string, 0, len(n.Tags))
	for k := range n.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(n.Tags[k])
		sb.WriteByte(',')
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
