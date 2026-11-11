import { useQuery } from "@tanstack/react-query";
import { api } from "../services/api";
import { Agent, AuditEntry, Service } from "../types";
import { Card, Empty, PageTitle, Stat, StatusBadge } from "../components/Ui";
import { MiniChart, useSeries } from "../components/Charts";
import { fmtSeconds, fmtTime } from "../services/format";

export default function CommandCenter() {
  const d = useQuery({ queryKey: ["dashboard"], queryFn: () => api<any>("/dashboard") });
  const agents = useQuery({ queryKey: ["agents"], queryFn: () => api<Agent[]>("/agents") });
  const services = useQuery({ queryKey: ["services"], queryFn: () => api<Service[]>("/services") });
  const agent = agents.data?.find((a) => a.connected);
  const series = useSeries(agent?.id);
  const s = d.data;
  if (d.isLoading) return <p className="text-slate-400">Loading…</p>;
  if (d.error) return <Empty title="Could not reach the REFORGE core">{(d.error as Error).message}</Empty>;
  const overall = !s.services.total ? "unknown" : s.services.failed ? "failed" : s.services.degraded || s.active_incidents ? "degraded" : "healthy";
  return (
    <>
      <PageTitle title="Command Center" sub="Live overview of monitored systems and self-healing activity" right={<StatusBadge value={overall} />} />
      {!s.agents_total && !s.services.total ? (
        <Empty title="Nothing is registered yet">Start the demo stack (docker compose up) or register an agent and a service from the Agent Management and System Monitoring pages.</Empty>
      ) : (
        <>
          <div className="grid grid-cols-2 gap-3 md:grid-cols-5">
            <Stat label="Agents" value={`${s.agents_connected}/${s.agents_total}`} hint="connected / registered" />
            <Stat label="Healthy" value={s.services.healthy} />
            <Stat label="Degraded" value={s.services.degraded} />
            <Stat label="Failed" value={s.services.failed} />
            <Stat label="Active incidents" value={s.active_incidents} />
            <Stat label="Recovering now" value={s.recoveries_in_progress} />
            <Stat label="Successful recoveries" value={s.successful_recoveries} />
            <Stat label="Failed recoveries" value={s.failed_recoveries} />
            <Stat label="Avg recovery time" value={fmtSeconds(s.avg_recovery_seconds)} hint="resolved service incidents" />
            <Stat label="Kill switch" value={s.kill_switch ? "ACTIVE" : "off"} />
          </div>
          <div className="mt-5 grid gap-3 md:grid-cols-3">
            <MiniChart title="CPU" unit="%" data={series.data?.cpu_percent ?? []} max={100} />
            <MiniChart title="Memory" unit="%" data={series.data?.memory_percent ?? []} max={100} color="#3b82f6" />
            <MiniChart title="Disk" unit="%" data={series.data?.disk_percent ?? []} max={100} color="#8b5cf6" />
          </div>
          <div className="mt-5 grid gap-4 lg:grid-cols-2">
            <Card title="Services">
              {!services.data?.length ? <Empty title="No services registered" /> : (
                <ul className="divide-y divide-ink-600">
                  {services.data.map((v) => (
                    <li key={v.id} className="flex items-center justify-between py-2 text-sm">
                      <span>{v.name} <span className="text-xs text-slate-500">{v.last_latency_ms} ms</span></span>
                      <StatusBadge value={v.status} />
                    </li>
                  ))}
                </ul>
              )}
            </Card>
            <Card title="Activity feed">
              {!s.activity?.length ? <Empty title="No activity yet" /> : (
                <ul className="max-h-80 space-y-1.5 overflow-auto text-sm">
                  {(s.activity as AuditEntry[]).map((a) => (
                    <li key={a.id} className="flex gap-3">
                      <span className="w-16 shrink-0 font-mono text-xs text-slate-500">{fmtTime(a.ts)}</span>
                      <span><span className="text-accent">{a.action}</span> <span className="text-slate-400">{a.target}</span></span>
                    </li>
                  ))}
                </ul>
              )}
            </Card>
          </div>
        </>
      )}
    </>
  );
}
