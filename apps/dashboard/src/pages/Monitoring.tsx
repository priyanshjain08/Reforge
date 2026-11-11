import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../services/api";
import { Agent, Service } from "../types";
import { Btn, Card, Empty, Field, PageTitle, StatusBadge, Table, inputCls } from "../components/Ui";
import { MiniChart, ServiceLatency, useSeries } from "../components/Charts";
import { fmtBytes } from "../services/format";
import { useAct } from "../hooks/useAct";

export default function Monitoring() {
  const act = useAct();
  const agents = useQuery({ queryKey: ["agents"], queryFn: () => api<Agent[]>("/agents") });
  const services = useQuery({ queryKey: ["services"], queryFn: () => api<Service[]>("/services") });
  const agent = agents.data?.find((a) => a.connected) ?? agents.data?.[0];
  const series = useSeries(agent?.id);
  const m = series.data ?? {};
  const [f, setF] = useState({ name: "", health_url: "", expected_status: 200, timeout_ms: 2000, interval_s: 10, environment: "production", container: "" });

  return (
    <>
      <PageTitle title="System Monitoring" sub={agent ? `Host metrics from ${agent.hostname || agent.name} (${agent.os || "awaiting first report"})` : "No agent registered"} />
      {!agent ? <Empty title="No agent yet">Host metrics appear once a monitoring agent connects.</Empty> : (
        <>
          <div className="grid gap-3 md:grid-cols-3">
            <MiniChart title="CPU utilisation" unit="%" data={m.cpu_percent ?? []} max={100} />
            <MiniChart title="Memory utilisation" unit="%" data={m.memory_percent ?? []} max={100} color="#3b82f6" />
            <MiniChart title="Disk usage" unit="%" data={m.disk_percent ?? []} max={100} color="#8b5cf6" />
            <MiniChart title="Network receive (B/s)" data={m.network_rx_bps ?? []} color="#34d399" />
            <MiniChart title="Network transmit (B/s)" data={m.network_tx_bps ?? []} color="#fbbf24" />
            <MiniChart title="Process count" data={m.process_count ?? []} color="#22d3ee" />
            <MiniChart title="Disk read (B/s)" data={m.disk_read_bps ?? []} color="#3b82f6" />
            <MiniChart title="Disk write (B/s)" data={m.disk_write_bps ?? []} color="#8b5cf6" />
            {services.data?.map((v) => <ServiceLatency key={v.id} id={v.id} name={v.name} />)}
          </div>
          {agent.unsupported_metrics?.length > 0 && (
            <p className="mt-3 text-sm text-warn">Not supported on this platform (never fabricated): {agent.unsupported_metrics.join(", ")}</p>
          )}
          <p className="mt-1 text-xs text-slate-500">Net now: ↓ {fmtBytes(agent.metrics?.network_rx_bps)} ↑ {fmtBytes(agent.metrics?.network_tx_bps)}</p>
        </>
      )}

      <div className="mt-5 grid gap-4 lg:grid-cols-2">
        <Card title="Monitored services">
          {!services.data?.length ? <Empty title="No services registered" /> : (
            <Table head={["Service", "Status", "Latency", "Failures", "Container"]}>
              {services.data.map((v) => (
                <tr key={v.id}>
                  <td className="px-3 py-2">{v.name}<div className="text-xs text-slate-500">{v.environment}</div></td>
                  <td className="px-3 py-2"><StatusBadge value={v.status} /></td>
                  <td className="px-3 py-2">{v.last_latency_ms} ms</td>
                  <td className="px-3 py-2">{v.consecutive_failures} <span className="text-slate-500">/ {v.total_failures} total</span></td>
                  <td className="px-3 py-2">{v.container ? <StatusBadge value={v.container_state || "unknown"} /> : "—"}</td>
                </tr>
              ))}
            </Table>
          )}
        </Card>
        <Card title="Containers (via agent)">
          {!agent?.containers?.length ? <Empty title="No container data">The agent needs Docker socket access and container targets.</Empty> : (
            <Table head={["Container", "State", "Health", "Restarts"]}>
              {agent.containers.map((c) => (
                <tr key={c.name}><td className="px-3 py-2">{c.name}</td><td className="px-3 py-2"><StatusBadge value={c.state} /></td>
                  <td className="px-3 py-2">{c.health || "—"}</td><td className="px-3 py-2">{c.restarts}</td></tr>
              ))}
            </Table>
          )}
        </Card>
      </div>

      <Card title="Register an application" className="mt-5">
        <form className="grid gap-3 md:grid-cols-4" onSubmit={async (e) => {
          e.preventDefault();
          await act("/services", "POST", { ...f, container: f.container || undefined, base_url: f.health_url });
        }}>
          <Field label="Name"><input className={inputCls} required value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} /></Field>
          <Field label="Health-check URL"><input className={inputCls} required placeholder="http://host:port/health" value={f.health_url} onChange={(e) => setF({ ...f, health_url: e.target.value })} /></Field>
          <Field label="Expected status"><input className={inputCls} type="number" value={f.expected_status} onChange={(e) => setF({ ...f, expected_status: +e.target.value })} /></Field>
          <Field label="Timeout (ms)"><input className={inputCls} type="number" value={f.timeout_ms} onChange={(e) => setF({ ...f, timeout_ms: +e.target.value })} /></Field>
          <Field label="Interval (s)"><input className={inputCls} type="number" value={f.interval_s} onChange={(e) => setF({ ...f, interval_s: +e.target.value })} /></Field>
          <Field label="Environment"><input className={inputCls} value={f.environment} onChange={(e) => setF({ ...f, environment: e.target.value })} /></Field>
          <Field label="Container name (optional)"><input className={inputCls} value={f.container} onChange={(e) => setF({ ...f, container: e.target.value })} /></Field>
          <div className="flex items-end"><Btn type="submit" kind="primary">Register</Btn></div>
        </form>
      </Card>
    </>
  );
}
