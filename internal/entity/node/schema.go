package node

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// SchemaFormat classifica o formato externo do contrato (F-022).
type SchemaFormat string

const (
	SchemaProto    SchemaFormat = "proto"
	SchemaOpenAPI  SchemaFormat = "openapi"
	SchemaJSON     SchemaFormat = "json-schema"
	SchemaAvro     SchemaFormat = "avro"
	SchemaGraphQL  SchemaFormat = "graphql"
)

// Schema representa um contrato externo (proto message, OpenAPI
// components.schema, JSON Schema, Avro record, GraphQL type).
// Distinto de Type — Schema é o contrato declarado fora do código de
// uma linguagem; Type é o resultado da geração / declaração no código.
// Pontuados pela aresta SERIALIZES_AS (F-022).
//
// URN: urn:ce:code:<repo>:schema/<service-module-path>!<schema-id>
//
// Ou `urn:ce:code:_global:schema/<schema-id>` para contratos compartilhados
// entre repos (proto package globais).
type Schema struct {
	Base
	ServiceURN URN          `json:"service_urn,omitempty"` // vazio = schema global
	Format     SchemaFormat `json:"format"`
	Namespace  string       `json:"namespace,omitempty"` // proto package, OpenAPI tag…
	Symbol     string       `json:"symbol"`
	Version    string       `json:"version,omitempty"`
	SourceFile string       `json:"source_file,omitempty"` // path no repo

	Fields []FieldSlot `json:"fields,omitempty"`

	FeatureTags []string `json:"feature_tags,omitempty"`
}

// NewSchemaURN produz a URN canônica para um Schema.
func NewSchemaURN(repo, serviceModulePath, schemaID string) URN {
	if repo == "" {
		repo = "_global"
	}
	if serviceModulePath == "" {
		serviceModulePath = "."
	}
	return NewURN(ProviderCode, repo, KindSchema, serviceModulePath+"!"+schemaID)
}

// ContentHash dos campos significativos.
func (s Schema) ContentHash() string {
	var sb strings.Builder
	sb.WriteString(string(s.Format))
	sb.WriteByte('|')
	sb.WriteString(s.Namespace)
	sb.WriteByte('|')
	sb.WriteString(s.Symbol)
	sb.WriteByte('|')
	sb.WriteString(s.Version)
	sb.WriteByte('|')
	for _, f := range s.Fields {
		sb.WriteString(f.Name)
		sb.WriteByte(':')
		sb.WriteString(f.TypeRef)
		sb.WriteByte(',')
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
