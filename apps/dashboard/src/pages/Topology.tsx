import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import ReactFlow, { Background, Controls, Edge, Node } from "reactflow";
import "reactflow/dist/style.css";
import { api } from "../services/api";
import { Card, Empty, PageTitle, StatusBadge } from "../components/Ui";

interface TNode { id: string; kind: string; label: string; status: string; detail: Record<string, any> }
const color: Record<string, string> = { healthy: "#34d399", degraded: "#fbbf24", failed: "#f87171", unknown: "#64748b" };

export default function Topology() {
  const q = useQuery({ queryKey: ["topology"], queryFn: () => api<{ nodes: TNode[]; edges: { source: string; target: string; kind: string }[] }>("/topology") });
  const [sel, setSel] = useState<TNode | null>(null);
  const { nodes, edges } = useMemo(() => {
    const svc = (q.data?.nodes ?? []).filter((n) => n.id.startsWith("svc:"));
    const ctr = (q.data?.nodes ?? []).filter((n) => n.id.startsWith("ctr:"));
    const mk = (n: TNode, i: number, y: number): Node => ({
      id: n.id, position: { x: 40 + i * 230, y }, data: { label: `${n.label}\n${n.status}` },
      style: { background: "#141b24", color: "#e2e8f0", border: `2px solid ${color[n.status] ?? color.unknown}`, borderRadius: 8, width: 190, whiteSpace: "pre-line", fontSize: 12 },
    });
    const e: Edge[] = (q.data?.edges ?? []).map((x, i) => ({ id: String(i), source: x.source, target: x.target, label: x.kind, animated: x.kind === "depends_on", style: { stroke: "#64748b" }, labelStyle: { fill: "#94a3b8", fontSize: 10 } }));
    return { nodes: [...svc.map((n, i) => mk(n, i, 40)), ...ctr.map((n, i) => mk(n, i, 220))], edges: e };
  }, [q.data]);
  const byId = (id: string) => q.data?.nodes.find((n) => n.id === id) ?? null;
  return (
    <>
      <PageTitle title="Service Topology" sub="Live dependency map. Node colour reflects current health; click a node for details." />
      {!q.data?.nodes.length ? <Empty title="No services to draw yet" /> : (
        <div className="grid gap-4 lg:grid-cols-[1fr_20rem]">
          <div className="h-[520px] rounded-lg border border-ink-600 bg-ink-900">
            <ReactFlow nodes={nodes} edges={edges} fitView onNodeClick={(_, n) => setSel(byId(n.id))} proOptions={{ hideAttribution: true }}>
              <Background color="#273343" /><Controls showInteractive={false} />
            </ReactFlow>
          </div>
          <Card title={sel ? sel.label : "Details"}>
            {!sel ? <p className="text-sm text-slate-400">Select a node.</p> : (
              <div className="space-y-2 text-sm">
                <StatusBadge value={sel.status} />
                <p className="text-slate-400">Type: {sel.kind}</p>
                {Object.entries(sel.detail).map(([k, v]) => <p key={k}><span className="text-slate-500">{k.replace(/_/g, " ")}:</span> {String(v)}</p>)}
              </div>
            )}
          </Card>
        </div>
      )}
    </>
  );
}
