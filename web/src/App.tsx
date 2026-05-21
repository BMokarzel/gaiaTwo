// App root — aplica tema, define o shell (top bar + left rail + main)
// e roteia os 3 níveis (F-031):
//   /services
//   /services/:repo                         → redirect → architecture
//   /services/:repo/architecture
//   /services/:repo/endpoints
//   /services/:repo/endpoints/:urn/*        → steps do endpoint

import { useEffect } from "react";
import { Navigate, Route, Routes, useParams } from "react-router-dom";
import { useUIStore } from "@/store/uiStore";
import { TopBar } from "@/shell/TopBar";
import { LeftRail } from "@/shell/LeftRail";
import { ServicesPage } from "@/pages/ServicesPage";
import { ServiceArchitecturePage } from "@/pages/ServiceArchitecturePage";
import { ServiceEndpointsPage } from "@/pages/ServiceEndpointsPage";
import { EndpointStepsPage } from "@/pages/EndpointStepsPage";
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
          <Route
            path="/services/:repo"
            element={<RepoRedirect />}
          />
          <Route
            path="/services/:repo/architecture"
            element={<ServiceArchitecturePage />}
          />
          <Route
            path="/services/:repo/endpoints"
            element={<ServiceEndpointsPage />}
          />
          {/* URN contém `:` e `/` — usamos splat (`*`) e lemos via
              useParams()["*"]. */}
          <Route
            path="/services/:repo/endpoints/:urn/*"
            element={<EndpointStepsPage />}
          />
          <Route path="*" element={<Navigate to="/services" replace />} />
        </Routes>
      </main>
    </div>
  );
}

// Redireciona `/services/:repo` para a sub-view padrão (architecture).
function RepoRedirect() {
  const { repo = "" } = useParams();
  return <Navigate to={`/services/${repo}/architecture`} replace />;
}
