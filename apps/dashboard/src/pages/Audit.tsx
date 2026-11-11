import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, download } from "../services/api";
import { AuditEntry } from "../types";
import { Btn, Card, Empty, PageTitle, StatusBadge, Table, inputCls } from "../components/Ui";

export default function Audit() {
  const [q, setQ] = useState("");
  const [action, setAction] = useState("");
  const [page, setPage] = useState(1);
  const qs = `q=${encodeURIComponent(q)}&action=${encodeURIComponent(action)}&page=${page}`;
  const r = useQuery({ queryKey: ["audit", qs], queryFn: () => api<{ items: AuditEntry[]; total: number }>(`/audit?${qs}`) });
  return (
    <>
      <PageTitle title="Audit Logs" sub="Append-only record of every automated and manual action" right={<Btn onClick={() => download(`/audit?${qs}&format=csv`, "reforge-audit.csv")}>Export CSV</Btn>} />
      <div className="mb-3 flex gap-2">
        <input className={inputCls + " max-w-xs"} placeholder="Search" value={q} onChange={(e) => { setQ(e.target.value); setPage(1); }} />
        <input className={inputCls + " max-w-xs"} placeholder="Action prefix (e.g. recovery.)" value={action} onChange={(e) => { setAction(e.target.value); setPage(1); }} />
      </div>
      <Card>
        {!r.data?.items.length ? <Empty title="No audit entries" /> : (
          <>
            <Table head={["Time", "Actor", "Action", "Target", "Outcome", "Incident"]}>
              {r.data.items.map((a) => (
                <tr key={a.id}>
                  <td className="px-3 py-2 text-xs">{new Date(a.ts).toLocaleString()}</td><td className="px-3 py-2">{a.actor}</td>
                  <td className="px-3 py-2 text-accent">{a.action}</td><td className="px-3 py-2">{a.target}</td>
                  <td className="px-3 py-2"><StatusBadge value={a.outcome} /></td><td className="px-3 py-2 font-mono text-xs">{a.incident_id || "—"}</td>
                </tr>
              ))}
            </Table>
            <div className="mt-3 flex items-center gap-2 text-sm text-slate-400">
              <Btn disabled={page === 1} onClick={() => setPage(page - 1)}>Prev</Btn>
              <span>Page {page} · {r.data.total} entries</span>
              <Btn disabled={page * 50 >= r.data.total} onClick={() => setPage(page + 1)}>Next</Btn>
            </div>
          </>
        )}
      </Card>
    </>
  );
}
