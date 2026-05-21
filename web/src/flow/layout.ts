// Layout hierárquico via dagre, **recursivo** com containers.
//
// Os Kinds têm relações naturais de contenção (Service → Module →
// Endpoint/Function/Type/Variable → Call). Cada container vira um
// node RF de tipo "container" e seus filhos recebem `parentId`
// + `extent: 'parent'` + `expandParent: true`.
//
// Estratégia:
//   1) Derivar parentOf por inspeção de NodeView.data (mais barato
//      e robusto que reconstruir de edges).
//   2) Para cada container, fazer um layout dagre só com seus filhos
//      diretos e edges siblings. Tamanho do container = bbox dos
//      filhos + padding + header.
//   3) Recursão bottom-up: filhos são posicionados primeiro (porque
//      o tamanho de um container depende do tamanho dos filhos
//      containerizados também).
//   4) Posições no react-flow são RELATIVAS ao parent quando há
//      parentId; senão absolutas.

import dagre from "@dagrejs/dagre";
import type { Edge, Node } from "@xyflow/react";
import type {
  CallData, EndpointData, FlowResponse, FunctionData, ModuleData,
  NodeView, TypeData, VariableData,
} from "@/api/types";
import { NODE_DIMS, resolveNodeType } from "./nodes";

// Espaçamentos do dagre. rankSep = distância entre níveis;
// nodeSep = distância entre nós irmãos.
const RANK_SEP = 80;
const NODE_SEP = 24;

// Padding interno do container (em torno dos filhos) e altura da
// faixa do header. Casam com `nodes/shapes.module.css`.
const CONTAINER_PADDING = 20;
const CONTAINER_HEADER = 28;

export interface FlowNodeData extends Record<string, unknown> {
  kind: string;
  view: NodeView;
  isRoot: boolean;
}

const ALL_KINDS: (keyof FlowResponse["nodes"])[] = [
  "service", "module", "endpoint", "function",
  "call", "type", "variable", "framework", "other",
];

// Indexa todos os nós por URN e materializa kind.
function indexNodes(resp: FlowResponse): Map<string, { view: NodeView; kind: string }> {
  const idx = new Map<string, { view: NodeView; kind: string }>();
  for (const kind of ALL_KINDS) {
    for (const view of resp.nodes[kind] ?? []) {
      idx.set(view.urn, { view, kind });
    }
  }
  return idx;
}

// Deriva o parent de cada nó a partir dos campos de NodeView.data.
// Só retorna parents que existem no payload (evita criar parentId
// "fantasma" que o RF rejeitaria).
function buildParentMap(
  resp: FlowResponse,
  index: Map<string, { view: NodeView; kind: string }>,
): Map<string, string> {
  const parent = new Map<string, string>();
  const has = (urn?: string): urn is string => Boolean(urn) && index.has(urn!);

  for (const m of resp.nodes.module ?? []) {
    const d = m.data as ModuleData;
    const p = has(d.parent_urn) ? d.parent_urn
            : has(d.service_urn) ? d.service_urn : undefined;
    if (p) parent.set(m.urn, p);
  }
  for (const e of resp.nodes.endpoint ?? []) {
    const d = e.data as EndpointData;
    const p = has(d.module_urn) ? d.module_urn
            : has(d.service_urn) ? d.service_urn : undefined;
    if (p) parent.set(e.urn, p);
  }
  for (const f of resp.nodes.function ?? []) {
    const d = f.data as FunctionData;
    const p = has(d.module_urn) ? d.module_urn
            : has(d.service_urn) ? d.service_urn : undefined;
    if (p) parent.set(f.urn, p);
  }
  for (const t of resp.nodes.type ?? []) {
    const d = t.data as TypeData;
    const p = has(d.module_urn) ? d.module_urn
            : has(d.service_urn) ? d.service_urn : undefined;
    if (p) parent.set(t.urn, p);
  }
  for (const v of resp.nodes.variable ?? []) {
    const d = v.data as VariableData;
    const p = has(d.module_urn) ? d.module_urn
            : has(d.service_urn) ? d.service_urn : undefined;
    if (p) parent.set(v.urn, p);
  }
  for (const c of resp.nodes.call ?? []) {
    const d = c.data as CallData;
    // Call mora dentro da function que a chama.
    if (has(d.caller_urn)) parent.set(c.urn, d.caller_urn!);
  }
  // Framework e Service: sem parent (top-level).
  return parent;
}

// Posição relativa de um filho dentro de seu grupo (ou absoluta no
// nível raiz). Coords são top-left, não centro.
interface ChildPos {
  urn: string;
  // posição RELATIVA ao parent (top-left)
  x: number;
  y: number;
  width: number;
  height: number;
  isContainer: boolean;
}

export function layoutFlow(
  resp: FlowResponse,
): { nodes: Node<FlowNodeData>[]; edges: Edge[]; bbox: { width: number; height: number } } {
  const index = indexNodes(resp);
  const parentOf = buildParentMap(resp, index);

  // Lista de filhos diretos por parent.
  const childrenOf = new Map<string, string[]>();
  for (const [child, parent] of parentOf) {
    if (!childrenOf.has(parent)) childrenOf.set(parent, []);
    childrenOf.get(parent)!.push(child);
  }

  // Conjunto de URNs top-level (sem parent).
  const topLevel: string[] = [];
  for (const urn of index.keys()) {
    if (!parentOf.has(urn)) topLevel.push(urn);
  }

  // Resultado acumulado: para cada URN, sua posição absoluta ou
  // relativa (definida durante a recursão), o size final e se é
  // container.
  const sizes = new Map<string, { width: number; height: number }>();
  const relPos = new Map<string, { x: number; y: number }>();
  const isContainer = new Set<string>();

  // layoutGroup: dada uma lista de URNs irmãs, posiciona-as via
  // dagre e devolve o bbox total. Posições saem relativas ao
  // canto superior-esquerdo do bbox (positivas).
  function layoutGroup(siblings: string[]): { width: number; height: number; positions: ChildPos[] } {
    if (siblings.length === 0) {
      return { width: 0, height: 0, positions: [] };
    }

    const g = new dagre.graphlib.Graph();
    g.setDefaultEdgeLabel(() => ({}));
    g.setGraph({
      rankdir: "LR",
      ranksep: RANK_SEP,
      nodesep: NODE_SEP,
      marginx: 0,
      marginy: 0,
    });

    // Para cada irmã, garante que seu size já está computado:
    // - se for container (tem filhos), recurse primeiro.
    // - senão, é uma folha → usa NODE_DIMS do shape correspondente.
    for (const urn of siblings) {
      let w: number, h: number;
      if (childrenOf.has(urn)) {
        const inner = layoutGroup(childrenOf.get(urn)!);
        // Container wrap: padding lateral + header em cima.
        w = inner.width + CONTAINER_PADDING * 2;
        h = inner.height + CONTAINER_PADDING * 2 + CONTAINER_HEADER;
        sizes.set(urn, { width: w, height: h });
        isContainer.add(urn);
        // posições dos filhos passam a ser relativas a este container
        for (const p of inner.positions) {
          relPos.set(p.urn, {
            x: p.x + CONTAINER_PADDING,
            y: p.y + CONTAINER_PADDING + CONTAINER_HEADER,
          });
        }
      } else {
        const meta = index.get(urn);
        const shape = resolveNodeType(meta?.kind ?? "other");
        const dims = NODE_DIMS[shape];
        w = dims.width;
        h = dims.height;
        sizes.set(urn, { width: w, height: h });
      }
      g.setNode(urn, { width: w, height: h });
    }

    // Edges entre irmãs apenas (atravessam container? ignoramos —
    // dagre não suporta nested layout, e edges intercontainer ficam
    // a cargo do RF que roteia sozinho).
    const sibSet = new Set(siblings);
    for (const e of resp.edges) {
      if (sibSet.has(e.from) && sibSet.has(e.to)) {
        g.setEdge(e.from, e.to);
      }
    }

    dagre.layout(g);

    // Normalizar: dagre devolve centro do node. Achar bbox e
    // re-centralizar pra origem em (0,0).
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    for (const urn of siblings) {
      const n = g.node(urn);
      const s = sizes.get(urn)!;
      const left = n.x - s.width / 2;
      const top = n.y - s.height / 2;
      if (left < minX) minX = left;
      if (top < minY) minY = top;
      if (left + s.width > maxX) maxX = left + s.width;
      if (top + s.height > maxY) maxY = top + s.height;
    }

    const positions: ChildPos[] = siblings.map((urn) => {
      const n = g.node(urn);
      const s = sizes.get(urn)!;
      return {
        urn,
        x: (n.x - s.width / 2) - minX,
        y: (n.y - s.height / 2) - minY,
        width: s.width,
        height: s.height,
        isContainer: isContainer.has(urn),
      };
    });

    return {
      width: maxX - minX,
      height: maxY - minY,
      positions,
    };
  }

  // Layout do nível raiz (top-level). Posições retornadas viram
  // absolutas (sem parent).
  const root = layoutGroup(topLevel);
  for (const p of root.positions) {
    relPos.set(p.urn, { x: p.x, y: p.y });
  }

  // Monta RF nodes na ordem: containers antes dos filhos (RF exige).
  // Como containerOf pode ser aninhado, ordenamos por profundidade.
  function depth(urn: string): number {
    let d = 0;
    let cur: string | undefined = urn;
    while (cur && parentOf.has(cur)) {
      d++;
      cur = parentOf.get(cur);
    }
    return d;
  }

  const allUrns = Array.from(index.keys());
  allUrns.sort((a, b) => depth(a) - depth(b));

  const nodes: Node<FlowNodeData>[] = allUrns.map((urn) => {
    const meta = index.get(urn)!;
    const pos = relPos.get(urn) ?? { x: 0, y: 0 };
    const size = sizes.get(urn) ?? NODE_DIMS.rect;
    const parent = parentOf.get(urn);
    const isCont = isContainer.has(urn);
    const type = isCont ? "container" : resolveNodeType(meta.kind);

    const rfNode: Node<FlowNodeData> = {
      id: urn,
      type,
      position: { x: pos.x, y: pos.y },
      data: {
        kind: meta.kind,
        view: meta.view,
        isRoot: urn === resp.root,
      },
    };
    if (parent) {
      rfNode.parentId = parent;
      rfNode.extent = "parent";
      rfNode.expandParent = true;
    }
    if (isCont) {
      rfNode.style = { width: size.width, height: size.height };
    }
    return rfNode;
  });

  const edges: Edge[] = resp.edges.map((e) => ({
    id: e.id,
    source: e.from,
    target: e.to,
    label: e.type,
    type: "smoothstep",
    animated: false,
  }));

  return {
    nodes,
    edges,
    bbox: { width: root.width, height: root.height },
  };
}
