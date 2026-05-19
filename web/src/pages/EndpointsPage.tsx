// Lista de Endpoints. Cada linha leva à página de detalhe (/flow).
//
// URN do endpoint contém `:` e `/` — não fazemos encode aqui porque
// react-router preserva o splat e o backend usa `{rest...}`. Não
// usar encodeURIComponent: quebraria a rota no servidor.

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { listEndpoints } from "@/api/graph";
import type { EndpointData, NodeView } from "@/api/types";
import { NodeRow } from "@/components/NodeRow";
import pageStyles from "./Page.module.css";

type MethodVariant = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

function methodVariant(m?: string): MethodVariant | undefined {
  if (!m) return undefined;
  const up = m.toUpperCase();
  if (
    up === "GET" || up === "POST" || up === "PUT" ||
    up === "PATCH" || up === "DELETE"
  ) {
    return up as MethodVariant;
  }
  return undefined;
}

export function EndpointsPage() {
  const [filter, setFilter] = useState("");
  const { data, isLoading, error } = useQuery({
    queryKey: ["endpoints"],
    queryFn: ({ signal }) => listEndpoints(500, signal),
  });

  const results = data?.results ?? [];
  const filtered = useMemo(() => {
    if (!filter.trim()) return results;
    const f = filter.toLowerCase();
    return results.filter((n: NodeView) => {
      const d = n.data as EndpointData;
      return (
        n.urn.toLowerCase().includes(f) ||
        (d.route ?? "").toLowerCase().includes(f) ||
        (d.handler ?? "").toLowerCase().includes(f) ||
        (d.method ?? "").toLowerCase().includes(f)
      );
    });
  }, [results, filter]);

  return (
    <div className={pageStyles.page}>
      <header className={pageStyles.header}>
        <h1 className={pageStyles.title}>Endpoints</h1>
        <span className={pageStyles.subtitle}>
          {data ? `${data.count} total` : ""}
        </span>
      </header>

      <div className={pageStyles.toolbar}>
        <input
          className={pageStyles.input}
          placeholder="filter by route / method / handler"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
      </div>

      {isLoading && <div className={pageStyles.loading}>loading…</div>}
      {error && (
        <div className={pageStyles.error}>
          failed to load: {(error as Error).message}
        </div>
      )}

      {!isLoading && !error && filtered.length === 0 && (
        <div className={pageStyles.empty}>no endpoints</div>
      )}

      <div className={pageStyles.list}>
        {filtered.map((n) => {
          const d = n.data as EndpointData;
          const method = d.method ?? "?";
          const route = d.route ?? "(no route)";
          return (
            <NodeRow
              key={n.urn}
              to={`/endpoints/${n.urn}`}
              tag={method}
              tagVariant={methodVariant(method)}
              primary={route}
              secondary={d.handler ?? n.urn}
              meta={d.framework}
            />
          );
        })}
      </div>
    </div>
  );
}
