// Espelha `internal/modules/code/collector/typescript/proto.go`.
// MUDOU AQUI? Atualize o Go-side ou bump o $schema.

export const SCHEMA = "v1";

export interface ConfigEnvelope {
  $schema: string;
  root: string;
  repo: string;
  run_id: string;
  observed_at: string;
  skip_dirs?: string[];
  verbose?: boolean;
}

export type EventKind =
  | "init"
  | "service"
  | "module"
  | "endpoint"
  | "function"
  | "call"
  | "type"
  | "variable"
  | "framework"
  | "edge"
  | "progress"
  | "done"
  | "error";

export interface SourceLocation {
  file: string;
  line: number;
  column?: number;
}

export interface ServicePayload {
  slug: string;
  module_path: string;
  namespace: string;
  manifest: string;
  manifest_type: string;
  language: string;
  tags?: Record<string, string>;
  feature_tags?: string[];
}

export interface ModulePayload {
  service_module_path: string;
  namespace: string;
  path: string;
}

export interface EndpointPayload {
  service_module_path: string;
  module_namespace: string;
  method: string;
  path: string;
  framework: string;
  handler_symbol?: string;
  location: SourceLocation;
  tags?: Record<string, string>;
}

export interface FunctionPayload {
  service_module_path: string;
  module_namespace: string;
  namespace: string;
  symbol: string;
  receiver?: string;
  exported: boolean;
  signature_hash: string;
  signature: string;
  location: SourceLocation;
}

export interface CallPayload {
  service_module_path: string;
  from_function_urn: string;
  subkind: "http!" | "db!" | "mq!" | "in-process" | "unresolved!";
  callee_expression: string;
  target_hint?: string;
  framework_name?: string;
  location: SourceLocation;
}

export interface TypePayload {
  service_module_path: string;
  module_namespace: string;
  namespace: string;
  symbol: string;
  kind: "interface" | "type" | "class" | "enum";
  exported: boolean;
  location: SourceLocation;
}

export interface VariablePayload {
  service_module_path: string;
  module_namespace: string;
  symbol: string;
  type_text?: string;
  exported: boolean;
  location: SourceLocation;
}

export interface FrameworkPayload {
  ecosystem: string;
  name: string;
  latest_version: string;
  is_dev_only: boolean;
}

export interface EdgePayload {
  type: "Invokes" | "Targets" | "Uses" | "Extends" | "Aliases" | "Contains" | "DependsOn";
  from_urn: string;
  to_urn: string;
  subkind?: string;
  strength?: number;
}

export interface DonePayload {
  services: number;
  modules: number;
  endpoints: number;
  functions: number;
  calls: number;
  types: number;
  variables: number;
  elapsed_ns: number;
}

export interface ErrorPayload {
  code: string;
  message: string;
  where?: string;
}

export interface InitPayload {
  sidecar_version: string;
  node_version: string;
  ts_morph_version?: string;
}
