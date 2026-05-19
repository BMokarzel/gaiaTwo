import * as fs from "fs";
import * as path from "path";
import { emit } from "../emit";
import { findPackageJsons } from "../walk";
import { ServicePayload } from "../proto";

export interface Service {
  modulePath: string; // "." se raiz, ex.: "packages/api"
  absPath: string;
  packageJsonPath: string; // rel ao repo
  name: string; // package.json name
  slug: string;
}

interface PackageJson {
  name?: string;
  version?: string;
  dependencies?: Record<string, string>;
  devDependencies?: Record<string, string>;
  peerDependencies?: Record<string, string>;
  optionalDependencies?: Record<string, string>;
  workspaces?: string[] | { packages?: string[] };
}

/**
 * collectServices emite 1 Service por `package.json` encontrado.
 * Devolve a lista para uso pelas próximas etapas (Module/Function/…).
 *
 * Convenção:
 *   - 1 package.json = 1 Service (simétrico com 1 go.mod = 1 Service).
 *   - `module_path` = dir relativo ao repo root contendo o package.json
 *     ("." se raiz).
 *   - `slug` = name sanitizado (sem @scope/, com `-` em vez de `_`).
 */
export function collectServices(repoRoot: string, extraSkip: string[]): Service[] {
  const pkgPaths = findPackageJsons(repoRoot, extraSkip);
  const services: Service[] = [];

  for (const rel of pkgPaths) {
    const abs = path.join(repoRoot, rel);
    const pkg = readPackageJson(abs);
    if (!pkg.name) continue; // skips workspace root sem `name`? não — emit mesmo assim, mas slug fica vazio
    const dir = path.dirname(rel);
    const modulePath = dir === "" || dir === "." ? "." : toUnix(dir);
    const slug = slugify(pkg.name || path.basename(dir || "root"));

    const svc: Service = {
      modulePath,
      absPath: path.dirname(abs),
      packageJsonPath: toUnix(rel),
      name: pkg.name || slug,
      slug,
    };
    services.push(svc);

    const payload: ServicePayload = {
      slug,
      module_path: modulePath,
      namespace: pkg.name || slug,
      manifest: toUnix(rel),
      manifest_type: "package.json",
      language: "typescript",
    };
    emit("service", payload);
  }

  return services;
}

export function readPackageJson(absPath: string): PackageJson {
  try {
    const raw = fs.readFileSync(absPath, "utf-8");
    return JSON.parse(raw) as PackageJson;
  } catch {
    return {};
  }
}

function slugify(name: string): string {
  return name
    .replace(/^@[^/]+\//, "") // strip @scope/
    .replace(/[^a-zA-Z0-9-]/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "")
    .toLowerCase();
}

function toUnix(p: string): string {
  return p.split(path.sep).join("/");
}
