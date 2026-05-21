// Container node — usado para Service (na arquitetura) e Module
// (em ambos os contextos). Renderiza uma caixa rotulada que envolve
// outros nós via `parentId` do React Flow. Quando o usuário arrasta
// um filho, `expandParent: true` aumenta este container automaticamente.
//
// O size do container é definido pelo layout (style.width/height) e
// re-calculado em tempo de mount; movimentações posteriores expandem
// (não contraem) via RF.

import { Handle, Position, type NodeProps } from "@xyflow/react";
import type { FlowNodeData } from "../layout";
import { primaryLabel, kindColorVar } from "./labels";
import styles from "./shapes.module.css";

export function ContainerNode({ data }: NodeProps) {
  const fd = data as FlowNodeData;
  const { view, kind, isRoot } = fd;
  const color = kindColorVar[kind];
  return (
    <div className={`${styles.container} ${isRoot ? styles.root : ""}`}>
      <Handle type="target" position={Position.Left} />
      <div className={styles.containerHeader}>
        <span className={styles.kind} style={color ? { color } : undefined}>
          {kind}
        </span>
        <span className={styles.containerTitle}>
          {primaryLabel(kind, view.data)}
        </span>
      </div>
      <Handle type="source" position={Position.Right} />
    </div>
  );
}
