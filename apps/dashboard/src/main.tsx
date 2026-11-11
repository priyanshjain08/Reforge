import React, { useState } from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter, Route, Routes } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "./index.css";
import Layout from "./components/Layout";
import { useUi } from "./stores/auth";
import { Btn, Card, inputCls } from "./components/Ui";
import { api } from "./services/api";
import CommandCenter from "./pages/CommandCenter";
import Monitoring from "./pages/Monitoring";
import Incidents from "./pages/Incidents";
import RootCause from "./pages/RootCause";
import Recovery from "./pages/Recovery";
import Topology from "./pages/Topology";
import ChaosLab from "./pages/ChaosLab";
import Policies from "./pages/Policies";
import Agents from "./pages/Agents";
import Audit from "./pages/Audit";
import Reports from "./pages/Reports";
import SettingsPage from "./pages/Settings";

const qc = new QueryClient({ defaultOptions: { queries: { refetchInterval: 5000, retry: 1, staleTime: 1000 } } });

function Login() {
  const setToken = useUi((s) => s.setToken);
  const [t, setT] = useState("");
  const [err, setErr] = useState("");
  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setToken(t.trim());
    try {
      await api("/dashboard");
      setErr("");
    } catch {
      setToken("");
      setErr("That token was rejected by the REFORGE core.");
    }
  }
  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <Card title="Sign in to REFORGE" className="w-full max-w-md">
        <form onSubmit={submit} className="space-y-3">
          <p className="text-sm text-slate-400">Enter the API token from your .env file (REFORGE_ADMIN_TOKEN).</p>
          <input className={inputCls} type="password" value={t} onChange={(e) => setT(e.target.value)} placeholder="API token" autoFocus />
          {err && <p className="text-sm text-bad">{err}</p>}
          <Btn type="submit" kind="primary">Sign in</Btn>
        </form>
      </Card>
    </div>
  );
}

function App() {
  const token = useUi((s) => s.token);
  if (!token) return <Login />;
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<CommandCenter />} />
        <Route path="monitoring" element={<Monitoring />} />
        <Route path="incidents" element={<Incidents />} />
        <Route path="incidents/:id" element={<Incidents />} />
        <Route path="rca" element={<RootCause />} />
        <Route path="recovery" element={<Recovery />} />
        <Route path="topology" element={<Topology />} />
        <Route path="chaos" element={<ChaosLab />} />
        <Route path="policies" element={<Policies />} />
        <Route path="agents" element={<Agents />} />
        <Route path="audit" element={<Audit />} />
        <Route path="reports" element={<Reports />} />
        <Route path="settings" element={<SettingsPage />} />
      </Route>
    </Routes>
  );
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <QueryClientProvider client={qc}>
      <BrowserRouter><App /></BrowserRouter>
    </QueryClientProvider>
  </React.StrictMode>,
);
