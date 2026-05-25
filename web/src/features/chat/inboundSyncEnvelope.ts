import type { Envelope } from "./envelope";
import { runStatusFromEnvelope } from "./envelopeRunStatus";
import { SESSION_RUN_STATUS } from "./sessionRunStatus";

export function envelopeSessionRevision(env: Envelope): number {
  if (typeof env.session_revision === "number" && env.session_revision > 0) {
    return env.session_revision;
  }
  const md = env.metadata as Record<string, unknown> | undefined;
  const fromMeta = md?.session_revision;
  if (typeof fromMeta === "number" && fromMeta > 0) return fromMeta;
  return 0;
}

export function envelopeSource(env: Envelope): string {
  const direct = (env.source ?? "").trim();
  if (direct) return direct;
  const md = env.metadata as Record<string, unknown> | undefined;
  return typeof md?.source === "string" ? md.source.trim() : "";
}

/** M55 session_revision sync — incremental hydrate only, not turn complete. */
export function isSessionRevisionSyncEnvelope(env: Envelope): boolean {
  if (env.type !== "run_status") return false;
  return runStatusFromEnvelope(env)?.status === SESSION_RUN_STATUS.SYNC;
}

export function isTurnCompleteEnvelope(env: Envelope): boolean {
  if (env.type === "runner_completion") return true;
  if (env.type !== "run_status") return false;
  const status = runStatusFromEnvelope(env)?.status;
  if (status === SESSION_RUN_STATUS.SYNC) return false;
  return (
    status === SESSION_RUN_STATUS.COMPLETED ||
    status === SESSION_RUN_STATUS.FAILED ||
    status === SESSION_RUN_STATUS.CANCELLED
  );
}
