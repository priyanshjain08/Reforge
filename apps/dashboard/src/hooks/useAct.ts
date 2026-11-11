import { useQueryClient } from "@tanstack/react-query";
import { api } from "../services/api";

// Runs a mutating API call, refreshes data, and surfaces server validation errors to the user.
export function useAct() {
  const qc = useQueryClient();
  return async function act<T = any>(path: string, method = "POST", body?: unknown): Promise<T | undefined> {
    try {
      const r = await api<T>(path, { method, body });
      await qc.invalidateQueries();
      return r;
    } catch (e: any) {
      window.alert(e?.message ?? "Request failed");
      return undefined;
    }
  };
}
