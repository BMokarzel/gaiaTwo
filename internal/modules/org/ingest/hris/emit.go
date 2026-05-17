package hris

import (
	"fmt"
	"strings"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

// EmitOptions agrupa metadados de coleta aplicados aos nós/arestas.
type EmitOptions struct {
	Tenant     string
	RunID      string
	ObservedAt time.Time
}

// Result agrega o que `Build` produz a partir das linhas válidas.
type Result struct {
	Teams     []node.Team
	Squads    []node.Squad
	Persons   []node.Person
	Roles     []node.Role // F-027: Role como nó primeiro-cidadão
	MemberOf  []edge.MemberOf
	PartOf    []edge.PartOf
	ReportsTo []edge.ReportsTo
	HasRole   []edge.HasRole // F-027: Person → Role
	LedBy     []edge.LedBy   // F-027: Team → Person (lideranças reconciliadas)
	// Terminated lista pessoas com end_date preenchido — o Writer deve
	// chamar Delete nas suas URNs e nas edges MEMBER_OF saindo delas.
	Terminated []node.URN
}

// Build converte linhas validadas em nós + edges. Detecta ciclo em
// manager_email (F-010 D6) e retorna erro fatal nesse caso.
//
// Dedup:
//   - Team por (tenant, slug)
//   - Squad por (tenant, slug)
//   - Person por (tenant, email_hash)
//
// A última linha vence em caso de duplicata para a mesma Person
// (idempotência simétrica para re-import — última observação ganha).
func Build(rows []Row, opts EmitOptions) (Result, error) {
	if opts.ObservedAt.IsZero() {
		opts.ObservedAt = time.Now().UTC()
	}
	if opts.Tenant == "" {
		return Result{}, fmt.Errorf("hris.Build: Tenant is required")
	}

	teamByName := map[string]node.Team{}
	squadByKey := map[string]node.Squad{} // key = team_slug+"/"+squad_slug
	personByHash := map[string]node.Person{}
	personRoleURN := map[string]node.URN{} // emailHash → Role URN (F-027)
	roleByURN := map[node.URN]node.Role{}   // Role URN → Role (dedup)
	terminatedHashes := map[string]bool{}
	managerOf := map[string]string{}        // personHash → managerHash
	personTeam := map[string]node.URN{}     // personHash → Team URN (F-027 LED_BY)
	titleLeader := map[string]bool{}        // personHash → tem título de liderança

	for _, r := range rows {
		emailHash := node.HashEmail(r.Email)
		teamSlug := node.Slugify(r.Team)
		squadSlug := node.Slugify(r.Squad)

		// Team
		teamURN := node.NewTeamURN(opts.Tenant, teamSlug)
		if _, ok := teamByName[teamSlug]; !ok {
			teamByName[teamSlug] = node.Team{
				Base:   newBase(teamURN, node.KindTeam, opts),
				Tenant: opts.Tenant, Slug: teamSlug, Name: r.Team,
			}
		}

		// Squad
		squadKey := teamSlug + "/" + squadSlug
		squadURN := node.NewSquadURN(opts.Tenant, squadSlug)
		if _, ok := squadByKey[squadKey]; !ok {
			squadByKey[squadKey] = node.Squad{
				Base:    newBase(squadURN, node.KindSquad, opts),
				Tenant:  opts.Tenant, Slug: squadSlug, Name: r.Squad,
				TeamURN: teamURN,
			}
		}

		// Person — última observação ganha (re-importar CSV sobrescreve).
		personURN := node.NewPersonURN(opts.Tenant, emailHash)
		mgrHash := ""
		if r.ManagerMail != "" {
			mgrHash = node.HashEmail(r.ManagerMail)
		}
		personByHash[emailHash] = node.Person{
			Base:        newBase(personURN, node.KindPerson, opts),
			Tenant:      opts.Tenant,
			EmailHash:   emailHash,
			EmailHint:   node.MaskEmail(r.Email),
			Name:        r.Name,
			Role:        r.Role,
			SquadURN:    squadURN,
			ManagerHash: mgrHash,
		}
		// F-027: tenta extrair Role canônico do free-text. Quando o
		// parse falha (formato inesperado, "Manager", etc.), mantemos
		// só `Person.Role` como fallback string.
		if track, level, ok := parseRoleString(r.Role); ok {
			roleURN := node.NewRoleURN(opts.Tenant, track, level)
			personRoleURN[emailHash] = roleURN
			if _, seen := roleByURN[roleURN]; !seen {
				roleByURN[roleURN] = node.Role{
					Base:         newBase(roleURN, node.KindRole, opts),
					Tenant:       opts.Tenant,
					Track:        track,
					Level:        level,
					Name:         roleDisplayName(track, level),
					IsLeadership: leadershipLevels[level],
				}
			}
			if leadershipLevels[level] {
				titleLeader[emailHash] = true
			}
		}
		// F-027 LED_BY: sinaliza liderança via título quando o parse
		// estruturado falha (e.g., "Engineering Manager", "Tech Lead",
		// "Head of Platform"). Reaproveita signal para reconciliação
		// posterior; não bloqueia o ingest.
		if hasLeaderKeyword(r.Role) {
			titleLeader[emailHash] = true
		}
		// Mapeia pessoa → time (via squad → team) para LED_BY.
		personTeam[emailHash] = teamURN
		if mgrHash != "" {
			managerOf[emailHash] = mgrHash
		}
		if r.EndDate != nil {
			terminatedHashes[emailHash] = true
		}
	}

	// Detect cycle in manager_email graph (F-010 D6).
	if c, ok := findCycle(managerOf); ok {
		return Result{}, fmt.Errorf("hris.Build: cycle detected in manager_email chain: %v", c)
	}

	res := Result{}

	// Sort-friendly emission: order não importa pro grafo (URN dedup),
	// mas estabilidade ajuda diffs em testes.
	for _, t := range teamByName {
		res.Teams = append(res.Teams, t)
	}
	for _, s := range squadByKey {
		res.Squads = append(res.Squads, s)
		// PART_OF: squad → team
		res.PartOf = append(res.PartOf, edge.PartOf{Base: edge.Base{
			EdgeID:   edge.DeterministicID(s.URN(), edge.TypePartOf, s.TeamURN, opts.ObservedAt),
			EdgeType: edge.TypePartOf,
			FromURN:  s.URN(),
			ToURN:    s.TeamURN,
			EdgeMeta: edgeMeta(opts),
		}})
	}
	for _, role := range roleByURN {
		res.Roles = append(res.Roles, role)
	}
	for _, p := range personByHash {
		res.Persons = append(res.Persons, p)
		// MEMBER_OF: person → squad
		res.MemberOf = append(res.MemberOf, edge.MemberOf{Base: edge.Base{
			EdgeID:   edge.DeterministicID(p.URN(), edge.TypeMemberOf, p.SquadURN, opts.ObservedAt),
			EdgeType: edge.TypeMemberOf,
			FromURN:  p.URN(),
			ToURN:    p.SquadURN,
			EdgeMeta: edgeMeta(opts),
		}})
		// F-027: HAS_ROLE: person → role (quando parseável).
		if roleURN, ok := personRoleURN[p.EmailHash]; ok {
			res.HasRole = append(res.HasRole, edge.HasRole{Base: edge.Base{
				EdgeID:   edge.DeterministicID(p.URN(), edge.TypeHasRole, roleURN, opts.ObservedAt),
				EdgeType: edge.TypeHasRole,
				FromURN:  p.URN(),
				ToURN:    roleURN,
				EdgeMeta: edgeMeta(opts),
			}})
		}
		// REPORTS_TO: person → manager (apenas se manager está no set; se
		// manager_email referencia alguém fora do CSV, ainda emitimos a
		// edge — a Person manager pode existir do import anterior).
		if p.ManagerHash != "" {
			mgrURN := node.NewPersonURN(opts.Tenant, p.ManagerHash)
			res.ReportsTo = append(res.ReportsTo, edge.ReportsTo{Base: edge.Base{
				EdgeID:   edge.DeterministicID(p.URN(), edge.TypeReportsTo, mgrURN, opts.ObservedAt),
				EdgeType: edge.TypeReportsTo,
				FromURN:  p.URN(),
				ToURN:    mgrURN,
				EdgeMeta: edgeMeta(opts),
			}})
		}
		if terminatedHashes[p.EmailHash] {
			res.Terminated = append(res.Terminated, p.URN())
		}
	}

	// F-027 LED_BY: reconcilia lideranças. Sinais (qualquer um basta):
	//   - alguém reporta a esta pessoa (managerOf reverso) → manager de facto
	//   - título de liderança (Manager/Lead/Head/Director/VP/CEO/…)
	//   - Role parsed com IsLeadership=true (Director/VP/C-level)
	//
	// A aresta é Team→Person; usa o Team do squad do líder. Lideranças
	// cross-team (manager de outra hierarquia) NÃO são modeladas — escopo
	// MVP. Dedup por (teamURN, personURN).
	hasReports := map[string]bool{}
	for _, mgrHash := range managerOf {
		if mgrHash != "" {
			hasReports[mgrHash] = true
		}
	}
	emittedLedBy := map[string]bool{} // key = teamURN+"|"+personURN
	for _, p := range personByHash {
		if terminatedHashes[p.EmailHash] {
			continue
		}
		if !hasReports[p.EmailHash] && !titleLeader[p.EmailHash] {
			continue
		}
		teamURN, ok := personTeam[p.EmailHash]
		if !ok {
			continue
		}
		key := string(teamURN) + "|" + string(p.URN())
		if emittedLedBy[key] {
			continue
		}
		emittedLedBy[key] = true
		res.LedBy = append(res.LedBy, edge.LedBy{Base: edge.Base{
			EdgeID:   edge.DeterministicID(teamURN, edge.TypeLedBy, p.URN(), opts.ObservedAt),
			EdgeType: edge.TypeLedBy,
			FromURN:  teamURN,
			ToURN:    p.URN(),
			EdgeMeta: edgeMeta(opts),
		}})
	}

	return res, nil
}

// hasLeaderKeyword reconhece títulos de liderança que não viram um
// (track, level) parseável (F-027). Heurística sintática deliberadamente
// restrita; falsos positivos são preferíveis a falsos negativos
// (operador revisa LED_BY como warning, não regra fechada).
func hasLeaderKeyword(role string) bool {
	if role == "" {
		return false
	}
	low := strings.ToLower(role)
	for _, kw := range []string{"manager", "lead", "head", "director", "vp ", " vp", "cto", "ceo", "cfo", "cpo"} {
		if strings.Contains(low, kw) {
			return true
		}
	}
	return false
}

func newBase(urn node.URN, kind node.Kind, opts EmitOptions) node.Base {
	return node.Base{
		NodeURN:  urn,
		NodeKind: kind,
		NodeMeta: node.Meta{
			Version:    1,
			ValidFrom:  opts.ObservedAt,
			ObservedAt: opts.ObservedAt,
			Source: node.Source{
				Collector: "org/hris",
				RunID:     opts.RunID,
				Method:    node.MethodDeclared,
			},
			Confidence: 1.0,
		},
	}
}

func edgeMeta(opts EmitOptions) edge.Meta {
	return edge.Meta{
		ValidFrom:   opts.ObservedAt,
		ObservedAt:  opts.ObservedAt,
		Source:      node.Source{Collector: "org/hris", RunID: opts.RunID, Method: node.MethodDeclared},
		Confidence:  1.0,
		Directional: true,
	}
}

// findCycle detecta ciclo em um grafo manager_of (mapa adjacência). Faz
// DFS por nó; retorna a sequência do ciclo se encontrado.
func findCycle(managerOf map[string]string) ([]string, bool) {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	for n := range managerOf {
		color[n] = white
	}
	var path []string
	var cycle []string
	var dfs func(n string) bool
	dfs = func(n string) bool {
		color[n] = gray
		path = append(path, n)
		if mgr, ok := managerOf[n]; ok {
			switch color[mgr] {
			case gray:
				// achou ciclo — registra a fatia desde mgr até o topo + mgr.
				for i, p := range path {
					if p == mgr {
						cycle = append(cycle, path[i:]...)
						cycle = append(cycle, mgr)
						break
					}
				}
				return true
			case white:
				if dfs(mgr) {
					return true
				}
			}
		}
		color[n] = black
		path = path[:len(path)-1]
		return false
	}
	for n := range managerOf {
		if color[n] == white {
			if dfs(n) {
				return cycle, true
			}
		}
	}
	return nil, false
}
