// Map único `nodeTypes` consumido pelo `<ReactFlow>`. A escolha do
// shape por Kind também é centralizada aqui (resolveNodeType) para
// que layout.ts e os pages compartilhem a mesma regra.

import { RectNode } from "./RectNode";
import { RoundNode } from "./RoundNode";
import { DiamondNode } from "./DiamondNode";
import { ContainerNode } from "./ContainerNode";

export const nodeTypes = {
  rect: RectNode,
  round: RoundNode,
  diamond: DiamondNode,
  container: ContainerNode,
};

// Mapa Kind → tipo react-flow.
// - service/endpoint → round (pontos de entrada do sistema)
// - flow_control     → diamond (placeholder; ainda não emitido)
// - resto            → rect
export function resolveNodeType(kind: string): "round" | "diamond" | "rect" {
  switch (kind) {
    case "service":
    case "endpoint":
      return "round";
    case "flow_control":
      return "diamond";
    default:
      return "rect";
  }
}

// Dimensões nominais por shape — usadas pelo dagre para reservar
// espaço sem sobreposição. Casam com `shapes.module.css`.
export const NODE_DIMS: Record<"round" | "diamond" | "rect", { width: number; height: number }> = {
  round: { width: 180, height: 180 },
  diamond: { width: 160, height: 160 },
  rect: { width: 240, height: 70 },
};
