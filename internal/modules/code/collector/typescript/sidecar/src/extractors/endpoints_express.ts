import { Project, SourceFile, CallExpression, SyntaxKind, Node, VariableDeclaration } from "ts-morph";
import * as path from "path";
import { emit } from "../emit";
import { EndpointPayload } from "../proto";
import { Service } from "./services";
import { isWithinService } from "./functions";
import { moduleNamespaceForFile } from "./modules";

const HTTP_METHODS = new Set(["get", "post", "put", "delete", "patch", "options", "head", "all", "use"]);

interface RouterInfo {
  basePath: string; // path prefix
  declName: string; // variable name in source ("app", "router", "r")
}

/**
 * collectExpressEndpoints procura:
 *   - `import express from 'express'` (ou similar)
 *   - `const app = express()` → app é router base com basePath=""
 *   - `const r = express.Router()` ou `Router()` → sub-router
 *   - `app.use('/prefix', r)` → propaga basePath para r
 *   - `app.get|post|put|...(path, handler)` → emite Endpoint
 *
 * Limitações MVP:
 *   - Não resolve handlers em outros arquivos (path mantém handler-symbol literal).
 *   - Não trata routers em arrays (`app.use([r1, r2])`).
 *   - Não trata `app.route('/x').get(...)`.
 */
export function collectExpressEndpoints(
  project: Project,
  repoRoot: string,
  service: Service,
): number {
  let count = 0;
  // Por arquivo: declName → basePath
  for (const sf of project.getSourceFiles()) {
    if (!isWithinService(sf, service)) continue;
    if (!hasExpressImport(sf)) continue;

    const fileRel = path.relative(repoRoot, sf.getFilePath()).split(path.sep).join("/");
    const moduleNs = moduleNamespaceForFile(repoRoot, service, fileRel);

    const routers = discoverRouters(sf);
    if (routers.size === 0) continue;

    sf.forEachDescendant((node) => {
      if (node.getKind() !== SyntaxKind.CallExpression) return;
      const call = node as CallExpression;
      const ep = matchRouteCall(call, routers);
      if (!ep) return;
      const { line } = sf.getLineAndColumnAtPos(call.getStart());
      const payload: EndpointPayload = {
        service_module_path: service.modulePath,
        module_namespace: moduleNs,
        method: ep.method,
        path: ep.path,
        framework: "express",
        handler_symbol: ep.handlerSymbol,
        location: { file: fileRel, line },
      };
      emit("endpoint", payload);
      count++;
    });
  }
  return count;
}

function hasExpressImport(sf: SourceFile): boolean {
  for (const imp of sf.getImportDeclarations()) {
    const mod = imp.getModuleSpecifierValue();
    if (mod === "express" || mod === "@hapi/express" || mod === "express-router") {
      return true;
    }
  }
  return false;
}

function discoverRouters(sf: SourceFile): Map<string, RouterInfo> {
  const out = new Map<string, RouterInfo>();
  // Step 1: encontrar declarações de router/app
  for (const v of sf.getVariableDeclarations()) {
    const init = v.getInitializer();
    if (!init) continue;
    const initText = init.getText();
    // `express()`
    if (/express\s*\(\s*\)/.test(initText)) {
      out.set(v.getName(), { basePath: "", declName: v.getName() });
    }
    // `Router()` ou `express.Router()`
    else if (/(^|\.)Router\s*\(\s*\)/.test(initText)) {
      out.set(v.getName(), { basePath: "", declName: v.getName() });
    }
  }

  // Step 2: propagar app.use('/prefix', subRouter) — segunda passada
  sf.forEachDescendant((node) => {
    if (node.getKind() !== SyntaxKind.CallExpression) return;
    const call = node as CallExpression;
    const expr = call.getExpression();
    if (expr.getKind() !== SyntaxKind.PropertyAccessExpression) return;
    const pae = expr.asKindOrThrow(SyntaxKind.PropertyAccessExpression);
    const methodName = pae.getName();
    if (methodName !== "use") return;
    const obj = pae.getExpression().getText();
    if (!out.has(obj)) return;
    const args = call.getArguments();
    if (args.length < 2) return;
    const first = args[0];
    if (first.getKind() !== SyntaxKind.StringLiteral) return;
    const prefix = stripQuotes(first.getText());
    // args[1] precisa ser referência a um router conhecido
    const subRef = args[1].getText();
    const sub = out.get(subRef);
    if (sub) {
      // Compose paths
      out.set(subRef, { ...sub, basePath: joinPaths(out.get(obj)!.basePath, prefix) });
    }
  });

  return out;
}

interface RouteMatch {
  method: string;
  path: string;
  handlerSymbol?: string;
}

function matchRouteCall(call: CallExpression, routers: Map<string, RouterInfo>): RouteMatch | null {
  const expr = call.getExpression();
  if (expr.getKind() !== SyntaxKind.PropertyAccessExpression) return null;
  const pae = expr.asKindOrThrow(SyntaxKind.PropertyAccessExpression);
  const methodName = pae.getName().toLowerCase();
  if (!HTTP_METHODS.has(methodName)) return null;
  if (methodName === "use") return null; // mounting, não rota
  const obj = pae.getExpression().getText();
  const router = routers.get(obj);
  if (!router) return null;
  const args = call.getArguments();
  if (args.length < 2) return null;
  const first = args[0];
  if (first.getKind() !== SyntaxKind.StringLiteral) return null;
  const routePath = joinPaths(router.basePath, stripQuotes(first.getText()));
  const handlerArg = args[args.length - 1];
  let handlerSymbol: string | undefined;
  if (handlerArg.getKind() === SyntaxKind.Identifier) {
    handlerSymbol = handlerArg.getText();
  } else if (handlerArg.getKind() === SyntaxKind.PropertyAccessExpression) {
    handlerSymbol = handlerArg.getText();
  }
  return { method: methodName.toUpperCase(), path: routePath, handlerSymbol };
}

function joinPaths(a: string, b: string): string {
  const A = a.replace(/\/+$/, "");
  const B = b.startsWith("/") ? b : "/" + b;
  const j = A + B;
  return j === "" ? "/" : j;
}

function stripQuotes(s: string): string {
  return s.replace(/^['"`]/, "").replace(/['"`]$/, "");
}
