// Top bar: brand + toggle de tema. Mantemos enxuta — navegação fica
// no LeftRail. Não duplica funcionalidade.

import { Link } from "react-router-dom";
import { useUIStore } from "@/store/uiStore";
import styles from "./TopBar.module.css";

export function TopBar() {
  const theme = useUIStore((s) => s.theme);
  const toggle = useUIStore((s) => s.toggleTheme);
  return (
    <header className={styles.bar}>
      <Link to="/" className={styles.brand}>
        <span className={styles.brandDot} />
        <span>costEngine</span>
      </Link>
      <span className={styles.spacer} />
      <button className={styles.themeBtn} onClick={toggle} title="Toggle theme">
        {theme === "dark" ? "◐ light" : "◑ dark"}
      </button>
    </header>
  );
}
