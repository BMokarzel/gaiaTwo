// Rail vertical à esquerda — navegação principal entre as views
// "Services" e "Endpoints". Detalhe de endpoint é alcançado via clique
// nas listas, não daqui.

import { NavLink } from "react-router-dom";
import styles from "./LeftRail.module.css";

const items: { to: string; label: string; glyph: string }[] = [
  { to: "/services", label: "Services", glyph: "▣" },
  { to: "/endpoints", label: "Endpoints", glyph: "→" },
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
