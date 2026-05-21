// Painel lateral direito — abre ao clicar num nó. Mostra `urn`,
// `kind`, campos relevantes de `data` por Kind, valid_from, confidence
// e location.file:line quando houver.
//
// Sem source snippet — coletor não persiste conteúdo do arquivo.

import { useEffect } from "react";
import type { NodeView } from "@/api/types";
import styles from "./DetailPanel.module.css";

interface Props {
  view: NodeView;
  kind: string;
  onClose: () => void;
}

export function DetailPanel({ view, kind, onClose }: Props) {
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const loc = (view.data as { location?: { file?: string; line_init?: number } }).location;
  const fileLine = loc?.file
    ? `${loc.file}${loc.line_init ? `:${loc.line_init}` : ""}`
    : null;

  return (
    <aside className={styles.panel}>
      <header className={styles.header}>
        <span className={styles.kind}>{kind}</span>
        <button
          className={styles.close}
          onClick={onClose}
          aria-label="close detail panel"
        >
          ×
        </button>
      </header>

      <div className={styles.section}>
        <div className={styles.label}>urn</div>
        <div className={styles.value}>{view.urn}</div>
      </div>

      <div className={styles.section}>
        <div className={styles.label}>data</div>
        <DataFields data={view.data} kind={kind} />
      </div>

      <div className={styles.section}>
        <div className={styles.label}>bitemporal</div>
        <div className={styles.kv}>
          <span>valid_from</span>
          <span>{view.valid_from}</span>
        </div>
        <div className={styles.kv}>
          <span>observed_at</span>
          <span>{view.observed_at}</span>
        </div>
        <div className={styles.kv}>
          <span>confidence</span>
          <span>{view.confidence.toFixed(2)}</span>
        </div>
        <div className={styles.kv}>
          <span>version</span>
          <span>{view.version}</span>
        </div>
      </div>

      {fileLine && (
        <div className={styles.section}>
          <div className={styles.label}>location</div>
          <div className={styles.value}>{fileLine}</div>
        </div>
      )}
    </aside>
  );
}

// Renderiza pares chave/valor dos campos de NodeView.data,
// filtrando location (já mostrado acima) e arrays/objetos vazios.
function DataFields({ data, kind: _kind }: { data: Record<string, unknown>; kind: string }) {
  const entries = Object.entries(data).filter(([k, v]) => {
    if (k === "location") return false;
    if (v == null || v === "") return false;
    if (Array.isArray(v) && v.length === 0) return false;
    return true;
  });

  if (entries.length === 0) {
    return <div className={styles.empty}>(no data)</div>;
  }

  return (
    <>
      {entries.map(([k, v]) => (
        <div className={styles.kv} key={k}>
          <span>{k}</span>
          <span className={styles.kvValue}>{formatVal(v)}</span>
        </div>
      ))}
    </>
  );
}

function formatVal(v: unknown): string {
  if (typeof v === "string") return v;
  if (typeof v === "number" || typeof v === "boolean") return String(v);
  if (Array.isArray(v)) return v.map(formatVal).join(", ");
  if (v && typeof v === "object") return JSON.stringify(v);
  return String(v);
}
