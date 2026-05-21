// Rótulos compartilhados entre os shape nodes (Round/Diamond/Rect).
// Cada Kind tem regras próprias de "qual campo de NodeView.data vira o
// label primário e o secundário". Mantemos isso aqui em vez de
// duplicar em cada componente.

import type {
  CallData, EndpointData, FrameworkData, FunctionData, ModuleData,
  ServiceData, TypeData, VariableData,
} from "@/api/types";

export function primaryLabel(kind: string, data: Record<string, unknown>): string {
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

export function secondaryLabel(kind: string, data: Record<string, unknown>): string | undefined {
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

// Map de Kind → classe CSS de cor (aplicada ao header "kind" e ao
// realce do shape). Mantemos um único map p/ todos os shapes.
export const kindColorVar: Record<string, string> = {
  service: "var(--accent-green)",
  endpoint: "var(--accent-green)",
  function: "var(--accent-teal)",
  call: "var(--accent-teal)",
  type: "var(--accent-blue)",
  variable: "var(--accent-blue)",
  framework: "var(--accent-purple)",
  module: "var(--accent-orange)",
};
