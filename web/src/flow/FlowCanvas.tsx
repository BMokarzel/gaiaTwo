// FlowCanvas — wrapper sobre <ReactFlow> com:
//   - zoom/pan desabilitados; o scroll nativo do `.scrollArea` faz a
//     navegação espacial (sem viewport interno do RF).
//   - tamanho do `.stage` = bbox computado pelo layout (sem fitView).
//   - hover highlight: nó hovered + vizinhos imediatos (1 hop) mantêm
//     opacidade normal; resto fica dim. Edges incidentes coloridas.
//   - click em nó abre <DetailPanel/> à direita (Esc fecha).
//
// Dedupe entre ServiceArchitecturePage e EndpointStepsPage.

import { useCallback, useMemo, useState } from "react";
import {
  Background, BackgroundVariant, ReactFlow,
  type Edge, type Node, type NodeMouseHandler,
} from "@xyflow/react";
import type { NodeView } from "@/api/types";
import { nodeTypes } from "./nodes";
import { DetailPanel } from "./DetailPanel";
import type { FlowNodeData } from "./layout";
import styles from "./FlowCanvas.module.css";

interface Props {
  nodes: Node[];
  edges: Edge[];
  bbox: { width: number; height: number };
}

export function FlowCanvas({ nodes: baseNodes, edges: baseEdges, bbox }: Props) {
  const [hoveredId, setHoveredId] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);

  // Vizinhos = {hoveredId} ∪ {nós conectados por uma edge}.
  const neighbors = useMemo(() => {
    if (!hoveredId) return null;
    const set = new Set<string>([hoveredId]);
    for (const e of baseEdges) {
      if (e.source === hoveredId) set.add(e.target);
      if (e.target === hoveredId) set.add(e.source);
    }
    return set;
  }, [hoveredId, baseEdges]);

  const nodes = useMemo(() => {
    if (!neighbors) return baseNodes;
    return baseNodes.map((n) => ({
      ...n,
      className: neighbors.has(n.id) ? styles.hot : styles.dim,
    }));
  }, [baseNodes, neighbors]);

  const edges = useMemo(() => {
    if (!hoveredId) return baseEdges;
    return baseEdges.map((e) => {
      const incident = e.source === hoveredId || e.target === hoveredId;
      return {
        ...e,
        className: incident ? styles.edgeHot : styles.edgeDim,
      };
    });
  }, [baseEdges, hoveredId]);

  const onEnter: NodeMouseHandler = (_, n) => setHoveredId(n.id);
  const onLeave: NodeMouseHandler = () => setHoveredId(null);
  const onClick: NodeMouseHandler = useCallback((_, n) => {
    setSelectedId(n.id);
  }, []);
  const closePanel = useCallback(() => setSelectedId(null), []);

  // Resolve nó selecionado para alimentar o painel (NodeView + kind).
  const selected = useMemo(() => {
    if (!selectedId) return null;
    const n = baseNodes.find((x) => x.id === selectedId);
    if (!n) return null;
    const d = n.data as FlowNodeData;
    return { view: d.view, kind: d.kind };
  }, [selectedId, baseNodes]);

  return (
    <div className={styles.layout}>
      <div className={styles.scrollArea}>
        <div
          className={styles.stage}
          style={{
            width: bbox.width + 64,
            height: bbox.height + 64,
          }}
        >
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            proOptions={{ hideAttribution: true }}
            panOnDrag={false}
            panOnScroll={false}
            zoomOnScroll={false}
            zoomOnPinch={false}
            zoomOnDoubleClick={false}
            preventScrolling={false}
            defaultViewport={{ x: 32, y: 32, zoom: 1 }}
            nodesDraggable
            onNodeMouseEnter={onEnter}
            onNodeMouseLeave={onLeave}
            onNodeClick={onClick}
          >
            <Background variant={BackgroundVariant.Dots} gap={20} size={1} />
          </ReactFlow>
        </div>
      </div>
      {selected && (
        <DetailPanel
          view={selected.view as NodeView}
          kind={selected.kind}
          onClose={closePanel}
        />
      )}
    </div>
  );
}
