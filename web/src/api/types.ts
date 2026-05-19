// Tipos espelhando os JSON views do backend Go
// (`internal/app/graph/controller/views.go`).
//
// Mantemos `data` como `Record<string, unknown>` — cada Kind tem campos
// específicos (Service.repo, Endpoint.method/route, etc.) que acessamos
// via helpers tipados (ver `nodeData.ts`).

export type URN = string;

export interface NodeView<TData = Record<string, unknown>> {
  urn: URN;
  kind: string;
  version: number;
  valid_from: string;
  observed_at: string;
  valid_to?: string;
  confidence: number;
  labels?: string[];
  data: TData;
}

export interface EdgeView<TData = Record<string, unknown>> {
  id: string;
  type: string;
  from: URN;
  to: URN;
  valid_from: string;
  observed_at: string;
  valid_to?: string;
  confidence: number;
  data: TData;
}

export interface SearchResponse {
  results: NodeView[];
  limit: number;
  offset: number;
  next_cursor: string;
}

export interface FlowResponse {
  root: URN;
  nodes: {
    service: NodeView[];
    module: NodeView[];
    endpoint: NodeView[];
    function: NodeView[];
    call: NodeView[];
    type: NodeView[];
    variable: NodeView[];
    framework: NodeView[];
    other: NodeView[];
  };
  edges: EdgeView[];
}

// Acessadores tipados para `data` de cada Kind. Backend serializa via
// tags Go — todos os campos são opcionais aqui porque a versão pode
// não ter o campo (e.g., Endpoint pré-F-018 sem ModuleURN).

export interface ServiceData {
  repo?: string;
  module_path?: string;
  language?: string;
  namespace?: string;
  manifest?: string;
  manifest_type?: string;
  feature_tags?: string[];
}

export interface ModuleData {
  service_urn?: URN;
  parent_urn?: URN;
  namespace?: string;
  short_name?: string;
  path?: string;
  language?: string;
}

export interface EndpointData {
  service_urn?: URN;
  module_urn?: URN;
  method?: string;
  route?: string;
  handler?: string;
  framework?: string;
  location?: { file?: string; line_init?: number; line_end?: number };
}

export interface FunctionData {
  service_urn?: URN;
  module_urn?: URN;
  namespace?: string;
  symbol?: string;
  signature?: string;
  signature_hash?: string;
  exported?: boolean;
  location?: { file?: string; line_init?: number; line_end?: number };
}

export interface CallData {
  kind?: string;
  caller_urn?: URN;
  target_urn?: URN;
  target_symbol?: string;
  framework_urn?: URN;
  ordinal?: number;
  is_dynamic?: boolean;
  location?: { file?: string; line_init?: number };
}

export interface TypeData {
  service_urn?: URN;
  module_urn?: URN;
  namespace?: string;
  symbol?: string;
  kind?: string;
  exported?: boolean;
}

export interface VariableData {
  service_urn?: URN;
  module_urn?: URN;
  namespace?: string;
  symbol?: string;
  type_ref?: string;
  exported?: boolean;
}

export interface FrameworkData {
  ecosystem?: string;
  name?: string;
  version?: string;
}
