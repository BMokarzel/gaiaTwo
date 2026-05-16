package n4j

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"costEngine/internal/entity/edge"
	"costEngine/internal/entity/node"
)

// nodeProps achata um node.Node em props Cypher.
//
// Top-level (queryáveis em índices): urn, kind, version, valid_from,
// valid_to, observed_at, source_collector, source_run_id, source_method,
// confidence, provider, account, region, external_id.
// JSON serializado: labels_json, properties_json, tags_json, spec_json,
// lineage_json, type_specific_json.
//
// Retorna também a primaryLabel (== Kind) para CREATE/MERGE.
func nodeProps(n node.Node) (primaryLabel string, props map[string]any, err error) {
	m := n.Meta()
	props = map[string]any{
		"urn":              string(n.URN()),
		"kind":             string(n.Kind()),
		"version":          int64(m.Version),
		"valid_from":       m.ValidFrom,
		"observed_at":      m.ObservedAt,
		"source_collector": m.Source.Collector,
		"source_run_id":    m.Source.RunID,
		"source_method":    string(m.Source.Method),
		"confidence":       float64(m.Confidence),
	}
	if m.ValidTo != nil {
		props["valid_to"] = *m.ValidTo
	} else {
		props["valid_to"] = nil
	}

	if len(m.Labels) > 0 {
		b, _ := json.Marshal(m.Labels)
		props["labels_json"] = string(b)
	}
	if len(m.Properties) > 0 {
		b, _ := json.Marshal(m.Properties)
		props["properties_json"] = string(b)
	}
	if len(m.Lineage.DerivedFrom) > 0 || m.Lineage.Rule != "" {
		b, _ := json.Marshal(m.Lineage)
		props["lineage_json"] = string(b)
	}

	if res, ok := n.(node.Resource); ok {
		props["provider"] = string(res.Provider())
		props["account"] = string(res.AccountURN())
		props["region"] = string(res.RegionURN())
		props["external_id"] = res.ExternalID()
		// F-003/S-002: se externalID for ARN, gravar short_id derivado
		// para suportar lookup por short ID (CUR mistura formas).
		if short := shortIDFromExternal(res.ExternalID()); short != "" {
			props["short_id"] = short
		}
		if tags := res.NativeTags(); len(tags) > 0 {
			b, _ := json.Marshal(tags)
			props["tags_json"] = string(b)
		}
		if spec := res.Spec(); len(spec) > 0 {
			b, _ := json.Marshal(spec)
			props["spec_json"] = string(b)
		}
	}

	// Serializa o struct concreto inteiro para o "type_specific_json".
	// É a forma simples e estável de reconstruir o tipo concreto na leitura.
	b, err := json.Marshal(n)
	if err != nil {
		return "", nil, fmt.Errorf("n4j: marshal node: %w", err)
	}
	props["type_specific_json"] = string(b)

	return string(n.Kind()), props, nil
}

// edgeProps achata uma edge.Edge em props Cypher.
func edgeProps(e edge.Edge) map[string]any {
	m := e.Meta()
	props := map[string]any{
		"id":            e.ID(),
		"type":          string(e.Type()),
		"from_urn":      string(e.From()),
		"to_urn":        string(e.To()),
		"valid_from":    m.ValidFrom,
		"observed_at":   m.ObservedAt,
		"source_collector": m.Source.Collector,
		"source_run_id":    m.Source.RunID,
		"source_method":    string(m.Source.Method),
		"confidence":    float64(m.Confidence),
		"directional":   m.Directional,
		"weight":        m.Weight,
	}
	if m.ValidTo != nil {
		props["valid_to"] = *m.ValidTo
	} else {
		props["valid_to"] = nil
	}
	if len(m.Properties) > 0 {
		b, _ := json.Marshal(m.Properties)
		props["properties_json"] = string(b)
	}
	if len(m.Lineage.DerivedFrom) > 0 || m.Lineage.Rule != "" {
		b, _ := json.Marshal(m.Lineage)
		props["lineage_json"] = string(b)
	}

	b, _ := json.Marshal(e)
	props["type_specific_json"] = string(b)
	return props
}

// decodeNode reconstrói o tipo concreto a partir de type_specific_json.
//
// A reconstrução é guiada pelo campo "kind" para escolher o struct alvo.
// Quaisquer kinds desconhecidos retornam erro.
func decodeNode(props map[string]any) (node.Node, error) {
	kindRaw, _ := props["kind"].(string)
	rawJSON, _ := props["type_specific_json"].(string)
	if rawJSON == "" {
		return nil, fmt.Errorf("n4j: missing type_specific_json")
	}
	switch node.Kind(kindRaw) {
	case node.KindCompute:
		var v node.Compute
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	case node.KindPersistence:
		var v node.Persistence
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	case node.KindMessaging:
		var v node.Messaging
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	case node.KindNetwork:
		var v node.Network
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	case node.KindProvider:
		var v node.Provider
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	case node.KindAccount:
		var v node.Account
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	case node.KindRegion:
		var v node.Region
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	case node.KindZone:
		var v node.Zone
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	case node.KindEnvironment:
		var v node.Environment
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	default:
		return nil, fmt.Errorf("n4j: unknown kind %q", kindRaw)
	}
}

// decodeEdge reconstrói o tipo concreto a partir de type_specific_json.
func decodeEdge(props map[string]any) (edge.Edge, error) {
	typeRaw, _ := props["type"].(string)
	rawJSON, _ := props["type_specific_json"].(string)
	if rawJSON == "" {
		return nil, fmt.Errorf("n4j: missing type_specific_json on edge")
	}
	switch edge.Type(typeRaw) {
	case edge.TypeContains:
		var v edge.Contains
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	case edge.TypeDeployedOn:
		var v edge.DeployedOn
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	case edge.TypeAttachedTo:
		var v edge.AttachedTo
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	case edge.TypeRoutes:
		var v edge.Routes
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	case edge.TypePeers:
		var v edge.Peers
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	case edge.TypeDependsOn:
		var v edge.DependsOn
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	case edge.TypeCommunicatesWith:
		var v edge.CommunicatesWith
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	case edge.TypeReplaces:
		var v edge.Replaces
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	case edge.TypeOwns:
		var v edge.Owns
		if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
			return nil, err
		}
		return v, nil
	default:
		return nil, fmt.Errorf("n4j: unknown edge type %q", typeRaw)
	}
}

// nowFn permite injeção em testes.
var nowFn = time.Now

// shortIDFromExternal extrai o ID nativo (último segmento) de um ARN AWS.
// Retorna "" se o input não é ARN. Mesma semântica da impl em memory.
//
// Exemplos:
//
//	arn:aws:ec2:us-east-1:111:instance/i-abc → "i-abc"
//	arn:aws:s3:::my-bucket                   → "my-bucket"
//	arn:aws:rds:us-east-1:111:db:prod-db     → "prod-db"
func shortIDFromExternal(eid string) string {
	if !strings.HasPrefix(eid, "arn:") {
		return ""
	}
	if i := strings.LastIndex(eid, "/"); i >= 0 && i < len(eid)-1 {
		return eid[i+1:]
	}
	if i := strings.LastIndex(eid, ":"); i >= 0 && i < len(eid)-1 {
		return eid[i+1:]
	}
	return ""
}
