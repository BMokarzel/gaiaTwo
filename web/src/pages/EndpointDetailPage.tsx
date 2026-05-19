// Detalhe de Endpoint — busca /flow e renderiza o sub-grafo completo
// (service ← module → endpoint → function/call/type/variable/framework)
// num canvas react-flow.
//
// URN é extraída via splat (`*`) porque contém `:` e `/`. Não
// usar decodeURIComponent — o backend e o react-router já lidam com
// o URN cru.

import { useMemo } from "react";
import { Link, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import {
  Background, BackgroundVariant, Controls, MiniMap, ReactFlow,
} from "@xyflow/react";
import { getFlow } from "@/api/graph";
import type { EndpointData } from "@/api/types";
import { CeNode } from "@/flow/CeNode";
import { layoutFlow } from "@/flow/layout";
import styles from "./EndpointDetailPage.module.css";

const nodeTypes = { ce: CeNode };

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

export function EndpointDetailPage() {
  const params = useParams();
  // O splat vem em params["*"]; em rotas com `:urn/*` o `urn` é
  // só o primeiro segmento, então reconstruímos.
  const urn = [params.urn, params["*"]].filter(Boolean).join("/");

  const { data, isLoading, error } = useQuery({
    queryKey: ["flow", urn],
    queryFn: ({ signal }) => getFlow(urn, 4, signal),
    enabled: Boolean(urn),
  });

  const layout = useMemo(() => (data ? layoutFlow(data) : null), [data]);

  // Encontra o NodeView do endpoint raiz para o cabeçalho.
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
        <Link to="/endpoints" className={styles.back}>← endpoints</Link>
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
          <ReactFlow
            nodes={layout.nodes}
            edges={layout.edges}
            nodeTypes={nodeTypes}
            fitView
            proOptions={{ hideAttribution: true }}
          >
            <Background variant={BackgroundVariant.Dots} gap={20} size={1} />
            <MiniMap pannable zoomable />
            <Controls />
          </ReactFlow>
        )}
      </div>
    </div>
  );
}
