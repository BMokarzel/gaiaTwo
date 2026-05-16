package node

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

// PersistenceFlavor classifica a natureza do armazenamento.
type PersistenceFlavor string

const (
	PersistenceBlock     PersistenceFlavor = "block"     // EBS, Persistent Disk
	PersistenceObject    PersistenceFlavor = "object"    // S3, GCS, Blob Storage
	PersistenceFile      PersistenceFlavor = "file"      // EFS, Filestore
	PersistenceRelDB     PersistenceFlavor = "rdbms"     // RDS, Cloud SQL
	PersistenceNoSQL     PersistenceFlavor = "nosql"     // DynamoDB, Firestore
	PersistenceCache     PersistenceFlavor = "cache"     // ElastiCache, Memorystore
	PersistenceWarehouse PersistenceFlavor = "warehouse" // Redshift, BigQuery
)

// Persistence representa qualquer recurso de armazenamento.
type Persistence struct {
	Base
	ProviderID ProviderID        `json:"provider_id"`
	Account    URN               `json:"account_urn"`
	Region     URN               `json:"region_urn"`
	ExtID      string            `json:"external_id"`
	Flavor     PersistenceFlavor `json:"flavor"`
	Engine     string            `json:"engine,omitempty"` // "postgres-15", "redis-7"
	SizeGiB    uint64            `json:"size_gib,omitempty"`
	IOPS       uint32            `json:"iops,omitempty"`
	Encrypted  bool              `json:"encrypted,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
	SpecRaw    map[string]any    `json:"spec_raw,omitempty"`
}

// Resource interface implementation.
func (p Persistence) Provider() ProviderID          { return p.ProviderID }
func (p Persistence) AccountURN() URN               { return p.Account }
func (p Persistence) RegionURN() URN                { return p.Region }
func (p Persistence) ExternalID() string            { return p.ExtID }
func (p Persistence) NativeTags() map[string]string { return p.Tags }
func (p Persistence) Spec() map[string]any          { return p.SpecRaw }

// ContentHash retorna um hash determinístico dos *campos significativos*
// de um Persistence — exclui Meta (versão/tempo) e SpecRaw (ruído).
// Mesmo papel de Compute.ContentHash: coletores usam para decidir
// entre Upsert (mudança real) e Touch (só ObservedAt).
//
// Inclui: Provider, Flavor, Engine, SizeGiB, IOPS, Encrypted, Tags
// (ordenadas por chave). NÃO inclui ExtID/Account/Region (parte da URN).
func (p Persistence) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(string(p.ProviderID))
	sb.WriteByte('|')
	sb.WriteString(string(p.Flavor))
	sb.WriteByte('|')
	sb.WriteString(p.Engine)
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatUint(p.SizeGiB, 10))
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatUint(uint64(p.IOPS), 10))
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatBool(p.Encrypted))
	sb.WriteByte('|')

	keys := make([]string, 0, len(p.Tags))
	for k := range p.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(p.Tags[k])
		sb.WriteByte(',')
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
