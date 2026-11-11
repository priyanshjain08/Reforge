import { useQuery } from "@tanstack/react-query";
import { api } from "../services/api";
import { Operation } from "../types";
import { Btn, Card, Empty, PageTitle, StatusBadge, Table } from "../components/Ui";
import { fmtSeconds, fmtTime, seconds } from "../services/format";
import { useAct } from "../hooks/useAct";

export default function Recovery() {
  const act = useAct();
  const ops = useQuery({ queryKey: ["recovery"], queryFn: () => api<Operation[]>("/recovery") });
  const ks = useQuery({ queryKey: ["killswitch"], queryFn: () => api<{ enabled: boolean }>("/killswitch") });
  const on = !!ks.data?.enabled;
  return (
    <>
      <PageTitle title="Recovery Operations" sub="Every action is allowlisted, authorised, verified and audited" right={
        <Btn kind={on ? "default" : "danger"} onClick={() => { if (on || window.confirm("Disable ALL automated recovery immediately?")) act("/killswitch", "POST", { enabled: !on }); }}>
          {on ? "Release kill switch" : "EMERGENCY STOP"}
        </Btn>
      } />
      {!ops.data?.length ? <Empty title="No recovery operations yet">They appear when a policy proposes or runs an action.</Empty> : (
        <Card>
          <Table head={["Started", "Action", "Target", "Policy / mode", "State", "Duration", "Details"]}>
            {ops.data.map((o) => (
              <tr key={o.id} className="align-top">
                <td className="px-3 py-2 text-xs">{fmtTime(o.started_at)}</td>
                <td className="px-3 py-2">{o.action_id}</td>
                <td className="px-3 py-2">{o.target}</td>
                <td className="px-3 py-2 text-xs">{o.policy_name}<div className="text-slate-500">{o.mode}</div></td>
                <td className="px-3 py-2"><StatusBadge value={o.state} /></td>
                <td className="px-3 py-2">{fmtSeconds(seconds(o.started_at, o.finished_at))}</td>
                <td className="px-3 py-2 text-xs text-slate-400">
                  {o.previous_state && <div>before: {o.previous_state}</div>}
                  {o.resulting_state && <div>after: {o.resulting_state}</div>}
                  {o.error && <div className="text-bad">{o.error}</div>}
                  {o.verified !== undefined && <div>verified: {String(o.verified)}</div>}
                  {o.state === "awaiting_approval" && (
                    <div className="mt-1 flex gap-2">
                      <Btn kind="primary" onClick={() => act(`/recovery/${o.id}/approve`)}>Approve</Btn>
                      <Btn onClick={() => act(`/recovery/${o.id}/cancel`)}>Cancel</Btn>
                    </div>
                  )}
                </td>
              </tr>
            ))}
          </Table>
        </Card>
      )}
    </>
  );
}
