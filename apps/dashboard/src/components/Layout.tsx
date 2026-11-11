import { NavLink, Outlet } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Activity, AlertTriangle, BarChart3, ClipboardList, FlaskConical, GitBranch, LayoutDashboard, Menu, Network, ScrollText, Settings, ShieldCheck, Wrench } from "lucide-react";
import { api } from "../services/api";
import { useUi } from "../stores/auth";
import { useLive } from "../hooks/useLive";

const nav = [
  ["/", "Command Center", LayoutDashboard], ["/monitoring", "System Monitoring", Activity], ["/incidents", "Incidents", AlertTriangle],
  ["/rca", "Root Cause Analysis", GitBranch], ["/recovery", "Recovery Operations", Wrench], ["/topology", "Service Topology", Network],
  ["/chaos", "Chaos Lab", FlaskConical], ["/policies", "Recovery Policies", ShieldCheck], ["/agents", "Agent Management", ClipboardList],
  ["/audit", "Audit Logs", ScrollText], ["/reports", "Reports", BarChart3], ["/settings", "Settings", Settings],
] as const;

export default function Layout() {
  useLive();
  const { collapsed, toggle } = useUi();
  const ks = useQuery({ queryKey: ["killswitch"], queryFn: () => api<{ enabled: boolean }>("/killswitch"), refetchInterval: 5000 });
  return (
    <div className="flex min-h-screen">
      <aside className={`${collapsed ? "w-14" : "w-60"} shrink-0 border-r border-ink-600 bg-ink-900 transition-all`}>
        <div className="flex h-14 items-center justify-between px-3">
          {!collapsed && <span className="bg-gradient-to-r from-accent to-accent-violet bg-clip-text text-lg font-bold tracking-widest text-transparent">REFORGE</span>}
          <button onClick={toggle} aria-label="Toggle sidebar" className="text-slate-400 hover:text-slate-100"><Menu size={18} /></button>
        </div>
        <nav className="space-y-0.5 px-2">
          {nav.map(([to, label, Icon]) => (
            <NavLink key={to} to={to} end={to === "/"} title={label}
              className={({ isActive }) => `flex items-center gap-3 rounded px-2.5 py-2 text-sm ${isActive ? "bg-ink-700 text-accent" : "text-slate-400 hover:bg-ink-800 hover:text-slate-100"}`}>
              <Icon size={16} />
              {!collapsed && label}
            </NavLink>
          ))}
        </nav>
      </aside>
      <main className="min-w-0 flex-1">
        {ks.data?.enabled && (
          <div className="bg-bad px-4 py-1.5 text-center text-sm font-semibold text-ink-950">
            EMERGENCY KILL SWITCH ACTIVE: automated recovery is disabled
          </div>
        )}
        <div className="p-6"><Outlet /></div>
      </main>
    </div>
  );
}
