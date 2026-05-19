import { Project } from "ts-morph";
import * as path from "path";
import { emit } from "../emit";
import { TypePayload, VariablePayload, EdgePayload } from "../proto";
import { Service } from "./services";
import { isWithinService } from "./functions";
import { moduleNamespaceForFile } from "./modules";

/**
 * collectTypesAndVariables emite:
 *   - node.Type por InterfaceDeclaration, TypeAliasDeclaration,
 *     ClassDeclaration, EnumDeclaration.
 *   - node.Variable por VariableStatement no escopo de módulo.
 *   - edges Extends (class extends, interface extends) e
 *     Aliases (`type X = Y` referenciando outro Type).
 */
export function collectTypesAndVariables(
  project: Project,
  repoRoot: string,
  repoURNBase: string,
  service: Service,
): { types: number; variables: number } {
  let typesCount = 0;
  let varsCount = 0;

  function typeURN(moduleNs: string, symbol: string): string {
    return `${repoURNBase}:type/${service.modulePath}!${moduleNs}!${symbol}`;
  }
  function varURN(moduleNs: string, symbol: string): string {
    return `${repoURNBase}:variable/${service.modulePath}!${moduleNs}!${symbol}`;
  }
  function modURN(moduleNs: string): string {
    return `${repoURNBase}:module/${service.modulePath}!${moduleNs}`;
  }

  for (const sf of project.getSourceFiles()) {
    if (!isWithinService(sf, service)) continue;
    const fileRel = path.relative(repoRoot, sf.getFilePath()).split(path.sep).join("/");
    const moduleNs = moduleNamespaceForFile(repoRoot, service, fileRel);

    // Interfaces
    for (const iface of sf.getInterfaces()) {
      const sym = iface.getName();
      const { line } = sf.getLineAndColumnAtPos(iface.getStart());
      const payload: TypePayload = {
        service_module_path: service.modulePath,
        module_namespace: moduleNs,
        namespace: moduleNs,
        symbol: sym,
        kind: "interface",
        exported: iface.isExported(),
        location: { file: fileRel, line },
      };
      emit("type", payload);
      typesCount++;
      emit("edge", { type: "Contains", from_urn: modURN(moduleNs), to_urn: typeURN(moduleNs, sym) } as EdgePayload);

      // Extends
      for (const ext of iface.getExtends()) {
        const targetName = ext.getExpression().getText();
        emit("edge", {
          type: "Extends",
          from_urn: typeURN(moduleNs, sym),
          to_urn: typeURN(moduleNs, targetName), // mesmo módulo (best-effort MVP)
        } as EdgePayload);
      }
    }

    // Type aliases
    for (const ta of sf.getTypeAliases()) {
      const sym = ta.getName();
      const { line } = sf.getLineAndColumnAtPos(ta.getStart());
      const payload: TypePayload = {
        service_module_path: service.modulePath,
        module_namespace: moduleNs,
        namespace: moduleNs,
        symbol: sym,
        kind: "type",
        exported: ta.isExported(),
        location: { file: fileRel, line },
      };
      emit("type", payload);
      typesCount++;
      emit("edge", { type: "Contains", from_urn: modURN(moduleNs), to_urn: typeURN(moduleNs, sym) } as EdgePayload);
    }

    // Classes
    for (const cls of sf.getClasses()) {
      const sym = cls.getName() || "(anonymous)";
      const { line } = sf.getLineAndColumnAtPos(cls.getStart());
      const payload: TypePayload = {
        service_module_path: service.modulePath,
        module_namespace: moduleNs,
        namespace: moduleNs,
        symbol: sym,
        kind: "class",
        exported: cls.isExported(),
        location: { file: fileRel, line },
      };
      emit("type", payload);
      typesCount++;
      emit("edge", { type: "Contains", from_urn: modURN(moduleNs), to_urn: typeURN(moduleNs, sym) } as EdgePayload);

      const ext = cls.getExtends();
      if (ext) {
        const targetName = ext.getExpression().getText();
        emit("edge", {
          type: "Extends",
          from_urn: typeURN(moduleNs, sym),
          to_urn: typeURN(moduleNs, targetName),
        } as EdgePayload);
      }
    }

    // Enums
    for (const en of sf.getEnums()) {
      const sym = en.getName();
      const { line } = sf.getLineAndColumnAtPos(en.getStart());
      const payload: TypePayload = {
        service_module_path: service.modulePath,
        module_namespace: moduleNs,
        namespace: moduleNs,
        symbol: sym,
        kind: "enum",
        exported: en.isExported(),
        location: { file: fileRel, line },
      };
      emit("type", payload);
      typesCount++;
      emit("edge", { type: "Contains", from_urn: modURN(moduleNs), to_urn: typeURN(moduleNs, sym) } as EdgePayload);
    }

    // Variables (module-level const/let, declarations NOT arrow funcs)
    for (const v of sf.getVariableDeclarations()) {
      const init = v.getInitializer();
      // Skip arrow funcs / function expressions — esses viraram Function.
      const k = init?.getKindName();
      if (k === "ArrowFunction" || k === "FunctionExpression") continue;
      const sym = v.getName();
      const exported = v.isExported() || isVarStmtExported(v);
      const typeText = v.getType().getText();
      const { line } = sf.getLineAndColumnAtPos(v.getStart());
      const payload: VariablePayload = {
        service_module_path: service.modulePath,
        module_namespace: moduleNs,
        symbol: sym,
        type_text: typeText,
        exported,
        location: { file: fileRel, line },
      };
      emit("variable", payload);
      varsCount++;
      emit("edge", { type: "Contains", from_urn: modURN(moduleNs), to_urn: varURN(moduleNs, sym) } as EdgePayload);
    }
  }

  return { types: typesCount, variables: varsCount };
}

function isVarStmtExported(v: import("ts-morph").VariableDeclaration): boolean {
  const stmt = v.getVariableStatement();
  if (!stmt) return false;
  return stmt.hasExportKeyword();
}
