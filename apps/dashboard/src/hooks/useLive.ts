import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useUi } from "../stores/auth";

// Subscribes to the core's Server-Sent Events stream and refreshes data when something changes.
export function useLive() {
  const qc = useQueryClient();
  const token = useUi((s) => s.token);
  useEffect(() => {
    if (!token) return;
    const es = new EventSource(`/api/v1/events?token=${encodeURIComponent(token)}`);
    let timer: number | undefined;
    es.onmessage = () => {
      window.clearTimeout(timer);
      timer = window.setTimeout(() => qc.invalidateQueries(), 300);
    };
    return () => {
      window.clearTimeout(timer);
      es.close();
    };
  }, [token, qc]);
}
