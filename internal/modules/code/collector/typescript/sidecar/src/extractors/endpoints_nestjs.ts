import { Project, ClassDeclaration, MethodDeclaration, Decorator, SyntaxKind } from "ts-morph";
import * as path from "path";
import { emit } from "../emit";
import { EndpointPayload } from "../proto";
import { Service } from "./services";
import { isWithinService } from "./functions";
import { moduleNamespaceForFile } from "./modules";

const NEST_METHOD_DECORATORS = new Set(["Get", "Post", "Put", "Delete", "Patch", "Options", "Head", "All"]);

/**
 * collectNestJsEndpoints procura:
 *   - Classes decoradas com `@Controller('prefix')` (ou `@Controller()` sem prefix).
 *   - Métodos decorados com `@Get|Post|Put|...('path')` (ou sem path).
 *
 * Emite 1 Endpoint por método, com path final = prefix + path.
 *
 * Limitações MVP:
 *   - Não trata `@Controller({ path: '/x', version: '1' })` (objeto literal).
 *   - Não trata heranças (controller estende outro controller).
 */
export function collectNestJsEndpoints(
  project: Project,
  repoRoot: string,
  service: Service,
): number {
  let count = 0;

  for (const sf of project.getSourceFiles()) {
    if (!isWithinService(sf, service)) continue;
    if (!hasNestImport(sf)) continue;

    const fileRel = path.relative(repoRoot, sf.getFilePath()).split(path.sep).join("/");
    const moduleNs = moduleNamespaceForFile(repoRoot, service, fileRel);

    for (const cls of sf.getClasses()) {
      const ctrlPrefix = extractControllerPrefix(cls);
      if (ctrlPrefix === null) continue;

      for (const m of cls.getMethods()) {
        const route = extractMethodRoute(m);
        if (!route) continue;
        const fullPath = joinPaths(ctrlPrefix, route.path);
        const { line } = sf.getLineAndColumnAtPos(m.getStart());
        const payload: EndpointPayload = {
          service_module_path: service.modulePath,
          module_namespace: moduleNs,
          method: route.method.toUpperCase(),
          path: fullPath,
          framework: "nestjs",
          handler_symbol: `(${cls.getName() || ""}).${m.getName()}`,
          location: { file: fileRel, line },
        };
        emit("endpoint", payload);
        count++;
      }
    }
  }
  return count;
}

function hasNestImport(sf: import("ts-morph").SourceFile): boolean {
  for (const imp of sf.getImportDeclarations()) {
    const mod = imp.getModuleSpecifierValue();
    if (mod === "@nestjs/common" || mod.startsWith("@nestjs/")) return true;
  }
  return false;
}

/** Retorna prefix ou string vazia se @Controller() sem path. null se class não é controller. */
function extractControllerPrefix(cls: ClassDeclaration): string | null {
  const dec = cls.getDecorators().find((d) => d.getName() === "Controller");
  if (!dec) return null;
  const args = dec.getArguments();
  if (args.length === 0) return "";
  const first = args[0];
  if (first.getKind() === SyntaxKind.StringLiteral) {
    return stripQuotes(first.getText());
  }
  // @Controller({ path: '...' })
  if (first.getKind() === SyntaxKind.ObjectLiteralExpression) {
    const ole = first.asKindOrThrow(SyntaxKind.ObjectLiteralExpression);
    const propPath = ole.getProperty("path");
    if (propPath && propPath.getKind() === SyntaxKind.PropertyAssignment) {
      const pa = propPath.asKindOrThrow(SyntaxKind.PropertyAssignment);
      const init = pa.getInitializer();
      if (init && init.getKind() === SyntaxKind.StringLiteral) {
        return stripQuotes(init.getText());
      }
    }
    return "";
  }
  return "";
}

function extractMethodRoute(m: MethodDeclaration): { method: string; path: string } | null {
  for (const d of m.getDecorators()) {
    const name = d.getName();
    if (!NEST_METHOD_DECORATORS.has(name)) continue;
    const args = d.getArguments();
    let pathArg = "";
    if (args.length > 0 && args[0].getKind() === SyntaxKind.StringLiteral) {
      pathArg = stripQuotes(args[0].getText());
    }
    return { method: name, path: pathArg };
  }
  return null;
}

function joinPaths(prefix: string, route: string): string {
  let P = prefix.replace(/^\/+|\/+$/g, "");
  let R = route.replace(/^\/+|\/+$/g, "");
  let result = "";
  if (P) result += "/" + P;
  if (R) result += "/" + R;
  return result || "/";
}

function stripQuotes(s: string): string {
  return s.replace(/^['"`]/, "").replace(/['"`]$/, "");
}
