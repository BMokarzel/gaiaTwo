// Rail vertical à esquerda — entrada única em "Services". Os demais
// níveis (arquitetura, endpoints do service, steps de endpoint) são
// alcançados via clique nas listas / nodes, mantendo a navegação
// hierárquica (F-031).

import { NavLink } from "react-router-dom";
import styles from "./LeftRail.module.css";

const items: { to: string; label: string; glyph: string }[] = [
  { to: "/services", label: "Services", glyph: "▣" },
];

export function LeftRail() {
  return (
    <nav className={styles.rail}>
      <div className={styles.label}>graph</div>
      {items.map((it) => (
        <NavLink
          key={it.to}
          to={it.to}
          className={({ isActive }) =>
            `${styles.link} ${isActive ? styles.linkActive : ""}`
          }
        >
          <span className={styles.glyph}>{it.glyph}</span>
          {it.label}
        </NavLink>
      ))}
    </nav>
  );
}
