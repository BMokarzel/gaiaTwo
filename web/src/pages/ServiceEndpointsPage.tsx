// Endpoints de um Service. Lista filtrada por `service_urn` no cliente
// (backend ainda não expõe filtro server-side; ver F-031 gaps).
//
// Co-irmã de `ServiceArchitecturePage` sob `/services/:repo`.

import { useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { listEndpoints } from "@/api/graph";
import { buildServiceURN } from "@/api/urn";
import type { EndpointData, NodeView } from "@/api/types";
import { NodeRow } from "@/components/NodeRow";
import pageStyles from "./Page.module.css";
import tabStyles from "./ServiceTabs.module.css";
import headerStyles from "./EndpointDetailPage.module.css";

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

export function ServiceEndpointsPage() {
  const { repo = "" } = useParams();
  const serviceURN = buildServiceURN(repo);
  const [filter, setFilter] = useState("");

  const { data, isLoading, error } = useQuery({
    queryKey: ["endpoints", repo],
    queryFn: ({ signal }) => listEndpoints(500, signal),
    enabled: Boolean(repo),
  });

  const ofService = useMemo(() => {
    const all = data?.results ?? [];
    return all.filter((n) => {
      const d = n.data as EndpointData;
      return d.service_urn === serviceURN;
    });
  }, [data, serviceURN]);

  const filtered = useMemo(() => {
    if (!filter.trim()) return ofService;
    const f = filter.toLowerCase();
    return ofService.filter((n: NodeView) => {
      const d = n.data as EndpointData;
      return (
        n.urn.toLowerCase().includes(f) ||
        (d.route ?? "").toLowerCase().includes(f) ||
        (d.handler ?? "").toLowerCase().includes(f) ||
        (d.method ?? "").toLowerCase().includes(f)
      );
    });
  }, [ofService, filter]);

  return (
    <div className={pageStyles.page}>
      <header className={headerStyles.header}>
        <Link to="/services" className={headerStyles.back}>← services</Link>
        <span className={headerStyles.route}>{repo}</span>
        <span className={headerStyles.urn}>{serviceURN}</span>
        <nav className={tabStyles.tabs}>
          <Link to={`/services/${repo}/architecture`} className={tabStyles.tab}>
            architecture
          </Link>
          <Link to={`/services/${repo}/endpoints`} className={`${tabStyles.tab} ${tabStyles.active}`}>
            endpoints
          </Link>
        </nav>
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
              to={`/services/${repo}/endpoints/${n.urn}`}
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
