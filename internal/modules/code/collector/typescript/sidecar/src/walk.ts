import * as fs from "fs";
import * as path from "path";

const DEFAULT_SKIP_DIRS = new Set([
  "node_modules",
  "dist",
  "build",
  "out",
  ".next",
  ".turbo",
  ".git",
  "coverage",
]);

/**
 * findPackageJsons percorre `root` recursivamente e devolve todos os
 * `package.json` (rel paths). Dirs em `DEFAULT_SKIP_DIRS` ou que
 * começam com "." (exceto root) são pulados.
 */
export function findPackageJsons(root: string, extraSkip: string[] = []): string[] {
  const skip = new Set([...DEFAULT_SKIP_DIRS, ...extraSkip]);
  const out: string[] = [];

  function walk(dir: string): void {
    let entries: fs.Dirent[];
    try {
      entries = fs.readdirSync(dir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const e of entries) {
      if (e.isDirectory()) {
        if (skip.has(e.name) || (e.name.startsWith(".") && dir !== root)) continue;
        walk(path.join(dir, e.name));
      } else if (e.isFile() && e.name === "package.json") {
        out.push(path.relative(root, path.join(dir, e.name)));
      }
    }
  }

  walk(root);
  return out.sort();
}

/**
 * findTsFiles percorre `serviceRoot` por arquivos `.ts`/`.tsx`. Skips
 * default + extras. Devolve rel paths ao `repoRoot`.
 */
export function findTsFiles(
  serviceRoot: string,
  repoRoot: string,
  extraSkip: string[] = [],
): string[] {
  const skip = new Set([...DEFAULT_SKIP_DIRS, ...extraSkip]);
  const out: string[] = [];

  function walk(dir: string): void {
    let entries: fs.Dirent[];
    try {
      entries = fs.readdirSync(dir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const e of entries) {
      if (e.isDirectory()) {
        if (skip.has(e.name) || (e.name.startsWith(".") && dir !== serviceRoot)) continue;
        // Sub-package.json indica outro Service; pula.
        const subPkg = path.join(dir, e.name, "package.json");
        if (fs.existsSync(subPkg)) continue;
        walk(path.join(dir, e.name));
      } else if (e.isFile()) {
        const ext = path.extname(e.name);
        if (ext === ".ts" || ext === ".tsx") {
          if (e.name.endsWith(".d.ts")) continue;
          out.push(path.relative(repoRoot, path.join(dir, e.name)));
        }
      }
    }
  }

  walk(serviceRoot);
  return out.sort();
}
