// Custom react-flow node — renderiza um NodeView do costEngine com
// rótulo primário escolhido conforme o Kind (route p/ endpoint, symbol
// p/ function, etc.).

import { Handle, Position, type NodeProps } from "@xyflow/react";
import type {
  CallData, EndpointData, FrameworkData, FunctionData, ModuleData,
  ServiceData, TypeData, VariableData,
} from "@/api/types";
import type { FlowNodeData } from "./layout";
import styles from "./CeNode.module.css";

function primaryLabel(kind: string, data: Record<string, unknown>): string {
  switch (kind) {
    case "service": {
      const d = data as ServiceData;
      return d.repo ? `${d.repo} :: ${d.module_path ?? "."}` : "(service)";
    }
    case "module": {
      const d = data as ModuleData;
      return d.path ?? d.namespace ?? "(module)";
    }
    case "endpoint": {
      const d = data as EndpointData;
      return `${d.method ?? "?"} ${d.route ?? ""}`.trim();
    }
    case "function": {
      const d = data as FunctionData;
      return d.symbol ?? "(function)";
    }
    case "call": {
      const d = data as CallData;
      return d.target_symbol ?? d.kind ?? "(call)";
    }
    case "type": {
      const d = data as TypeData;
      return d.symbol ?? "(type)";
    }
    case "variable": {
      const d = data as VariableData;
      return d.symbol ?? "(variable)";
    }
    case "framework": {
      const d = data as FrameworkData;
      return d.name ? `${d.name}${d.version ? `@${d.version}` : ""}` : "(fw)";
    }
    default:
      return kind;
  }
}

function secondary(kind: string, data: Record<string, unknown>): string | undefined {
  switch (kind) {
    case "function": {
      const d = data as FunctionData;
      return d.namespace;
    }
    case "endpoint": {
      const d = data as EndpointData;
      return d.handler;
    }
    case "module": {
      const d = data as ModuleData;
      return d.namespace;
    }
    case "call": {
      const d = data as CallData;
      return d.target_urn;
    }
    default:
      return undefined;
  }
}

const kindClass: Record<string, string> = {
  service: styles.kindService,
  module: styles.kindModule,
  endpoint: styles.kindEndpoint,
  function: styles.kindFunction,
  call: styles.kindCall,
  type: styles.kindType,
  variable: styles.kindVariable,
  framework: styles.kindFramework,
};

export function CeNode({ data }: NodeProps) {
  const fd = data as FlowNodeData;
  const { view, kind, isRoot } = fd;
  const sub = secondary(kind, view.data);
  return (
    <div className={`${styles.node} ${isRoot ? styles.root : ""}`}>
      <Handle type="target" position={Position.Left} />
      <div className={styles.header}>
        <span className={`${styles.kind} ${kindClass[kind] ?? ""}`}>
          {kind}
        </span>
      </div>
      <div className={styles.primary}>{primaryLabel(kind, view.data)}</div>
      {sub && <div className={styles.secondary}>{sub}</div>}
      <Handle type="source" position={Position.Right} />
    </div>
  );
}
