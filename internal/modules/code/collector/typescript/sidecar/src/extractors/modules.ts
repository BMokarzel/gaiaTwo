import * as fs from "fs";
import * as path from "path";
import { emit } from "../emit";
import { ModulePayload, EdgePayload } from "../proto";
import { Service } from "./services";

const DEFAULT_SKIP_DIRS = new Set([
  "node_modules", "dist", "build", "out", ".next", ".turbo", ".git", "coverage",
]);

/**
 * collectModules percorre o diretório de cada Service por subdirs que
 * contenham arquivos .ts e emite 1 Module por diretório.
 *
 * `namespace` = caminho relativo ao service root (slash-separated).
 *  Ex.: "src/users", "src/users/dto", "".
 *
 * Também emite edges:
 *  - Service → Module raiz (Contains)
 *  - Module pai → Module filho (Contains)
 */
export function collectModules(repoRoot: string, service: Service, serviceURN: string): string[] {
  const moduleNamespaces: string[] = [];
  const seen = new Set<string>();

  function urnOfModule(ns: string): string {
    // Espelha node.NewModuleURN: urn:ce:code:<repo>:module/<service-module-path>!<namespace>
    // O Go-side é quem materializa — sidecar precisa só do namespace.
    // Para edges, sidecar passa as URNs já montadas, então precisamos
    // reproduzir o formato. `repo` chega via globalContext.
    return `${serviceURN.split(":service/")[0]}:module/${service.modulePath}!${ns}`;
  }

  function walk(absDir: string, nsParts: string[]): boolean {
    let entries: fs.Dirent[];
    try {
      entries = fs.readdirSync(absDir, { withFileTypes: true });
    } catch {
      return false;
    }

    let hasTs = false;
    const childDirs: { name: string; absPath: string }[] = [];

    for (const e of entries) {
      if (e.isFile()) {
        const ext = path.extname(e.name);
        if (ext === ".ts" && !e.name.endsWith(".d.ts")) hasTs = true;
        else if (ext === ".tsx") hasTs = true;
      } else if (e.isDirectory()) {
        if (DEFAULT_SKIP_DIRS.has(e.name) || (e.name.startsWith(".") && absDir !== service.absPath)) continue;
        // Outro package.json = outro Service.
        if (fs.existsSync(path.join(absDir, e.name, "package.json"))) continue;
        childDirs.push({ name: e.name, absPath: path.join(absDir, e.name) });
      }
    }

    let descendantHasTs = false;
    for (const c of childDirs) {
      const childHas = walk(c.absPath, [...nsParts, c.name]);
      descendantHasTs = descendantHasTs || childHas;
    }

    if (hasTs || descendantHasTs) {
      const ns = nsParts.join("/");
      if (!seen.has(ns)) {
        seen.add(ns);
        moduleNamespaces.push(ns);
        const relToRepoRaw = path.relative(repoRoot, absDir).split(path.sep).join("/");
        const relToRepo = relToRepoRaw === "" ? "." : relToRepoRaw;
        const modPayload: ModulePayload = {
          service_module_path: service.modulePath,
          namespace: ns,
          path: relToRepo,
        };
        emit("module", modPayload);

        // Contains edge: pai → este
        if (nsParts.length === 0) {
          const edge: EdgePayload = {
            type: "Contains",
            from_urn: serviceURN,
            to_urn: urnOfModule(ns),
          };
          emit("edge", edge);
        } else {
          const parentNs = nsParts.slice(0, -1).join("/");
          const edge: EdgePayload = {
            type: "Contains",
            from_urn: urnOfModule(parentNs),
            to_urn: urnOfModule(ns),
          };
          emit("edge", edge);
        }
      }
      return true;
    }
    return false;
  }

  walk(service.absPath, []);
  return moduleNamespaces;
}

/**
 * moduleNamespaceForFile resolve o `namespace` do Module que contém
 * `fileRelToRepo`. Longest-prefix-match entre o dir do arquivo e o
 * service root.
 */
export function moduleNamespaceForFile(
  repoRoot: string,
  service: Service,
  fileRelToRepo: string,
): string {
  const fileAbs = path.join(repoRoot, fileRelToRepo);
  const dirAbs = path.dirname(fileAbs);
  const rel = path.relative(service.absPath, dirAbs);
  if (rel === "" || rel === ".") return "";
  return rel.split(path.sep).join("/");
}
