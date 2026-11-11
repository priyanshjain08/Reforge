import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../services/api";
import { Experiment, Service } from "../types";
import { Btn, Card, Empty, Field, PageTitle, StatusBadge, Table, inputCls } from "../components/Ui";
import { fmtSeconds, seconds } from "../services/format";
import { useAct } from "../hooks/useAct";

export default function ChaosLab() {
  const act = useAct();
  const services = useQuery({ queryKey: ["services"], queryFn: () => api<Service[]>("/services") });
  const chaos = useQuery({ queryKey: ["chaos"], queryFn: () => api<{ types: Record<string, string>; experiments: Experiment[]; manual_baseline_s: number; manual_baseline_note: string }>("/chaos") });
  const targets = services.data?.filter((s) => s.chaos_target) ?? [];
  const [svc, setSvc] = useState("");
  const [type, setType] = useState("crash");
  const [dur, setDur] = useState(60);
  const [inten, setInten] = useState(50);
  const exps = chaos.data?.experiments ?? [];
  const done = exps.filter((e) => e.recovered_at);
  const avgAuto = done.length ? done.reduce((a, e) => a + (seconds(e.injected_at, e.recovered_at) ?? 0), 0) / done.length : null;
  const baseline = chaos.data?.manual_baseline_s ?? 300;
  return (
    <>
      <PageTitle title="Chaos Lab" sub="Safely break the demo services and watch REFORGE detect, diagnose, recover and verify." />
      <Card title="Safety">
        <p className="text-sm text-slate-300">Failures can only be injected into registered demo services running in the isolated Docker network. Nothing here touches your computer, files or other software.</p>
      </Card>
      <Card title="Inject a failure" className="mt-4">
        {!targets.length ? <Empty title="No chaos targets">Start the demo stack with docker compose to get three demo services.</Empty> : (
          <div className="grid gap-3 md:grid-cols-5">
            <Field label="Service"><select className={inputCls} value={svc || targets[0].name} onChange={(e) => setSvc(e.target.value)}>{targets.map((t) => <option key={t.id}>{t.name}</option>)}</select></Field>
            <Field label="Failure type"><select className={inputCls} value={type} onChange={(e) => setType(e.target.value)}>{Object.entries(chaos.data?.types ?? {}).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</select></Field>
            <Field label={`Duration: ${dur}s`}><input type="range" min={10} max={300} value={dur} onChange={(e) => setDur(+e.target.value)} className="w-full" /></Field>
            <Field label={`Intensity: ${inten}%`}><input type="range" min={1} max={100} value={inten} onChange={(e) => setInten(+e.target.value)} className="w-full" /></Field>
            <div className="flex items-end"><Btn kind="danger" onClick={() => act("/chaos", "POST", { service: svc || targets[0].name, type, duration_s: dur, intensity: inten })}>Inject failure</Btn></div>
          </div>
        )}
      </Card>
      <Card title="Automated vs manual recovery" className="mt-4">
        <div className="grid gap-3 text-sm md:grid-cols-3">
          <div><div className="text-xs uppercase text-slate-400">REFORGE (measured, avg)</div><div className="text-2xl font-semibold text-ok">{fmtSeconds(avgAuto)}</div><div className="text-xs text-slate-500">{done.length} completed experiments</div></div>
          <div><div className="text-xs uppercase text-slate-400">Manual baseline (ESTIMATE)</div><div className="text-2xl font-semibold text-slate-300">{fmtSeconds(baseline)}</div><div className="text-xs text-slate-500">{chaos.data?.manual_baseline_note}</div></div>
          <div><div className="text-xs uppercase text-slate-400">Difference vs estimate</div><div className="text-2xl font-semibold">{avgAuto === null ? "—" : fmtSeconds(baseline - avgAuto)}</div></div>
        </div>
      </Card>
      <Card title="Experiment results" className="mt-4">
        {!exps.length ? <Empty title="No experiments yet" /> : (
          <Table head={["Service / type", "Status", "To detect", "To diagnose", "To start recovery", "To recover", "Verified", ""]}>
            {exps.map((e) => (
              <tr key={e.id}>
                <td className="px-3 py-2">{e.service}<div className="text-xs text-slate-500">{e.type}</div></td>
                <td className="px-3 py-2"><StatusBadge value={e.status} /></td>
                <td className="px-3 py-2">{fmtSeconds(seconds(e.injected_at, e.detected_at))}</td>
                <td className="px-3 py-2">{fmtSeconds(seconds(e.injected_at, e.diagnosed_at))}</td>
                <td className="px-3 py-2">{fmtSeconds(seconds(e.injected_at, e.recovery_started_at))}</td>
                <td className="px-3 py-2">{fmtSeconds(seconds(e.injected_at, e.recovered_at))}</td>
                <td className="px-3 py-2">{e.verified === undefined ? "—" : e.verified ? "yes" : "no"}</td>
                <td className="px-3 py-2">{e.status === "running" && <Btn onClick={() => act(`/chaos/${e.id}/stop`)}>Stop &amp; clear</Btn>}</td>
              </tr>
            ))}
          </Table>
        )}
      </Card>
    </>
  );
}
