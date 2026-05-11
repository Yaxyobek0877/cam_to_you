import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { HashRouter, Routes, Route, Navigate } from "react-router-dom";
import { Layout } from "./components/Layout";
import { Dashboard } from "./pages/Dashboard";
import { Cameras } from "./pages/Cameras";
import { Streams } from "./pages/Streams";
import { Settings } from "./pages/Settings";

// React Query — server state (Go backend chaqiriqlari) cache va auto-refresh
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Wails IPC tezkor — har 5s'da fresh deb hisoblaymiz
      staleTime: 5_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      {/* HashRouter ishlatilgan — Wails embed.FS uchun yaxshi (URL'sis ishlaydi) */}
      <HashRouter>
        <Routes>
          <Route path="/" element={<Layout />}>
            <Route index element={<Navigate to="/dashboard" replace />} />
            <Route path="dashboard" element={<Dashboard />} />
            <Route path="cameras" element={<Cameras />} />
            <Route path="streams" element={<Streams />} />
            <Route path="settings" element={<Settings />} />
          </Route>
        </Routes>
      </HashRouter>
    </QueryClientProvider>
  );
}

export default App;
