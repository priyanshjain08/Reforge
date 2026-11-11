import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api, download } from "../services/api";
import { Incident, Operation } from "../types";
import { Btn, Card, Empty, PageTitle, StatusBadge, Table, inputCls } from "../components/Ui";
import { fmtTime } from "../services/format";
import { useAct } from "../hooks/useAct";
import { DiagnosisView } from "./RootCause";

export default function Incidents() {
  const { id } = useParams();
  const nav = useNavigate();
  const [status, setStatus] = useState("");
  const [severity, setSeverity] = useState("");
  const [q, setQ] = useState("");
  const [sort, setSort] = useState("time");
  const list = useQuery({
    queryKey: ["incidents", status, severity, q, sort],
    queryFn: () => api<{ items: Incident[]; total: number }>(`/incidents?status=${status}&severity=${severity}&q=${encodeURIComponent(q)}&sort=${sort}`),
  });
  return (
    <>
      <PageTitle title="Incidents" sub="Detected problems, diagnoses, recoveries and full timelines" />
      <div className="mb-3 flex flex-wrap gap-2">
        <input className={inputCls + " max-w-xs"} placeholder="Search title, service or ID" value={q} onChange={(e) => setQ(e.target.value)} />
        <select aria-label="Status filter" className={inputCls + " max-w-[10rem]"} value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">All statuses</option>
          {["open", "investigating", "recovering", "resolved", "escalated"].map((s) => <option key={s}>{s}</option>)}
        </select>
        <select aria-label="Severity filter" className={inputCls + " max-w-[10rem]"} value={severity} onChange={(e) => setSeverity(e.target.value)}>
          <option value="">All severities</option>
          {["critical", "warning", "info"].map((s) => <option key={s}>{s}</option>)}
        </select>
        <select aria-label="Sort" className={inputCls + " max-w-[10rem]"} value={sort} onChange={(e) => setSort(e.target.value)}>
          <option value="time">Newest first</option><option value="severity">Severity first</option>
        </select>
      </div>
      <div className="grid gap-4 xl:grid-cols-2">
        <Card title={`Incidents (${list.data?.total ?? 0})`}>
          {!list.data?.items.length ? <Empty title="No incidents match" /> : (
            <Table head={["Title", "Severity", "Status", "Detected"]}>
              {list.data.items.map((i) => (
                <tr key={i.id} onClick={() => nav(`/incidents/${i.id}`)} className={`cursor-pointer hover:bg-ink-700 ${i.id === id ? "bg-ink-700" : ""}`}>
                  <td className="px-3 py-2">{i.title}<div className="font-mono text-xs text-slate-500">{i.id}</div></td>
                  <td className="px-3 py-2"><StatusBadge value={i.severity} /></td>
                  <td className="px-3 py-2"><StatusBadge value={i.status} /></td>
                  <td className="px-3 py-2 text-xs">{fmtTime(i.detected_at)}</td>
                </tr>
              ))}
            </Table>
          )}
        </Card>
        {id ? <Detail id={id} /> : <Empty title="Select an incident">Its timeline, diagnosis and recovery operations appear here.</Empty>}
      </div>
    </>
  );
}

function Detail({ id }: { id: string }) {
  const act = useAct();
  const [note, setNote] = useState("");
  const q = useQuery({ queryKey: ["incident", id], queryFn: () => api<{ incident: Incident; operations: Operation[] }>(`/incidents/${id}`) });
  if (!q.data) return <Card><p className="text-slate-400">{q.error ? (q.error as Error).message : "Loading…"}</p></Card>;
  const { incident: i, operations } = q.data;
  return (
    <div className="space-y-4">
      <Card title={i.title} right={<StatusBadge value={i.status} />}>
        <p className="text-sm text-slate-300">{i.description}</p>
        {i.summary && <p className="mt-2 rounded bg-ink-900 p-2 text-sm text-slate-200">Resolution: {i.summary}</p>}
        <p className="mt-2 text-xs text-slate-500">Assignee: {i.assignee || "unassigned"} · Acknowledged: {i.acknowledged ? "yes" : "no"}</p>
        <div className="mt-3 flex flex-wrap gap-2">
          <Btn onClick={() => act(`/incidents/${id}/ack`)} disabled={i.acknowledged}>Acknowledge</Btn>
          <Btn onClick={() => { const a = window.prompt("Assign to:"); if (a) act(`/incidents/${id}/assign`, "POST", { assignee: a }); }}>Assign</Btn>
          <Btn kind="danger" disabled={i.status === "resolved"} onClick={() => { const s = window.prompt("Resolution summary:"); if (s) act(`/incidents/${id}/resolve`, "POST", { summary: s }); }}>Resolve manually</Btn>
          <Btn onClick={() => download(`/reports/incidents/${id}?format=json`, `incident-${id}.json`)}>Report JSON</Btn>
          <Btn onClick={() => download(`/reports/incidents/${id}?format=pdf`, `incident-${id}.pdf`)}>Report PDF</Btn>
        </div>
      </Card>
      <Card title="Evidence"><ul className="list-disc pl-5 text-sm text-slate-300">{i.evidence?.map((e, n) => <li key={n}>{e}</li>)}</ul></Card>
      <Card title="Diagnosis">
        {!i.diagnoses?.length ? <p className="text-sm text-slate-400">Diagnosis pending…</p> : i.diagnoses.map((d, n) => <DiagnosisView key={n} d={d} />)}
      </Card>
      <Card title="Recovery operations">
        {!operations.length ? <p className="text-sm text-slate-400">None yet.</p> : operations.map((o) => (
          <div key={o.id} className="mb-2 rounded border border-ink-600 p-2 text-sm">
            <div className="flex justify-between"><span>{o.action_id} → {o.target}</span><StatusBadge value={o.state} /></div>
            {o.error && <div className="text-xs text-bad">{o.error}</div>}
          </div>
        ))}
      </Card>
      <Card title="Timeline">
        <ol className="space-y-1.5 text-sm">
          {i.events?.map((e, n) => <li key={n} className="flex gap-3"><span className="w-16 shrink-0 font-mono text-xs text-slate-500">{fmtTime(e.ts)}</span><span><b className="text-accent">{e.kind}</b> {e.detail}</span></li>)}
        </ol>
      </Card>
      <Card title="Investigation notes">
        {i.notes?.map((n, k) => <p key={k} className="mb-1 text-sm"><span className="text-slate-500">{n.author}:</span> {n.text}</p>)}
        <div className="mt-2 flex gap-2">
          <input className={inputCls} value={note} placeholder="Add a note" onChange={(e) => setNote(e.target.value)} />
          <Btn onClick={async () => { if (note.trim()) { await act(`/incidents/${id}/notes`, "POST", { text: note }); setNote(""); } }}>Add</Btn>
        </div>
      </Card>
    </div>
  );
}
