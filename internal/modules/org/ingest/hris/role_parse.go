package hris

import (
	"strings"

	"costEngine/internal/entity/node"
)

// trackKeywords mapeia tokens livres do CSV `role` para o RoleTrack
// canônico. Lista intencionalmente curta — preferimos não-classificar a
// classificar errado (a string original fica em `Person.Role` como
// fallback de auditoria).
var trackKeywords = map[string]node.RoleTrack{
	"backend":   node.TrackBackend,
	"back-end":  node.TrackBackend,
	"server":    node.TrackBackend,
	"frontend":  node.TrackFrontend,
	"front-end": node.TrackFrontend,
	"web":       node.TrackFrontend,
	"mobile":    node.TrackMobile,
	"ios":       node.TrackMobile,
	"android":   node.TrackMobile,
	"data":      node.TrackData,
	"ml":        node.TrackData,
	"analytics": node.TrackData,
	"sre":       node.TrackSRE,
	"devops":    node.TrackSRE,
	"infra":     node.TrackSRE,
	"platform":  node.TrackSRE,
	"qa":        node.TrackQA,
	"test":      node.TrackQA,
	"pm":        node.TrackPM,
	"product":   node.TrackPM,
	"design":    node.TrackDesign,
	"ux":        node.TrackDesign,
	"security":  node.TrackSecurity,
	"sec":       node.TrackSecurity,
	"fullstack": node.TrackFullstack,
	"full":      node.TrackFullstack, // "Full Stack" / "Full-Stack"
}

// levelKeywords mapeia tokens livres do CSV para o RoleLevel canônico.
var levelKeywords = map[string]node.RoleLevel{
	"junior":    node.LevelJunior,
	"jr":        node.LevelJunior,
	"mid":       node.LevelMid,
	"pleno":     node.LevelMid,
	"senior":    node.LevelSenior,
	"sr":        node.LevelSenior,
	"staff":     node.LevelStaff,
	"principal": node.LevelPrincipal,
	"director":  node.LevelDirector,
	"head":      node.LevelDirector,
	"vp":        node.LevelVP,
	"cto":       node.LevelC,
	"ceo":       node.LevelC,
	"cfo":       node.LevelC,
	"cpo":       node.LevelC,
	"c-level":   node.LevelC,
}

// leadershipLevels é o conjunto de níveis que marcam `IsLeadership=true`.
var leadershipLevels = map[node.RoleLevel]bool{
	node.LevelDirector: true,
	node.LevelVP:       true,
	node.LevelC:        true,
}

// parseRoleString tenta extrair `(track, level)` de uma string livre
// vinda do CSV (ex.: "Backend Senior", "Senior Backend Engineer",
// "Staff SRE", "VP Engineering", "CTO"). Devolve `ok=false` quando não
// achou pelo menos um track + um level. Para "VP"/"CTO" sem track
// explícito, o track é deduzido como vazio → `ok=false` (mantém Role
// só no campo string da Person).
func parseRoleString(raw string) (node.RoleTrack, node.RoleLevel, bool) {
	if raw == "" {
		return "", "", false
	}
	// Normaliza separadores comuns ("-", "/") em espaço.
	s := strings.ToLower(raw)
	s = strings.NewReplacer("-", " ", "/", " ", "_", " ").Replace(s)

	var track node.RoleTrack
	var level node.RoleLevel
	for _, tok := range strings.Fields(s) {
		if track == "" {
			if t, ok := trackKeywords[tok]; ok {
				track = t
				continue
			}
		}
		if level == "" {
			if l, ok := levelKeywords[tok]; ok {
				level = l
			}
		}
	}
	if track == "" || level == "" {
		return "", "", false
	}
	return track, level, true
}

// roleDisplayName produz um label legível ("Backend Senior") a partir
// do par (track, level). Usado em `node.Role.Name`.
func roleDisplayName(track node.RoleTrack, level node.RoleLevel) string {
	t := strings.ToUpper(string(track[:1])) + string(track[1:])
	l := strings.ToUpper(string(level[:1])) + string(level[1:])
	return t + " " + l
}
