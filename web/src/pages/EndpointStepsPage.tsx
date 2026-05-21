// Steps de um Endpoint — sub-grafo derivado de `/flow`
// (service ← module → endpoint → function/call/type/variable/framework)
// renderizado num canvas react-flow.
//
// URN é reconstruída via splat (`*`) porque contém `:` e `/`. O back
// link aponta pra `/services/:repo/endpoints` (extraímos `repo` da
// URN). Antigo: EndpointDetailPage.

import { useMemo } from "react";
import { Link, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { getFlow } from "@/api/graph";
import { repoFromURN } from "@/api/urn";
import type { EndpointData } from "@/api/types";
import { FlowCanvas } from "@/flow/FlowCanvas";
import { layoutFlow } from "@/flow/layout";
import styles from "./EndpointDetailPage.module.css";

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

export function EndpointStepsPage() {
  const params = useParams();
  // Splat traz o resto da URN após `:urn`. Reconstruímos junto.
  const urn = [params.urn, params["*"]].filter(Boolean).join("/");
  const repo = repoFromURN(urn);
  const backTo = repo ? `/services/${repo}/endpoints` : "/services";

  const { data, isLoading, error } = useQuery({
    queryKey: ["flow", urn],
    queryFn: ({ signal }) => getFlow(urn, 4, signal),
    enabled: Boolean(urn),
  });

  const layout = useMemo(() => (data ? layoutFlow(data) : null), [data]);

  const rootEndpoint = useMemo(() => {
    if (!data) return null;
    return data.nodes.endpoint.find((n) => n.urn === data.root) ?? null;
  }, [data]);
  const epData = rootEndpoint?.data as EndpointData | undefined;
  const method = epData?.method ?? "";
  const variant = methodVariant(method);

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <Link to={backTo} className={styles.back}>← endpoints</Link>
        {variant && (
          <span className={`${styles.method} ${styles[variant]}`}>{method}</span>
        )}
        <span className={styles.route}>{epData?.route ?? urn}</span>
        <span className={styles.urn}>{urn}</span>
      </header>

      <div className={styles.canvas}>
        {isLoading && <div className={styles.status}>loading flow…</div>}
        {error && (
          <div className={`${styles.status} ${styles.error}`}>
            failed: {(error as Error).message}
          </div>
        )}
        {layout && (
          <FlowCanvas
            nodes={layout.nodes}
            edges={layout.edges}
            bbox={layout.bbox}
          />
        )}
      </div>
    </div>
  );
}
