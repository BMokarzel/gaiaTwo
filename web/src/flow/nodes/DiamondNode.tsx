// Shape losango — reservado para nós de controle de fluxo (if, switch,
// loop). O coletor ainda não emite esses kinds; o componente fica como
// placeholder para quando F-XYZ adicionar `flow_control` ao schema.
//
// Implementação: quadrado rotacionado 45° com conteúdo des-rotacionado
// por cima. Handles ficam nas pontas (top/bottom) para casar com os
// vértices "leste/oeste" do losango após a rotação.

import { Handle, Position, type NodeProps } from "@xyflow/react";
import type { FlowNodeData } from "../layout";
import { primaryLabel, kindColorVar } from "./labels";
import styles from "./shapes.module.css";

export function DiamondNode({ data }: NodeProps) {
  const fd = data as FlowNodeData;
  const { view, kind, isRoot } = fd;
  const color = kindColorVar[kind];
  return (
    <div className={styles.diamondWrap}>
      <Handle type="target" position={Position.Left} />
      <div className={`${styles.diamond} ${isRoot ? styles.root : ""}`} />
      <div className={styles.diamondContent}>
        <div className={styles.header}>
          <span className={styles.kind} style={color ? { color } : undefined}>
            {kind}
          </span>
        </div>
        <div className={styles.primary}>{primaryLabel(kind, view.data)}</div>
      </div>
      <Handle type="source" position={Position.Right} />
    </div>
  );
}
