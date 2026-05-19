import * as fs from "fs";
import * as path from "path";
import { emit } from "../emit";
import { FrameworkPayload, EdgePayload } from "../proto";
import { Service } from "./services";

interface PackageJson {
  dependencies?: Record<string, string>;
  devDependencies?: Record<string, string>;
  peerDependencies?: Record<string, string>;
  optionalDependencies?: Record<string, string>;
}

/**
 * collectFrameworks lê o package.json do Service e emite 1 Framework
 * por dep declarada (dedup por URN). Também emite DependsOn
 * Service→Framework.
 *
 * NOTA: o Go-side npm.Collect (F-023) ainda existe — esta função
 * duplica o trabalho durante a transição. Slice futura: Go-side passa
 * a chamar typescript.Collect que já carrega Frameworks via sidecar, e
 * `npm.Collect` segue isolado para repos que não rodam o sidecar TS.
 */
export function collectFrameworks(repoRoot: string, repoURNBase: string, service: Service): void {
  const pkgPath = path.join(repoRoot, service.packageJsonPath);
  let pkg: PackageJson;
  try {
    pkg = JSON.parse(fs.readFileSync(pkgPath, "utf-8")) as PackageJson;
  } catch {
    return;
  }
  const serviceURN = `${repoURNBase}:service/${service.modulePath}`;

  const sections: { obj?: Record<string, string>; dev: boolean }[] = [
    { obj: pkg.dependencies, dev: false },
    { obj: pkg.devDependencies, dev: true },
    { obj: pkg.peerDependencies, dev: true },
    { obj: pkg.optionalDependencies, dev: true },
  ];

  const seen = new Set<string>();
  for (const s of sections) {
    if (!s.obj) continue;
    const names = Object.keys(s.obj).sort();
    for (const name of names) {
      if (seen.has(name)) continue;
      seen.add(name);
      const payload: FrameworkPayload = {
        ecosystem: "npm",
        name,
        latest_version: s.obj[name],
        is_dev_only: s.dev,
      };
      emit("framework", payload);

      // URN canônica do Framework: ver Go `node.NewFrameworkURN`
      // (account = "_global", id = "<ecosystem>!<name>"). Manter em
      // sync — DependsOn aponta para o mesmo nó emitido por
      // `appendFramework` no decoder Go.
      const fwURN = `urn:ce:code:_global:framework/npm!${name}`;
      const edge: EdgePayload = {
        type: "DependsOn",
        from_urn: serviceURN,
        to_urn: fwURN,
      };
      emit("edge", edge);
    }
  }
}
