import { SCHEMA, EventKind } from "./proto";

// Stats acumula contagens para o `done` final.
export interface Stats {
  services: number;
  modules: number;
  endpoints: number;
  functions: number;
  calls: number;
  types: number;
  variables: number;
}

export function newStats(): Stats {
  return { services: 0, modules: 0, endpoints: 0, functions: 0, calls: 0, types: 0, variables: 0 };
}

/**
 * Emit imprime uma linha NDJSON em stdout. Cada evento carrega
 * `$schema` para versionamento (Go-side aborta se incompatível).
 */
export function emit(kind: EventKind, payload: unknown): void {
  const line = JSON.stringify({ $schema: SCHEMA, kind, payload });
  // stdout síncrono: garantia de ordem com stderr (logs).
  process.stdout.write(line + "\n");
}

export function emitLog(msg: string, verbose: boolean): void {
  if (!verbose) return;
  process.stderr.write(`[sidecar] ${msg}\n`);
}

export function fatal(code: string, message: string, where?: string): never {
  emit("error", { code, message, where });
  process.exit(1);
}
