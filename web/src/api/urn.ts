// Helpers para manipulação de URNs do costEngine.
//
// Formato: `urn:ce:<provider>:<repo>:<kind>/<id>`
// Ex.: `urn:ce:code:sample-api:service/.`
//      `urn:ce:code:sample-api:endpoint/.!GET:/users/:id`
//
// Como o `id` pode conter `:`, qualquer parsing por split:`:` precisa
// se limitar aos 4 primeiros segmentos. Daí a função `repoFromURN`
// abaixo — não usar regex global porque os IDs têm vários `:`.

import type { URN } from "./types";

/**
 * Extrai o slug do repo da URN. Retorna `""` se a URN não tiver pelo
 * menos 4 segmentos `:`.
 */
export function repoFromURN(urn: URN): string {
  const parts = urn.split(":", 5);
  if (parts.length < 4) return "";
  return parts[3] ?? "";
}

/**
 * Constrói a URN canônica do Service raiz de um repo
 * (módulo `.`, conforme convenção do coletor TS).
 */
export function buildServiceURN(repo: string): URN {
  return `urn:ce:code:${repo}:service/.`;
}
