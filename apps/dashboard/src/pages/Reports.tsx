import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, download } from "../services/api";
import { Incident } from "../types";
import { Btn, Card, Empty, PageTitle, Stat, Table } from "../components/Ui";
import { fmtSeconds } from "../services/format";

export default function Reports() {
  const [days, setDays] = useState(7);
  const from = new Date(Date.now() - days * 86400000).toISOString();
  const s = useQuery({ queryKey: ["report", days], queryFn: () => api<any>(`/reports/summary?from=${encodeURIComponent(from)}`) });
  const inc = useQuery({ queryKey: ["incidents", "report"], queryFn: () => api<{ items: Incident[] }>("/incidents?page_size=20") });
  const d = s.data;
  const rows = (o: Record<string, number> | undefined) => Object.entries(o ?? {}).sort((a, b) => b[1] - a[1]);
  return (
    <>
      <PageTitle title="Reports" sub="Summaries computed from recorded incidents, operations and experiments" right={
        <select aria-label="Range" className="rounded border border-ink-600 bg-ink-900 px-2 py-1 text-sm" value={days} onChange={(e) => setDays(+e.target.value)}>
          <option value={1}>Last 24 hours</option><option value={7}>Last 7 days</option><option value={30}>Last 30 days</option><option value={3650}>All time</option>
        </select>} />
      {!d ? <p className="text-slate-400">Loading…</p> : (
        <>
          <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
            <Stat label="Incidents" value={d.total_incidents} hint={`${d.resolved_incidents} resolved`} />
            <Stat label="Recovery success rate" value={d.recovery_success_rate === null ? "—" : `${Math.round(d.recovery_success_rate * 100)}%`} hint={`${d.successful_recoveries} ok / ${d.failed_recoveries} failed`} />
            <Stat label="Avg time to recover" value={fmtSeconds(d.avg_time_to_recover_s)} />
            <Stat label="Avg time to detect" value={fmtSeconds(d.avg_time_to_detect_s)} hint="chaos experiments only" />
          </div>
          <p className="mt-2 text-xs text-slate-500">{d.note}</p>
          <div className="mt-4 grid gap-4 md:grid-cols-3">
            {([["Most affected services", d.most_affected_services], ["Failure types", d.failure_types], ["Most effective policies", d.effective_policies]] as const).map(([t, o]) => (
              <Card key={t} title={t}>{!rows(o).length ? <Empty title="No data" /> : <ul className="space-y-1 text-sm">{rows(o).map(([k, v]) => <li key={k} className="flex justify-between"><span>{k}</span><b>{v}</b></li>)}</ul>}</Card>
            ))}
          </div>
        </>
      )}
      <Card title="Incident reports" className="mt-4">
        {!inc.data?.items.length ? <Empty title="No incidents to report on" /> : (
          <Table head={["Incident", "Service", "Status", ""]}>
            {inc.data.items.map((i) => (
              <tr key={i.id}><td className="px-3 py-2">{i.title}</td><td className="px-3 py-2">{i.service}</td><td className="px-3 py-2">{i.status}</td>
                <td className="px-3 py-2 space-x-1"><Btn onClick={() => download(`/reports/incidents/${i.id}?format=pdf`, `incident-${i.id}.pdf`)}>PDF</Btn><Btn onClick={() => download(`/reports/incidents/${i.id}?format=json`, `incident-${i.id}.json`)}>JSON</Btn></td></tr>
            ))}
          </Table>
        )}
      </Card>
    </>
  );
}
