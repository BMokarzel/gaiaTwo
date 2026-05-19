package typescript

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

const collectorTag = "code/typescript"

// nodeMeta produz o `node.Meta` padrão para entidades emitidas por este
// coletor. Bitemporal MVP: ValidFrom = ObservedAt; Confidence = 1.0
// (declarado pelo AST).
func (d *decoder) nodeMeta() node.Meta {
	now := d.cfg.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return node.Meta{
		Version:    1,
		ValidFrom:  now,
		ObservedAt: now,
		Source: node.Source{
			Collector: collectorTag,
			RunID:     d.cfg.RunID,
			Method:    node.MethodDeclared,
		},
		Confidence: 1.0,
	}
}

func (d *decoder) edgeMeta() edge.Meta {
	now := d.cfg.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return edge.Meta{
		ValidFrom:   now,
		ObservedAt:  now,
		Source:      node.Source{Collector: collectorTag, RunID: d.cfg.RunID, Method: node.MethodDeclared},
		Confidence:  1.0,
		Directional: true,
	}
}

func (d *decoder) base(urn node.URN, kind node.Kind) node.Base {
	return node.Base{NodeURN: urn, NodeKind: kind, NodeMeta: d.nodeMeta()}
}

// ---------- service / module ----------

func (d *decoder) appendService(p ServicePayload) {
	urn := node.NewServiceURN(d.cfg.Repo, p.ModulePath)
	mt := node.ManifestType(p.ManifestType)
	if mt == "" {
		mt = node.ManifestNpm
	}
	lang := p.Language
	if lang == "" {
		lang = "typescript"
	}
	d.res.Services = append(d.res.Services, node.Service{
		Base:         d.base(urn, node.KindService),
		Repo:         d.cfg.Repo,
		ModulePath:   p.ModulePath,
		Language:     lang,
		Namespace:    p.Namespace,
		Manifest:     p.Manifest,
		ManifestType: mt,
		FeatureTags:  p.FeatureTags,
		Tags:         p.Tags,
	})
}

func (d *decoder) appendModule(p ModulePayload) {
	urn := node.NewModuleURN(d.cfg.Repo, p.ServiceModulePath, p.Namespace)
	serviceURN := node.NewServiceURN(d.cfg.Repo, p.ServiceModulePath)
	short := p.Namespace
	if i := strings.LastIndex(short, "/"); i >= 0 {
		short = short[i+1:]
	}
	d.res.Modules = append(d.res.Modules, node.Module{
		Base:       d.base(urn, node.KindModule),
		ServiceURN: serviceURN,
		Namespace:  p.Namespace,
		ShortName:  short,
		Path:       p.Path,
		Language:   "typescript",
	})
}

// ---------- endpoint / function ----------

func (d *decoder) appendEndpoint(p EndpointPayload) {
	urn := node.NewEndpointURN(d.cfg.Repo, p.ServiceModulePath, p.Method, p.Path)
	serviceURN := node.NewServiceURN(d.cfg.Repo, p.ServiceModulePath)
	// `ModuleNamespace` pode ser "" (módulo raiz). Sidecar emite um Module
	// para namespace=="" também, então sempre podemos pintar Contains.
	modURN := node.NewModuleURN(d.cfg.Repo, p.ServiceModulePath, p.ModuleNamespace)
	d.res.Endpoints = append(d.res.Endpoints, node.Endpoint{
		Base:       d.base(urn, node.KindEndpoint),
		ServiceURN: serviceURN,
		ModuleURN:  modURN,
		Method:     strings.ToUpper(p.Method),
		Route:      p.Path,
		Handler:    p.HandlerSymbol,
		Framework:  p.Framework,
		Location:   locFrom(p.Location),
	})
	// Sidecar `endpoints_express.ts` não emite Contains Module→Endpoint
	// porque o ModuleURN só é conhecido Go-side. Materializamos aqui.
	now := d.cfg.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	id := edge.DeterministicID(modURN, edge.TypeContains, urn, now)
	d.res.Contains = append(d.res.Contains, edge.Contains{Base: edge.Base{
		EdgeID: id, EdgeType: edge.TypeContains, FromURN: modURN, ToURN: urn, EdgeMeta: d.edgeMeta(),
	}})
}

func (d *decoder) appendFunction(p FunctionPayload) {
	urn := node.NewFunctionURN(d.cfg.Repo, p.ServiceModulePath, p.Namespace, p.Symbol)
	serviceURN := node.NewServiceURN(d.cfg.Repo, p.ServiceModulePath)
	var modURN node.URN
	if p.ModuleNamespace != "" {
		modURN = node.NewModuleURN(d.cfg.Repo, p.ServiceModulePath, p.ModuleNamespace)
	}
	sigHash := p.SignatureHash
	if sigHash == "" {
		sigHash = signatureHash(p.Signature)
	}
	d.res.Functions = append(d.res.Functions, node.Function{
		Base:          d.base(urn, node.KindFunction),
		ServiceURN:    serviceURN,
		ModuleURN:     modURN,
		Namespace:     p.Namespace,
		Symbol:        p.Symbol,
		SignatureHash: sigHash,
		Signature:     p.Signature,
		Exported:      p.Exported,
		Location:      locFrom(p.Location),
	})
}

// ---------- call ----------

// callOrdinal mantém contador por Function-caller no decodificador.
// Sidecar não controla ordinal — emite na ordem; Go-side numera.
func (d *decoder) callOrdinal(callerURN node.URN) int {
	if d.callerOrd == nil {
		d.callerOrd = map[node.URN]int{}
	}
	n := d.callerOrd[callerURN]
	d.callerOrd[callerURN] = n + 1
	return n
}

func (d *decoder) appendCall(p CallPayload) {
	callerURN := node.URN(p.FromFunctionURN)
	kind := mapCallKind(p.Subkind)
	ord := d.callOrdinal(callerURN)
	urn := node.NewCallURN(d.cfg.Repo, kind, callerURN, ord)
	d.res.Calls = append(d.res.Calls, node.Call{
		Base:         d.base(urn, node.KindCall),
		Kind_:        kind,
		CallerURN:    callerURN,
		Ordinal:      ord,
		Location:     locFrom(p.Location),
		TargetSymbol: p.CalleeExpression,
		TargetURL:    p.TargetHint,
	})
	// Sidecar `calls.ts` documenta "Invokes Function→Call (sempre)" mas o
	// emit ficou Go-side porque a URN do Call só existe aqui.
	now := d.cfg.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	id := edge.DeterministicID(callerURN, edge.TypeInvokes, urn, now)
	d.res.Invokes = append(d.res.Invokes, edge.Invokes{Base: edge.Base{
		EdgeID: id, EdgeType: edge.TypeInvokes, FromURN: callerURN, ToURN: urn, EdgeMeta: d.edgeMeta(),
	}})
}

func mapCallKind(subkind string) node.CallKind {
	switch subkind {
	case "http!":
		return node.CallHttpCall
	case "db!":
		return node.CallDataAccess
	case "mq!":
		return node.CallQueueSend
	case "in-process", "":
		return node.CallFunctionCall
	default:
		return node.CallFunctionCall
	}
}

// ---------- type / variable ----------

func (d *decoder) appendType(p TypePayload) {
	urn := node.NewTypeURN(d.cfg.Repo, p.ServiceModulePath, p.Namespace, p.Symbol)
	serviceURN := node.NewServiceURN(d.cfg.Repo, p.ServiceModulePath)
	var modURN node.URN
	if p.ModuleNamespace != "" {
		modURN = node.NewModuleURN(d.cfg.Repo, p.ServiceModulePath, p.ModuleNamespace)
	}
	d.res.Types = append(d.res.Types, node.Type{
		Base:       d.base(urn, node.KindType),
		ServiceURN: serviceURN,
		ModuleURN:  modURN,
		Namespace:  p.Namespace,
		Symbol:     p.Symbol,
		Kind_:      mapTypeKind(p.Kind),
		Exported:   p.Exported,
		Location:   locFrom(p.Location),
	})
}

func mapTypeKind(k string) node.TypeKind {
	switch k {
	case "interface":
		return node.TypeKindInterface
	case "class":
		return node.TypeKindClass
	case "enum":
		return node.TypeKindEnum
	case "type", "alias":
		return node.TypeKindAlias
	default:
		return node.TypeKindAlias
	}
}

func (d *decoder) appendVariable(p VariablePayload) {
	urn := node.NewVariableURN(d.cfg.Repo, p.ServiceModulePath, p.ModuleNamespace, p.Symbol)
	serviceURN := node.NewServiceURN(d.cfg.Repo, p.ServiceModulePath)
	var modURN node.URN
	if p.ModuleNamespace != "" {
		modURN = node.NewModuleURN(d.cfg.Repo, p.ServiceModulePath, p.ModuleNamespace)
	}
	d.res.Variables = append(d.res.Variables, node.Variable{
		Base:       d.base(urn, node.KindVariable),
		ServiceURN: serviceURN,
		ModuleURN:  modURN,
		Namespace:  p.ModuleNamespace,
		Symbol:     p.Symbol,
		TypeRef:    p.TypeText,
		Mutability: node.VariableConst, // MVP: TS const/let — sidecar pode preencher mais tarde
		Exported:   p.Exported,
		Location:   locFrom(p.Location),
	})
}

// ---------- framework / edges ----------

func (d *decoder) appendFramework(p FrameworkPayload) {
	urn := node.NewFrameworkURN(p.Ecosystem, p.Name)
	d.res.Frameworks = append(d.res.Frameworks, node.Framework{
		Base:          d.base(urn, node.KindFramework),
		Ecosystem:     p.Ecosystem,
		Name:          p.Name,
		LatestVersion: p.LatestVersion,
		IsDevOnly:     p.IsDevOnly,
	})
}

// appendEdge materializa edges cross-entidade emitidas pelo sidecar.
// Sidecar manda `from_urn`/`to_urn` já como URNs canônicas. Go-side
// constrói o struct edge tipado correspondente.
func (d *decoder) appendEdge(p EdgePayload) {
	from := node.URN(p.FromURN)
	to := node.URN(p.ToURN)
	now := d.cfg.ObservedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	switch p.Type {
	case "Invokes":
		id := edge.DeterministicID(from, edge.TypeInvokes, to, now)
		d.res.Invokes = append(d.res.Invokes, edge.Invokes{Base: edge.Base{
			EdgeID: id, EdgeType: edge.TypeInvokes, FromURN: from, ToURN: to, EdgeMeta: d.edgeMeta(),
		}})
	case "Targets":
		id := edge.DeterministicID(from, edge.TypeTargets, to, now)
		d.res.Targets = append(d.res.Targets, edge.Targets{Base: edge.Base{
			EdgeID: id, EdgeType: edge.TypeTargets, FromURN: from, ToURN: to, EdgeMeta: d.edgeMeta(),
		}})
	case "Uses":
		id := edge.DeterministicID(from, edge.TypeUses, to, now)
		d.res.Uses = append(d.res.Uses, edge.Uses{Base: edge.Base{
			EdgeID: id, EdgeType: edge.TypeUses, FromURN: from, ToURN: to, EdgeMeta: d.edgeMeta(),
		}})
	case "Extends":
		id := edge.DeterministicID(from, edge.TypeExtends, to, now)
		d.res.Extends = append(d.res.Extends, edge.Extends{Base: edge.Base{
			EdgeID: id, EdgeType: edge.TypeExtends, FromURN: from, ToURN: to, EdgeMeta: d.edgeMeta(),
		}})
	case "Aliases":
		id := edge.DeterministicID(from, edge.TypeAliases, to, now)
		d.res.Aliases = append(d.res.Aliases, edge.Aliases{Base: edge.Base{
			EdgeID: id, EdgeType: edge.TypeAliases, FromURN: from, ToURN: to, EdgeMeta: d.edgeMeta(),
		}})
	case "Contains":
		id := edge.DeterministicID(from, edge.TypeContains, to, now)
		d.res.Contains = append(d.res.Contains, edge.Contains{Base: edge.Base{
			EdgeID: id, EdgeType: edge.TypeContains, FromURN: from, ToURN: to, EdgeMeta: d.edgeMeta(),
		}})
	case "DependsOn":
		id := edge.DeterministicID(from, edge.TypeDependsOn, to, now)
		d.res.DependsOn = append(d.res.DependsOn, edge.DependsOn{
			Base:     edge.Base{EdgeID: id, EdgeType: edge.TypeDependsOn, FromURN: from, ToURN: to, EdgeMeta: d.edgeMeta()},
			Declared: true,
		})
	default:
		// Edge desconhecida: ignora (forward-compat).
	}
}

// ---------- helpers ----------

func locFrom(s SourceLocation) node.Location {
	return node.Location{
		File:     s.File,
		LineInit: s.Line,
		ColInit:  s.Column,
	}
}

func signatureHash(sig string) string {
	if sig == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(sig))
	return hex.EncodeToString(sum[:])
}
