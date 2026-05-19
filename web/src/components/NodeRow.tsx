// Linha clicável de listagem genérica. Recebe slots opcionais para
// um "tag" à esquerda (e.g., HTTP method) e meta à direita
// (e.g., framework, language).

import { Link } from "react-router-dom";
import styles from "./NodeRow.module.css";

interface NodeRowProps {
  to?: string;
  tag?: string;
  tagVariant?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  primary: string;
  secondary?: string;
  meta?: string;
}

export function NodeRow({
  to,
  tag,
  tagVariant,
  primary,
  secondary,
  meta,
}: NodeRowProps) {
  const inner = (
    <>
      {tag && (
        <span
          className={`${styles.tag} ${
            tagVariant
              ? `${styles.method} ${styles[tagVariant]}`
              : ""
          }`}
        >
          {tag}
        </span>
      )}
      <div className={styles.body}>
        <span className={styles.primary}>{primary}</span>
        {secondary && <span className={styles.secondary}>{secondary}</span>}
      </div>
      {meta && <span className={styles.meta}>{meta}</span>}
    </>
  );
  if (to) {
    return (
      <Link to={to} className={styles.row}>
        {inner}
      </Link>
    );
  }
  return <div className={styles.row}>{inner}</div>;
}
