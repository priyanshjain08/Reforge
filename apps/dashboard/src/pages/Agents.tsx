import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../services/api";
import { Agent } from "../types";
import { Btn, Card, Empty, Field, PageTitle, StatusBadge, Table, inputCls } from "../components/Ui";
import { fmtTime } from "../services/format";
import { useAct } from "../hooks/useAct";

export default function Agents() {
  const act = useAct();
  const q = useQuery({ queryKey: ["agents"], queryFn: () => api<Agent[]>("/agents") });
  const [name, setName] = useState("");
  const [targets, setTargets] = useState("");
  const [perms, setPerms] = useState<string[]>(["metrics.read", "containers.inspect"]);
  const [secret, setSecret] = useState<string | null>(null);
  const all = ["metrics.read", "containers.inspect", "containers.restart"];
  return (
    <>
      <PageTitle title="Agent Management" sub="Agents report host metrics and execute only allowlisted actions on explicitly authorised targets." />
      <Card>
        {!q.data?.length ? <Empty title="No agents registered" /> : (
          <Table head={["Agent", "Status", "Host", "Last heartbeat", "Permissions", "Targets", ""]}>
            {q.data.map((a) => (
              <tr key={a.id}>
                <td className="px-3 py-2">{a.name}<div className="font-mono text-xs text-slate-500">{a.id}</div></td>
                <td className="px-3 py-2"><StatusBadge value={a.revoked ? "revoked" : a.connected ? "connected" : "disconnected"} /></td>
                <td className="px-3 py-2 text-xs">{a.hostname || "—"}<div className="text-slate-500">{a.os} {a.version && `· v${a.version}`}</div></td>
                <td className="px-3 py-2 text-xs">{fmtTime(a.last_heartbeat)}</td>
                <td className="px-3 py-2 text-xs">{a.permissions?.join(", ")}</td>
                <td className="px-3 py-2 text-xs">{a.targets?.join(", ") || "—"}</td>
                <td className="px-3 py-2">{!a.revoked && <Btn kind="danger" onClick={() => window.confirm("Revoke this agent? It will be rejected immediately.") && act(`/agents/${a.id}/revoke`)}>Revoke</Btn>}</td>
              </tr>
            ))}
          </Table>
        )}
      </Card>
      <Card title="Register a new agent" className="mt-4">
        <form className="grid gap-3 md:grid-cols-4" onSubmit={async (e) => {
          e.preventDefault();
          const r = await act<{ secret: string }>("/agents", "POST", { name, permissions: perms, targets: targets.split(",").map((s) => s.trim()).filter(Boolean) });
          if (r) setSecret(r.secret);
        }}>
          <Field label="Agent name"><input className={inputCls} required value={name} onChange={(e) => setName(e.target.value)} /></Field>
          <Field label="Allowed container targets (comma separated)"><input className={inputCls} value={targets} onChange={(e) => setTargets(e.target.value)} /></Field>
          <fieldset className="text-xs text-slate-400">
            <legend className="mb-1">Permissions</legend>
            {all.map((p) => <label key={p} className="mr-3 inline-flex items-center gap-1"><input type="checkbox" checked={perms.includes(p)} onChange={() => setPerms(perms.includes(p) ? perms.filter((x) => x !== p) : [...perms, p])} />{p}</label>)}
          </fieldset>
          <div className="flex items-end"><Btn type="submit" kind="primary">Create agent</Btn></div>
        </form>
        {secret && <p className="mt-3 rounded border border-warn p-2 text-sm text-warn">Copy this agent secret now. It is shown only once: <code className="break-all text-slate-100">{secret}</code></p>}
      </Card>
    </>
  );
}
