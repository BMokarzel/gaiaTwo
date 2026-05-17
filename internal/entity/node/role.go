package node

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// RoleTrack classifica o track de carreira (F-027).
type RoleTrack string

const (
	TrackBackend   RoleTrack = "backend"
	TrackFrontend  RoleTrack = "frontend"
	TrackMobile    RoleTrack = "mobile"
	TrackData      RoleTrack = "data"
	TrackSRE       RoleTrack = "sre"
	TrackQA        RoleTrack = "qa"
	TrackPM        RoleTrack = "pm"
	TrackDesign    RoleTrack = "design"
	TrackSecurity  RoleTrack = "security"
	TrackFullstack RoleTrack = "fullstack"
)

// RoleLevel classifica o nível de senioridade (F-027).
type RoleLevel string

const (
	LevelJunior    RoleLevel = "junior"
	LevelMid       RoleLevel = "mid"
	LevelSenior    RoleLevel = "senior"
	LevelStaff     RoleLevel = "staff"
	LevelPrincipal RoleLevel = "principal"
	LevelDirector  RoleLevel = "director"
	LevelVP        RoleLevel = "vp"
	LevelC         RoleLevel = "c-level"
)

// Role é um nó por combinação `(track, level)`. Várias Persons
// referenciam o mesmo Role via HAS_ROLE (cardinalidade 1).
//
// URN: urn:ce:org:<tenant>:role/<track>-<level>
//
// Substitui o atributo string `Role` em Person (F-010) — esse atributo
// fica deprecated durante a migração.
type Role struct {
	Base
	Tenant       string    `json:"tenant"`
	Track        RoleTrack `json:"track"`
	Level        RoleLevel `json:"level"`
	Name         string    `json:"name"`           // "Backend Senior"
	IsLeadership bool      `json:"is_leadership,omitempty"`
}

// NewRoleURN produz a URN canônica para um Role.
func NewRoleURN(tenant string, track RoleTrack, level RoleLevel) URN {
	id := strings.ToLower(string(track)) + "-" + strings.ToLower(string(level))
	return NewURN(ProviderOrg, tenant, KindRole, id)
}

// ContentHash dos campos significativos.
func (r Role) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(string(r.Track))
	sb.WriteByte('|')
	sb.WriteString(string(r.Level))
	sb.WriteByte('|')
	sb.WriteString(r.Name)
	sb.WriteByte('|')
	if r.IsLeadership {
		sb.WriteString("L")
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
