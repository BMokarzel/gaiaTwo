// Layout determinístico simples para grafos pequenos (<200 nós).
// Agrupamos nós em colunas por Kind, na ordem natural de fluxo
// (service → module → endpoint → function → call → type/variable →
// framework → other). Dentro de cada coluna empilhamos verticalmente.
//
// Não é dagre porque (a) o objetivo é exibir um sub-grafo curado
// de um único endpoint, e (b) garantimos legibilidade sem precisar
// resolver layered placement. Trocar por dagre se vier a precisar.

import type { Edge, Node } from "@xyflow/react";
import type { FlowResponse, NodeView } from "@/api/types";

const COLUMN_ORDER: (keyof FlowResponse["nodes"])[] = [
  "service",
  "module",
  "endpoint",
  "function",
  "call",
  "type",
  "variable",
  "framework",
  "other",
];

const COL_WIDTH = 280;
const ROW_HEIGHT = 70;
const ROW_GAP = 16;

export interface FlowNodeData extends Record<string, unknown> {
  kind: string;
  view: NodeView;
  isRoot: boolean;
}

export function layoutFlow(
  resp: FlowResponse,
): { nodes: Node<FlowNodeData>[]; edges: Edge[] } {
  const nodes: Node<FlowNodeData>[] = [];

  for (const kind of COLUMN_ORDER) {
    const list = resp.nodes[kind] ?? [];
    const col = COLUMN_ORDER.indexOf(kind);
    list.forEach((view, idx) => {
      nodes.push({
        id: view.urn,
        type: "ce",
        position: {
          x: col * COL_WIDTH,
          y: idx * (ROW_HEIGHT + ROW_GAP),
        },
        data: {
          kind,
          view,
          isRoot: view.urn === resp.root,
        },
      });
    });
  }

  const edges: Edge[] = resp.edges.map((e) => ({
    id: e.id,
    source: e.from,
    target: e.to,
    label: e.type,
    type: "smoothstep",
    animated: false,
  }));

  return { nodes, edges };
}
