import { useUi } from "../stores/auth";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export async function api<T = any>(path: string, opts: { method?: string; body?: unknown } = {}): Promise<T> {
  const token = useUi.getState().token;
  const res = await fetch("/api/v1" + path, {
    method: opts.method ?? "GET",
    headers: { Authorization: `Bearer ${token}`, ...(opts.body ? { "Content-Type": "application/json" } : {}) },
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  });
  if (res.status === 401) useUi.getState().setToken("");
  let json: any = null;
  try {
    json = await res.json();
  } catch {
    /* non-JSON body */
  }
  if (!res.ok) throw new ApiError(res.status, json?.error?.message ?? `Request failed (${res.status})`);
  return json.data as T;
}

export async function download(path: string, filename: string) {
  const token = useUi.getState().token;
  const res = await fetch("/api/v1" + path, { headers: { Authorization: `Bearer ${token}` } });
  if (!res.ok) throw new ApiError(res.status, "Download failed");
  const url = URL.createObjectURL(await res.blob());
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}
