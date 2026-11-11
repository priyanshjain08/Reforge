import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { useQuery } from "@tanstack/react-query";
import { api } from "../services/api";

export interface Pt { ts: number; value: number; ok?: boolean }

export function MiniChart({ title, data, unit = "", color = "#22d3ee", max }: { title: string; data: Pt[]; unit?: string; color?: string; max?: number }) {
  const last = data.length ? data[data.length - 1].value : undefined;
  return (
    <div className="rounded-lg border border-ink-600 bg-ink-800 p-3">
      <div className="flex items-baseline justify-between">
        <span className="text-xs uppercase tracking-wide text-slate-400">{title}</span>
        <span className="text-sm font-semibold text-slate-100">{last === undefined ? "no data" : `${last.toFixed(1)}${unit}`}</span>
      </div>
      <div className="h-28">
        {data.length < 2 ? (
          <div className="flex h-full items-center justify-center text-xs text-slate-500">waiting for samples</div>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={data}>
              <CartesianGrid stroke="#273343" strokeDasharray="3 3" />
              <XAxis dataKey="ts" hide />
              <YAxis domain={[0, max ?? "auto"]} width={36} tick={{ fill: "#64748b", fontSize: 10 }} />
              <Tooltip contentStyle={{ background: "#141b24", border: "1px solid #273343" }} labelFormatter={(v) => new Date(v as number).toLocaleTimeString()} />
              <Area type="monotone" dataKey="value" stroke={color} fill={color} fillOpacity={0.15} isAnimationActive={false} />
            </AreaChart>
          </ResponsiveContainer>
        )}
      </div>
    </div>
  );
}

export function useSeries(agentId?: string) {
  return useQuery({ queryKey: ["metrics", agentId], enabled: !!agentId, queryFn: () => api<Record<string, Pt[]>>(`/metrics?agent_id=${agentId}`) });
}

export function ServiceLatency({ id, name }: { id: string; name: string }) {
  const q = useQuery({ queryKey: ["history", id], queryFn: () => api<Pt[]>(`/services/${id}/history`) });
  return <MiniChart title={`${name} response time`} data={q.data ?? []} unit=" ms" color="#8b5cf6" />;
}
