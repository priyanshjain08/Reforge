import { ReactNode } from "react";

const tone: Record<string, string> = {
  healthy: "bg-ok/15 text-ok", successful: "bg-ok/15 text-ok", resolved: "bg-ok/15 text-ok", connected: "bg-ok/15 text-ok", running: "bg-ok/15 text-ok",
  confirmed: "bg-ok/15 text-ok", completed: "bg-ok/15 text-ok", success: "bg-ok/15 text-ok",
  degraded: "bg-warn/15 text-warn", warning: "bg-warn/15 text-warn", investigating: "bg-warn/15 text-warn", suspected: "bg-warn/15 text-warn",
  recovering: "bg-accent/15 text-accent", executing: "bg-accent/15 text-accent", verifying: "bg-accent/15 text-accent", validating: "bg-accent/15 text-accent",
  awaiting_approval: "bg-accent-violet/20 text-violet-300", open: "bg-bad/15 text-bad",
  failed: "bg-bad/15 text-bad", critical: "bg-bad/15 text-bad", escalated: "bg-bad/15 text-bad", rolled_back: "bg-bad/15 text-bad", exited: "bg-bad/15 text-bad",
  disconnected: "bg-bad/15 text-bad", revoked: "bg-bad/15 text-bad",
};

export function StatusBadge({ value }: { value: string }) {
  return (
    <span className={`inline-flex items-center rounded px-2 py-0.5 text-xs font-medium ${tone[value] ?? "bg-ink-600 text-slate-300"}`}>
      {value.replace(/_/g, " ")}
    </span>
  );
}

export function Card({ title, children, right, className = "" }: { title?: string; children: ReactNode; right?: ReactNode; className?: string }) {
  return (
    <section className={`rounded-lg border border-ink-600 bg-ink-800 shadow-sm ${className}`}>
      {title && (
        <header className="flex items-center justify-between border-b border-ink-600 px-4 py-2.5">
          <h3 className="text-sm font-semibold text-slate-100">{title}</h3>
          {right}
        </header>
      )}
      <div className="p-4">{children}</div>
    </section>
  );
}

export function Stat({ label, value, hint }: { label: string; value: ReactNode; hint?: string }) {
  return (
    <div className="rounded-lg border border-ink-600 bg-ink-800 p-4">
      <div className="text-xs uppercase tracking-wide text-slate-400">{label}</div>
      <div className="mt-1 text-2xl font-semibold text-slate-50">{value}</div>
      {hint && <div className="mt-1 text-xs text-slate-500">{hint}</div>}
    </div>
  );
}

export function Empty({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="rounded-lg border border-dashed border-ink-600 p-8 text-center">
      <div className="font-medium text-slate-200">{title}</div>
      {children && <div className="mt-1 text-sm text-slate-400">{children}</div>}
    </div>
  );
}

export function PageTitle({ title, sub, right }: { title: string; sub?: string; right?: ReactNode }) {
  return (
    <div className="mb-5 flex items-end justify-between">
      <div>
        <h1 className="text-xl font-semibold text-slate-50">{title}</h1>
        {sub && <p className="text-sm text-slate-400">{sub}</p>}
      </div>
      {right}
    </div>
  );
}

export function Btn({ children, onClick, kind = "default", disabled, type = "button" }: {
  children: ReactNode; onClick?: () => void; kind?: "default" | "primary" | "danger"; disabled?: boolean; type?: "button" | "submit";
}) {
  const k = { default: "bg-ink-600 hover:bg-ink-700 text-slate-100", primary: "bg-accent text-ink-950 hover:bg-cyan-300", danger: "bg-bad/90 hover:bg-bad text-ink-950" }[kind];
  return (
    <button type={type} disabled={disabled} onClick={onClick} className={`rounded px-3 py-1.5 text-sm font-medium transition disabled:opacity-40 ${k}`}>
      {children}
    </button>
  );
}

export const inputCls = "w-full rounded border border-ink-600 bg-ink-900 px-2.5 py-1.5 text-sm text-slate-100 outline-none focus:border-accent";

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block text-xs text-slate-400">
      <span className="mb-1 block">{label}</span>
      {children}
    </label>
  );
}

export function Table({ head, children }: { head: string[]; children: ReactNode }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-sm">
        <thead className="text-xs uppercase text-slate-400">
          <tr>{head.map((h) => <th key={h} className="px-3 py-2 font-medium">{h}</th>)}</tr>
        </thead>
        <tbody className="divide-y divide-ink-600">{children}</tbody>
      </table>
    </div>
  );
}
