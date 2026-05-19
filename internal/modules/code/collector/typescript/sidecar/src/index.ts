/**
 * Entry-point do sidecar TypeScript do CostEngine (F-030).
 *
 * Fluxo:
 *   1. Lê 1 linha JSON (ConfigEnvelope) do stdin.
 *   2. Emite `init` event.
 *   3. Para cada Service descoberto (1 package.json = 1 Service):
 *      a. emit Service
 *      b. collectModules (S-030.2)
 *      c. collectTypesAndVariables (S-030.3)
 *      d. collectFunctions (S-030.3)
 *      e. collectExpressEndpoints (S-030.4)
 *      f. collectNestJsEndpoints (S-030.5)
 *      g. collectCalls (S-030.6)
 *      h. collectFrameworks (S-030.7)
 *   4. Emite `done` event com totais.
 *
 * Erros não recuperáveis → emite `error` e exit(1).
 */
import { Project } from "ts-morph";
import * as fs from "fs";
import * as path from "path";
import { emit, emitLog, fatal, newStats } from "./emit";
import { ConfigEnvelope, SCHEMA, InitPayload, DonePayload } from "./proto";
import { collectServices } from "./extractors/services";
import { collectModules } from "./extractors/modules";
import { collectTypesAndVariables } from "./extractors/types";
import { collectFunctions } from "./extractors/functions";
import { collectExpressEndpoints } from "./extractors/endpoints_express";
import { collectNestJsEndpoints } from "./extractors/endpoints_nestjs";
import { collectCalls } from "./extractors/calls";
import { collectFrameworks } from "./extractors/frameworks";

const SIDECAR_VERSION = "0.1.0";

async function main(): Promise<void> {
  const t0 = process.hrtime.bigint();

  const cfg = await readConfig();
  if (cfg.$schema !== SCHEMA) {
    fatal("E_SCHEMA", `incompatible schema: got=${cfg.$schema} want=${SCHEMA}`);
  }

  const initPayload: InitPayload = {
    sidecar_version: SIDECAR_VERSION,
    node_version: process.version,
    ts_morph_version: readTsMorphVersion(),
  };
  emit("init", initPayload);

  if (!fs.existsSync(cfg.root)) {
    fatal("E_ROOT_MISSING", `root does not exist: ${cfg.root}`);
  }

  const repoURNBase = `urn:ce:code:${cfg.repo}`;
  // URNs no formato `urn:ce:code:<repo>:<kind>/<id>` — `repoURNBase` é
  // concatenado com `:<kind>/...` nos extractors (NÃO `/` entre repo e kind).
  const verbose = cfg.verbose ?? false;
  const stats = newStats();

  const services = collectServices(cfg.root, cfg.skip_dirs ?? []);
  stats.services = services.length;
  emitLog(`discovered ${services.length} services`, verbose);

  for (const svc of services) {
    const serviceURN = `${repoURNBase}:service/${svc.modulePath}`;
    emitLog(`service: ${svc.slug} (${svc.modulePath})`, verbose);

    // Modules
    const moduleNs = collectModules(cfg.root, svc, serviceURN);
    stats.modules += moduleNs.length;

    // ts-morph project para este Service
    const project = createProject(cfg.root, svc.absPath, verbose);

    // Types/Variables
    const tv = collectTypesAndVariables(project, cfg.root, repoURNBase, svc);
    stats.types += tv.types;
    stats.variables += tv.variables;

    // Functions
    const fns = collectFunctions(project, cfg.root, repoURNBase, svc, serviceURN);
    stats.functions += fns.count;

    // Endpoints
    stats.endpoints += collectExpressEndpoints(project, cfg.root, svc);
    stats.endpoints += collectNestJsEndpoints(project, cfg.root, svc);

    // Calls
    stats.calls += collectCalls(project, cfg.root, repoURNBase, svc, fns.emittedURNs);

    // Frameworks (DependsOn Service→Framework)
    collectFrameworks(cfg.root, repoURNBase, svc);
  }

  const t1 = process.hrtime.bigint();
  const done: DonePayload = {
    services: stats.services,
    modules: stats.modules,
    endpoints: stats.endpoints,
    functions: stats.functions,
    calls: stats.calls,
    types: stats.types,
    variables: stats.variables,
    elapsed_ns: Number(t1 - t0),
  };
  emit("done", done);
}

function createProject(repoRoot: string, serviceRoot: string, verbose: boolean): Project {
  // Procura tsconfig.json no service root; se não houver, cria Project
  // sem tsconfig (usa defaults). Em ambos casos, NÃO carrega tipos do
  // node_modules pesadamente (skipFileDependencyResolution=true) para
  // manter o MVP rápido — sacrifica precisão em alguns Targets, ok.
  const tsconfig = path.join(serviceRoot, "tsconfig.json");
  let project: Project;
  if (fs.existsSync(tsconfig)) {
    project = new Project({
      tsConfigFilePath: tsconfig,
      skipAddingFilesFromTsConfig: false,
      skipFileDependencyResolution: true,
      skipLoadingLibFiles: true,
    });
  } else {
    project = new Project({
      compilerOptions: { target: 99, module: 99, strict: false },
      skipFileDependencyResolution: true,
      skipLoadingLibFiles: true,
    });
    // Adiciona arquivos manualmente
    project.addSourceFilesAtPaths([
      `${serviceRoot}/**/*.ts`,
      `!${serviceRoot}/**/node_modules/**`,
      `!${serviceRoot}/**/dist/**`,
      `!${serviceRoot}/**/build/**`,
      `!${serviceRoot}/**/*.d.ts`,
    ]);
  }
  if (verbose) {
    emitLog(`project loaded: ${project.getSourceFiles().length} files`, verbose);
  }
  return project;
}

async function readConfig(): Promise<ConfigEnvelope> {
  const chunks: Buffer[] = [];
  for await (const c of process.stdin) chunks.push(c as Buffer);
  const raw = Buffer.concat(chunks).toString("utf-8").trim();
  if (!raw) {
    fatal("E_CONFIG", "no config received on stdin");
  }
  try {
    return JSON.parse(raw) as ConfigEnvelope;
  } catch (e) {
    fatal("E_CONFIG", `invalid JSON config: ${(e as Error).message}`);
  }
}

function readTsMorphVersion(): string | undefined {
  try {
    const p = require.resolve("ts-morph/package.json");
    const pkg = JSON.parse(fs.readFileSync(p, "utf-8")) as { version?: string };
    return pkg.version;
  } catch {
    return undefined;
  }
}

main().catch((err) => {
  fatal("E_UNCAUGHT", err?.message ?? String(err));
});
