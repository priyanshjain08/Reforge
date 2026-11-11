export function seconds(a?: string, b?: string): number | null {
  if (!a || !b) return null;
  return Math.max(0, (new Date(b).getTime() - new Date(a).getTime()) / 1000);
}
export function fmtSeconds(v: number | null | undefined): string {
  if (v === null || v === undefined) return "—";
  return v < 60 ? `${v.toFixed(1)}s` : `${Math.floor(v / 60)}m ${Math.round(v % 60)}s`;
}
export function fmtTime(ts?: string): string {
  return ts ? new Date(ts).toLocaleTimeString() : "—";
}
export function fmtBytes(v?: number): string {
  if (v === undefined) return "—";
  const u = ["B/s", "KB/s", "MB/s", "GB/s"];
  let i = 0;
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(1)} ${u[i]}`;
}
// Mirrors server-side policy validation for instant feedback; the server remains authoritative.
export function validatePolicyInput(p: { name: string; retry_limit: number; cooldown_s: number; verify_timeout_s: number; min_failures: number; priority: number }): string | null {
  if (!p.name.trim()) return "Name is required";
  if (p.retry_limit < 1 || p.retry_limit > 10) return "Retry limit must be 1-10";
  if (p.cooldown_s < 0 || p.cooldown_s > 3600) return "Cooldown must be 0-3600 seconds";
  if (p.verify_timeout_s < 5 || p.verify_timeout_s > 300) return "Verification timeout must be 5-300 seconds";
  if (p.min_failures < 1 || p.min_failures > 20) return "Minimum failures must be 1-20";
  if (p.priority < 1 || p.priority > 1000) return "Priority must be 1-1000";
  return null;
}
