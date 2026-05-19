import type { Envelope } from "./envelope";
import type { RunStatusValue } from "./api";

export type RunStatusFromWs = {
  status: RunStatusValue;
  runId: string;
  errorMessage: string;
};

export function runStatusFromEnvelope(env: Envelope): RunStatusFromWs | null {
  if (env.type !== "run_status") return null;
  const meta = env.metadata ?? {};
  const status = String(meta.status ?? "idle") as RunStatusValue;
  return {
    status,
    runId: String(meta.run_id ?? ""),
    errorMessage: String(meta.error_message ?? ""),
  };
}
