// HTTP client thin wrapper. Todas as chamadas batem em `/api/*` que o
// Vite proxia para o backend Go (default :8080). Em produção, o mesmo
// path pode ser servido pelo mesmo binário ou por um reverse-proxy.
//
// O backend retorna ou JSON puro (success) ou RFC 7807 Problem Detail
// (errors). Aqui distinguimos os dois e levantamos `ApiError` no segundo
// caso, deixando a UI tratar via `useQuery({onError})`.

const BASE = "/api";

export class ApiError extends Error {
  status: number;
  code?: string;
  detail?: unknown;
  constructor(status: number, message: string, code?: string, detail?: unknown) {
    super(message);
    this.status = status;
    this.code = code;
    this.detail = detail;
  }
}

export async function apiGet<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { Accept: "application/json" },
    signal,
  });
  return handle<T>(res);
}

async function handle<T>(res: Response): Promise<T> {
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  const json = text ? safeParse(text) : null;
  if (!res.ok) {
    const code = (json && typeof json === "object" && "code" in json && typeof json.code === "string")
      ? json.code
      : undefined;
    const msg = (json && typeof json === "object" && "detail" in json && typeof json.detail === "string")
      ? json.detail
      : res.statusText || `HTTP ${res.status}`;
    throw new ApiError(res.status, msg, code, json);
  }
  return json as T;
}

function safeParse(s: string): unknown {
  try {
    return JSON.parse(s);
  } catch {
    return s;
  }
}
