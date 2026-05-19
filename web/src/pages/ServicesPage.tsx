// Lista de Services. Busca via /v1/architecture/nodes?kind=service.
// Filtro client-side por substring sobre repo/module_path/URN.

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { listServices } from "@/api/graph";
import type { NodeView, ServiceData } from "@/api/types";
import { NodeRow } from "@/components/NodeRow";
import pageStyles from "./Page.module.css";

export function ServicesPage() {
  const [filter, setFilter] = useState("");
  const { data, isLoading, error } = useQuery({
    queryKey: ["services"],
    queryFn: ({ signal }) => listServices(200, signal),
  });

  const results = data?.results ?? [];
  const filtered = useMemo(() => {
    if (!filter.trim()) return results;
    const f = filter.toLowerCase();
    return results.filter((n: NodeView) => {
      const d = n.data as ServiceData;
      return (
        n.urn.toLowerCase().includes(f) ||
        (d.repo ?? "").toLowerCase().includes(f) ||
        (d.module_path ?? "").toLowerCase().includes(f)
      );
    });
  }, [results, filter]);

  return (
    <div className={pageStyles.page}>
      <header className={pageStyles.header}>
        <h1 className={pageStyles.title}>Services</h1>
        <span className={pageStyles.subtitle}>
          {data ? `${data.count} total` : ""}
        </span>
      </header>

      <div className={pageStyles.toolbar}>
        <input
          className={pageStyles.input}
          placeholder="filter by repo / module / urn"
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
        <div className={pageStyles.empty}>no services</div>
      )}

      <div className={pageStyles.list}>
        {filtered.map((n) => {
          const d = n.data as ServiceData;
          return (
            <NodeRow
              key={n.urn}
              tag="svc"
              primary={d.repo ? `${d.repo} :: ${d.module_path ?? "."}` : n.urn}
              secondary={n.urn}
              meta={d.language}
            />
          );
        })}
      </div>
    </div>
  );
}
