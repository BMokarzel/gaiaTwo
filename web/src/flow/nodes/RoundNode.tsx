// Shape redondo — usado para `service` e `endpoint`, os "nós-entrada"
// do modelo. Forma circular sinaliza que são pontos de entrada/saída
// do sistema (diferente de funções/calls que são "passos" internos).

import { Handle, Position, type NodeProps } from "@xyflow/react";
import type { FlowNodeData } from "../layout";
import { primaryLabel, secondaryLabel, kindColorVar } from "./labels";
import styles from "./shapes.module.css";

export function RoundNode({ data }: NodeProps) {
  const fd = data as FlowNodeData;
  const { view, kind, isRoot } = fd;
  const sub = secondaryLabel(kind, view.data);
  const color = kindColorVar[kind];
  return (
    <div className={`${styles.round} ${isRoot ? styles.root : ""}`}>
      <Handle type="target" position={Position.Left} />
      <div className={styles.header}>
        <span className={styles.kind} style={color ? { color } : undefined}>
          {kind}
        </span>
      </div>
      <div className={styles.primary}>{primaryLabel(kind, view.data)}</div>
      {sub && <div className={styles.secondary}>{sub}</div>}
      <Handle type="source" position={Position.Right} />
    </div>
  );
}
