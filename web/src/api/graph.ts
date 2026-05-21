// Wrappers tipados sobre os endpoints `/v1/architecture/*` do backend.
//
// URNs do costEngine contêm `:` e `/` (e.g.
// `urn:ce:code:acme:service/.`). Precisam ser percent-encoded como path
// segment porque:
//   - Service URN termina em `/.` — `http.ServeMux` do Go (1.22+) faz
//     path cleaning e strip de `/./`, corrompendo a URN.
//   - Browsers também normalizam `/./` antes de enviar.
// `encodeURIComponent` resolve ambos (encoda `/`, `.`, `:` etc.); o
// servidor decoda após roteamento e o handler recebe a URN íntegra.

import { apiGet } from "./client";
import type { FlowResponse, NodeView, SearchResponse } from "./types";

function encURN(urn: string): string {
  return encodeURIComponent(urn);
}

export function getNode(urn: string, signal?: AbortSignal) {
  return apiGet<NodeView>(`/v1/architecture/nodes/${encURN(urn)}`, signal);
}

export function getNeighbors(
  urn: string,
  opts?: { depth?: number; dir?: "in" | "out" | "any"; edgeTypes?: string[] },
  signal?: AbortSignal,
) {
  const q = new URLSearchParams();
  if (opts?.depth) q.set("depth", String(opts.depth));
  if (opts?.dir) q.set("dir", opts.dir);
  if (opts?.edgeTypes?.length) q.set("edge_types", opts.edgeTypes.join(","));
  const qs = q.toString();
  return apiGet<{ edges: unknown[]; nodes: NodeView[] }>(
    `/v1/architecture/nodes/${encURN(urn)}/neighbors${qs ? `?${qs}` : ""}`,
    signal,
  );
}

export function getFlow(urn: string, depth?: number, signal?: AbortSignal) {
  const qs = depth ? `?depth=${depth}` : "";
  return apiGet<FlowResponse>(`/v1/architecture/nodes/${encURN(urn)}/flow${qs}`, signal);
}

export function searchNodes(
  q: string,
  kind?: string,
  opts?: { limit?: number; cursor?: string },
  signal?: AbortSignal,
) {
  const params = new URLSearchParams({ q });
  if (kind) params.set("kind", kind);
  if (opts?.limit) params.set("limit", String(opts.limit));
  if (opts?.cursor) params.set("cursor", opts.cursor);
  return apiGet<SearchResponse>(
    `/v1/architecture/search?${params.toString()}`,
    signal,
  );
}

// Listagem por Kind via `/v1/architecture/nodes?kind=`. Search exige
// `q` não-vazio, então usamos este path para popular dropdowns/listas.
export interface ListNodesResponse {
  results: NodeView[];
  limit: number;
  offset: number;
  count: number;
}

export function listNodes(
  opts?: { kind?: string; limit?: number; offset?: number },
  signal?: AbortSignal,
) {
  const params = new URLSearchParams();
  if (opts?.kind) params.set("kind", opts.kind);
  if (opts?.limit) params.set("limit", String(opts.limit));
  if (opts?.offset) params.set("offset", String(opts.offset));
  const qs = params.toString();
  return apiGet<ListNodesResponse>(
    `/v1/architecture/nodes${qs ? `?${qs}` : ""}`,
    signal,
  );
}

export function listServices(limit = 100, signal?: AbortSignal) {
  return listNodes({ kind: "service", limit }, signal);
}

export function listEndpoints(limit = 200, signal?: AbortSignal) {
  return listNodes({ kind: "endpoint", limit }, signal);
}
