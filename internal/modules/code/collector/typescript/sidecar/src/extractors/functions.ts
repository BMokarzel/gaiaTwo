import { Project, SourceFile, FunctionDeclaration, MethodDeclaration, ArrowFunction, FunctionExpression, ClassDeclaration, SyntaxKind, Node } from "ts-morph";
import * as crypto from "crypto";
import * as path from "path";
import { emit } from "../emit";
import { FunctionPayload, EdgePayload } from "../proto";
import { Service } from "./services";
import { moduleNamespaceForFile } from "./modules";

/**
 * collectFunctions percorre os SourceFiles do Project e emite uma
 * Function por declaração relevante:
 *   - FunctionDeclaration (top-level)
 *   - ClassDeclaration.methods
 *   - Exported ArrowFunction / FunctionExpression em const declarations
 *
 * Closures aninhadas, IIFEs e callbacks anônimos NÃO viram Function
 * (mesmo critério que o coletor Go).
 *
 * Edges Contains Module→Function são emitidas conforme o
 * module_namespace resolvido por longest-prefix-match.
 */
export function collectFunctions(
  project: Project,
  repoRoot: string,
  repoURNBase: string,
  service: Service,
  serviceURN: string,
): { count: number; emittedURNs: Map<string, string> } {
  let count = 0;
  // key: `${file}:${name}` → function_urn. Útil para resolver call->Function.
  const emittedURNs = new Map<string, string>();

  for (const sf of project.getSourceFiles()) {
    if (!isWithinService(sf, service)) continue;
    const fileRel = path.relative(repoRoot, sf.getFilePath()).split(path.sep).join("/");
    const moduleNs = moduleNamespaceForFile(repoRoot, service, fileRel);
    const namespace = moduleNs; // alias: TS function namespace = module namespace

    // Top-level FunctionDeclaration.
    for (const fn of sf.getFunctions()) {
      emitFunction(fn, namespace, "");
    }

    // Class methods.
    for (const cls of sf.getClasses()) {
      const clsName = cls.getName() || "(anonymous)";
      for (const m of cls.getMethods()) {
        emitFunction(m, namespace, clsName);
      }
    }

    // Const arrows: `export const findById = (...) => {...}`.
    for (const v of sf.getVariableDeclarations()) {
      const init = v.getInitializer();
      if (!init) continue;
      if (init.getKind() !== SyntaxKind.ArrowFunction && init.getKind() !== SyntaxKind.FunctionExpression) continue;
      const isExported = v.isExported() || v.hasExportKeyword?.() || isParentExported(v);
      const symbol = v.getName();
      emitArrow(init as ArrowFunction | FunctionExpression, namespace, symbol, isExported);
    }
  }

  return { count, emittedURNs };

  function emitFunction(
    fn: FunctionDeclaration | MethodDeclaration,
    moduleNs: string,
    receiver: string,
  ): void {
    const name = fn.getName?.() ?? "";
    if (!name) return;
    const symbol = receiver ? `(${receiver}).${name}` : name;
    const exported = isDeclExported(fn);
    const sig = canonicalSignature(fn, symbol);
    const sigHash = sha256(sig);
    const start = fn.getStart();
    const file = path.relative(repoRoot, fn.getSourceFile().getFilePath()).split(path.sep).join("/");
    const { line } = fn.getSourceFile().getLineAndColumnAtPos(start);

    const payload: FunctionPayload = {
      service_module_path: service.modulePath,
      module_namespace: moduleNs,
      namespace: moduleNs,
      symbol,
      receiver: receiver || undefined,
      exported,
      signature_hash: sigHash,
      signature: sig,
      location: { file, line },
    };
    emit("function", payload);
    count++;

    const fnURN = `${repoURNBase}:function/${service.modulePath}!${moduleNs}!${symbol}`;
    emittedURNs.set(`${file}:${symbol}`, fnURN);

    // Contains Module→Function edge
    const modURN = `${repoURNBase}:module/${service.modulePath}!${moduleNs}`;
    const edgePayload: EdgePayload = { type: "Contains", from_urn: modURN, to_urn: fnURN };
    emit("edge", edgePayload);
  }

  function emitArrow(
    fn: ArrowFunction | FunctionExpression,
    moduleNs: string,
    symbol: string,
    exported: boolean,
  ): void {
    const sig = canonicalArrowSignature(fn, symbol);
    const sigHash = sha256(sig);
    const start = fn.getStart();
    const file = path.relative(repoRoot, fn.getSourceFile().getFilePath()).split(path.sep).join("/");
    const { line } = fn.getSourceFile().getLineAndColumnAtPos(start);

    const payload: FunctionPayload = {
      service_module_path: service.modulePath,
      module_namespace: moduleNs,
      namespace: moduleNs,
      symbol,
      exported,
      signature_hash: sigHash,
      signature: sig,
      location: { file, line },
    };
    emit("function", payload);
    count++;

    const fnURN = `${repoURNBase}:function/${service.modulePath}!${moduleNs}!${symbol}`;
    emittedURNs.set(`${file}:${symbol}`, fnURN);

    const modURN = `${repoURNBase}:module/${service.modulePath}!${moduleNs}`;
    const edgePayload: EdgePayload = { type: "Contains", from_urn: modURN, to_urn: fnURN };
    emit("edge", edgePayload);
  }
}

export function isWithinService(sf: SourceFile, service: Service): boolean {
  const p = sf.getFilePath();
  const norm = p.split(path.sep).join("/");
  const root = service.absPath.split(path.sep).join("/");
  return norm.startsWith(root + "/") || norm === root;
}

function isDeclExported(fn: FunctionDeclaration | MethodDeclaration): boolean {
  if ("isExported" in fn && typeof fn.isExported === "function") {
    return fn.isExported();
  }
  return false;
}

function isParentExported(node: Node): boolean {
  const stmt = node.getFirstAncestorByKind(SyntaxKind.VariableStatement);
  if (!stmt) return false;
  return stmt.hasExportKeyword();
}

function canonicalSignature(fn: FunctionDeclaration | MethodDeclaration, symbol: string): string {
  const params = fn.getParameters().map((p) => `${p.getName()}:${p.getType().getText()}`).join(",");
  const ret = fn.getReturnType().getText();
  return `${symbol}(${params}):${ret}`;
}

function canonicalArrowSignature(fn: ArrowFunction | FunctionExpression, symbol: string): string {
  const params = fn.getParameters().map((p) => `${p.getName()}:${p.getType().getText()}`).join(",");
  const ret = fn.getReturnType().getText();
  return `${symbol}(${params}):${ret}`;
}

function sha256(s: string): string {
  return crypto.createHash("sha256").update(s).digest("hex");
}
