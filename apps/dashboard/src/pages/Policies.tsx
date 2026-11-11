import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../services/api";
import { Policy } from "../types";
import { Btn, Card, Empty, Field, PageTitle, StatusBadge, Table, inputCls } from "../components/Ui";
import { validatePolicyInput } from "../services/format";
import { useAct } from "../hooks/useAct";

const blank = { name: "", service: "*", min_failures: 3, container_states: "exited,dead,unhealthy", min_confidence: 0.6, action_id: "restart_container", mode: "approval", retry_limit: 3, cooldown_s: 30, verify_timeout_s: 30, priority: 50, enabled: true };

export default function Policies() {
  const act = useAct();
  const q = useQuery({ queryKey: ["policies"], queryFn: () => api<Policy[]>("/policies") });
  const [f, setF] = useState(blank);
  const [err, setErr] = useState<string | null>(null);
  const [t, setT] = useState({ service: "payment-service", consecutive_failures: 3, container_state: "exited", confidence: 0.9, action: "restart_container" });
  const [res, setRes] = useState<any>(null);
  const set = (k: string, v: any) => setF({ ...f, [k]: v });
  return (
    <>
      <PageTitle title="Recovery Policies" sub="Policies decide whether a diagnosed problem may be fixed automatically. Default mode is safe: simulate or approval." />
      <Card title="Policies">
        {!q.data?.length ? <Empty title="No policies" /> : (
          <Table head={["Priority", "Name", "Scope", "Action", "Mode", "Limits", "Enabled", ""]}>
            {q.data.map((p) => (
              <tr key={p.id}>
                <td className="px-3 py-2">{p.priority}</td><td className="px-3 py-2">{p.name}</td><td className="px-3 py-2">{p.service}</td>
                <td className="px-3 py-2">{p.action_id}</td><td className="px-3 py-2"><StatusBadge value={p.mode} /></td>
                <td className="px-3 py-2 text-xs text-slate-400">≥{p.min_failures} failures · {p.retry_limit} tries · {p.cooldown_s}s cooldown · verify {p.verify_timeout_s}s</td>
                <td className="px-3 py-2"><input type="checkbox" aria-label={`Enable ${p.name}`} checked={p.enabled} onChange={() => act(`/policies/${p.id}`, "PUT", { ...p, enabled: !p.enabled })} /></td>
                <td className="px-3 py-2 space-x-1">
                  {(["simulate", "approval", "auto"] as const).filter((m) => m !== p.mode).map((m) => <Btn key={m} onClick={() => act(`/policies/${p.id}`, "PUT", { ...p, mode: m })}>{m}</Btn>)}
                  <Btn kind="danger" onClick={() => window.confirm(`Delete "${p.name}"?`) && act(`/policies/${p.id}`, "DELETE")}>Delete</Btn>
                </td>
              </tr>
            ))}
          </Table>
        )}
      </Card>
      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <Card title="Create policy">
          <form className="grid grid-cols-2 gap-3" onSubmit={async (e) => {
            e.preventDefault();
            const msg = validatePolicyInput(f);
            setErr(msg);
            if (msg) return;
            await act("/policies", "POST", { ...f, container_states: f.container_states.split(",").map((s) => s.trim()).filter(Boolean) });
          }}>
            <Field label="Name"><input className={inputCls} value={f.name} onChange={(e) => set("name", e.target.value)} /></Field>
            <Field label='Service ("*" = all)'><input className={inputCls} value={f.service} onChange={(e) => set("service", e.target.value)} /></Field>
            <Field label="Action"><select className={inputCls} value={f.action_id} onChange={(e) => set("action_id", e.target.value)}><option>restart_container</option><option>app_recovery_hook</option></select></Field>
            <Field label="Mode"><select className={inputCls} value={f.mode} onChange={(e) => set("mode", e.target.value)}><option>simulate</option><option>approval</option><option>auto</option></select></Field>
            <Field label="Allowed container states (comma separated)"><input className={inputCls} value={f.container_states} onChange={(e) => set("container_states", e.target.value)} /></Field>
            <Field label="Min confidence (0-1)"><input className={inputCls} type="number" step="0.05" value={f.min_confidence} onChange={(e) => set("min_confidence", +e.target.value)} /></Field>
            <Field label="Min failures"><input className={inputCls} type="number" value={f.min_failures} onChange={(e) => set("min_failures", +e.target.value)} /></Field>
            <Field label="Retry limit"><input className={inputCls} type="number" value={f.retry_limit} onChange={(e) => set("retry_limit", +e.target.value)} /></Field>
            <Field label="Cooldown (s)"><input className={inputCls} type="number" value={f.cooldown_s} onChange={(e) => set("cooldown_s", +e.target.value)} /></Field>
            <Field label="Verify timeout (s)"><input className={inputCls} type="number" value={f.verify_timeout_s} onChange={(e) => set("verify_timeout_s", +e.target.value)} /></Field>
            <Field label="Priority (lower runs first)"><input className={inputCls} type="number" value={f.priority} onChange={(e) => set("priority", +e.target.value)} /></Field>
            <div className="flex items-end"><Btn type="submit" kind="primary">Create</Btn></div>
            {err && <p className="col-span-2 text-sm text-bad">{err}</p>}
          </form>
        </Card>
        <Card title="Test a policy (dry run, executes nothing)">
          <div className="grid grid-cols-2 gap-3">
            <Field label="Service"><input className={inputCls} value={t.service} onChange={(e) => setT({ ...t, service: e.target.value })} /></Field>
            <Field label="Container state"><input className={inputCls} value={t.container_state} onChange={(e) => setT({ ...t, container_state: e.target.value })} /></Field>
            <Field label="Consecutive failures"><input className={inputCls} type="number" value={t.consecutive_failures} onChange={(e) => setT({ ...t, consecutive_failures: +e.target.value })} /></Field>
            <Field label="Diagnosis confidence"><input className={inputCls} type="number" step="0.05" value={t.confidence} onChange={(e) => setT({ ...t, confidence: +e.target.value })} /></Field>
            <Field label="Recommended action"><select className={inputCls} value={t.action} onChange={(e) => setT({ ...t, action: e.target.value })}><option>restart_container</option><option>app_recovery_hook</option></select></Field>
            <div className="flex items-end"><Btn onClick={async () => setRes(await act("/policies/test", "POST", t))}>Evaluate</Btn></div>
          </div>
          {res && <p className="mt-3 rounded bg-ink-900 p-2 text-sm">{res.matched ? <>Matched <b>{res.policy.name}</b> in <b>{res.policy.mode}</b> mode. Nothing was executed.</> : <>No policy matches: {res.reason}</>}</p>}
        </Card>
      </div>
    </>
  );
}
