// Arquitetura do sistema vista a partir de um Service raiz.
// Chama `/flow` com a URN canônica do service (depth padrão do
// backend = ∞ por enquanto; refinar em F-031 step 3).
//
// Sub-view de `/services/:repo`. Co-irmã de `ServiceEndpointsPage`.

import { useMemo } from "react";
import { Link, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { getFlow } from "@/api/graph";
import { buildServiceURN } from "@/api/urn";
import { FlowCanvas } from "@/flow/FlowCanvas";
import { layoutFlow } from "@/flow/layout";
import styles from "./EndpointDetailPage.module.css";
import tabStyles from "./ServiceTabs.module.css";

export function ServiceArchitecturePage() {
  const { repo = "" } = useParams();
  const urn = buildServiceURN(repo);

  const { data, isLoading, error } = useQuery({
    queryKey: ["flow", urn],
    queryFn: ({ signal }) => getFlow(urn, undefined, signal),
    enabled: Boolean(repo),
  });

  const layout = useMemo(() => (data ? layoutFlow(data) : null), [data]);

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <Link to="/services" className={styles.back}>← services</Link>
        <span className={styles.route}>{repo}</span>
        <span className={styles.urn}>{urn}</span>
        <nav className={tabStyles.tabs}>
          <Link to={`/services/${repo}/architecture`} className={`${tabStyles.tab} ${tabStyles.active}`}>
            architecture
          </Link>
          <Link to={`/services/${repo}/endpoints`} className={tabStyles.tab}>
            endpoints
          </Link>
        </nav>
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
