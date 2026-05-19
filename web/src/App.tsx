// App root — aplica tema, define o shell (top bar + left rail + main)
// e roteia as 3 views (services list, endpoints list, endpoint detail).

import { useEffect } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { useUIStore } from "@/store/uiStore";
import { TopBar } from "@/shell/TopBar";
import { LeftRail } from "@/shell/LeftRail";
import { ServicesPage } from "@/pages/ServicesPage";
import { EndpointsPage } from "@/pages/EndpointsPage";
import { EndpointDetailPage } from "@/pages/EndpointDetailPage";
import styles from "./App.module.css";

export default function App() {
  const theme = useUIStore((s) => s.theme);

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
  }, [theme]);

  return (
    <div className={styles.shell}>
      <div className={styles.top}>
        <TopBar />
      </div>
      <div className={styles.rail}>
        <LeftRail />
      </div>
      <main className={styles.main}>
        <Routes>
          <Route path="/" element={<Navigate to="/services" replace />} />
          <Route path="/services" element={<ServicesPage />} />
          <Route path="/endpoints" element={<EndpointsPage />} />
          {/* URN contém `:` e `/` — usamos splat (`*`) e lemos via
              useParams()["*"]. */}
          <Route path="/endpoints/:urn/*" element={<EndpointDetailPage />} />
          <Route path="*" element={<Navigate to="/services" replace />} />
        </Routes>
      </main>
    </div>
  );
}
