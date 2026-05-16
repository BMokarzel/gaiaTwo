package edge

// Contains modela contenção forte (cascata semântica).
//
// Provider ──▶ Account ──▶ Region ──▶ Resource
// VPC      ──▶ Subnet
// Cluster  ──▶ Pod
//
// O nó container aponta para o contido.
type Contains struct {
	Base
}
