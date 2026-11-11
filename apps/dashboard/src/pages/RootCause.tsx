import { useQueries, useQuery } from "@tanstack/react-query";
import { api } from "../services/api";
import { Diagnosis, Incident } from "../types";
import { Card, Empty, PageTitle, StatusBadge } from "../components/Ui";

export function DiagnosisView({ d }: { d: Diagnosis }) {
  return (
    <div className="mb-3 rounded border border-ink-600 p-3 text-sm">
      <div className="flex items-center justify-between">
        <b>{d.probable_cause}</b>
        <span className="flex items-center gap-2"><StatusBadge value={d.status} /><span className="text-xs text-slate-400">confidence {(d.confidence * 100).toFixed(0)}%</span></span>
      </div>
      <p className="mt-1 text-slate-400">{d.explanation}</p>
      <div className="mt-2 text-xs uppercase text-slate-500">Evidence</div>
      <ul className="list-disc pl-5 text-slate-300">{d.evidence.map((e, n) => <li key={n}>{e}</li>)}</ul>
      {d.alternatives?.length > 0 && <p className="mt-2 text-slate-400">Alternative explanations: {d.alternatives.join("; ")}</p>}
      <p className="mt-2">Recommended: <code className="rounded bg-ink-900 px-1.5 py-0.5 text-accent">{d.recommended_action}</code> <span className="text-xs text-slate-500">(source: {d.source}; a recommendation, not an executed action)</span></p>
      {d.status === "suspected" && <p className="mt-1 text-xs text-warn">Suspected, not confirmed: based on indirect evidence.</p>}
    </div>
  );
}

export default function RootCause() {
  const list = useQuery({ queryKey: ["incidents", "rca"], queryFn: () => api<{ items: Incident[] }>("/incidents?page_size=15") });
  const details = useQueries({ queries: (list.data?.items ?? []).map((i) => ({ queryKey: ["incident", i.id], queryFn: () => api<{ incident: Incident }>(`/incidents/${i.id}`) })) });
  const rows = details.map((d) => d.data?.incident).filter((i): i is Incident => !!i && i.diagnoses?.length > 0);
  return (
    <>
      <PageTitle title="Root Cause Analysis" sub="Rule-based diagnoses with evidence. Suspected causes are never presented as confirmed." />
      {!rows.length ? <Empty title="No diagnoses yet">They appear after an incident is detected and analysed.</Empty> : rows.map((i) => (
        <Card key={i.id} title={`${i.title}  ·  ${i.id}`} className="mb-4">{i.diagnoses.map((d, n) => <DiagnosisView key={n} d={d} />)}</Card>
      ))}
    </>
  );
}
