// Layout hierárquico via dagre.
//
// Antes o layout era determinístico por kind (uma coluna por
// service/module/endpoint/function/...). Isso "agrupava" os nós por
// tipo e ignorava o fluxo real das edges, produzindo um arranjo que
// não comunicava a estrutura do sub-grafo de um endpoint.
//
// Agora usamos dagre (layered/Sugiyama) com rank "LR": cada nó é
// posicionado pelo seu nível topológico em relação às edges, e o
// arranjo respeita a direção do fluxo (service/module → endpoint →
// function → call → type/...). Para grafos curados de um endpoint
// específico (<200 nós) o resultado é nítido e previsível.

import dagre from "@dagrejs/dagre";
import type { Edge, Node } from "@xyflow/react";
import type { FlowResponse, NodeView } from "@/api/types";

// Dimensões nominais do node-box no canvas. Precisam casar com o
// que o CeNode renderiza (ver CeNode.module.css) — dagre usa isso
// para reservar espaço entre nós sem sobreposição.
const NODE_WIDTH = 240;
const NODE_HEIGHT = 70;

// Espaçamentos do dagre. rankSep = distância entre níveis (eixo do
// rank); nodeSep = distância entre nós do mesmo nível.
const RANK_SEP = 90;
const NODE_SEP = 28;

// Ordem dos kinds usada apenas para iterar o objeto de resposta —
// não influencia mais a posição (dagre cuida disso).
const KINDS: (keyof FlowResponse["nodes"])[] = [
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

export interface FlowNodeData extends Record<string, unknown> {
  kind: string;
  view: NodeView;
  isRoot: boolean;
}

// DagreNodeMeta é o que pendamos na meta de cada nó dentro do grafo
// dagre. width/height são lidos pelo solver; os demais campos são
// nosso "carry-over" para montar a saída do react-flow.
interface DagreNodeMeta {
  width: number;
  height: number;
  kind: string;
  view: NodeView;
  isRoot: boolean;
  // Preenchido por dagre.layout — centro do node-box.
  x?: number;
  y?: number;
}

export function layoutFlow(
  resp: FlowResponse,
): { nodes: Node<FlowNodeData>[]; edges: Edge[] } {
  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({
    rankdir: "LR",
    ranksep: RANK_SEP,
    nodesep: NODE_SEP,
    marginx: 24,
    marginy: 24,
  });

  // 1) Registra todos os nós no grafo dagre com data anexada (vamos
  //    reaproveitar depois ao montar a saída para o react-flow).
  for (const kind of KINDS) {
    const list = resp.nodes[kind] ?? [];
    for (const view of list) {
      g.setNode(view.urn, {
        width: NODE_WIDTH,
        height: NODE_HEIGHT,
        kind,
        view,
        isRoot: view.urn === resp.root,
      });
    }
  }

  // 2) Registra as edges. dagre exige que os endpoints já existam
  //    como nós — se vier uma edge para URN que não está em
  //    resp.nodes (caso anômalo), é silenciosamente ignorada aqui.
  for (const e of resp.edges) {
    if (g.hasNode(e.from) && g.hasNode(e.to)) {
      g.setEdge(e.from, e.to);
    }
  }

  dagre.layout(g);

  const nodes: Node<FlowNodeData>[] = g.nodes().map((id) => {
    const meta = g.node(id) as DagreNodeMeta;
    // dagre devolve o CENTRO do node-box; react-flow espera o
    // canto superior-esquerdo, então subtraímos metade da dimensão.
    return {
      id,
      type: "ce",
      position: {
        x: (meta.x ?? 0) - NODE_WIDTH / 2,
        y: (meta.y ?? 0) - NODE_HEIGHT / 2,
      },
      data: {
        kind: meta.kind,
        view: meta.view,
        isRoot: meta.isRoot,
      },
    };
  });

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
