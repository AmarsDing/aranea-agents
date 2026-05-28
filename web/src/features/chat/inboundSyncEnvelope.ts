import type { Envelope } from "./envelope";
import { resolveEnvelopeRevision, resolveEnvelopeSource } from "../../realtime/envelope";
import { runStatusFromEnvelope } from "./envelopeRunStatus";
import { SESSION_RUN_STATUS } from "./sessionRunStatus";

export function envelopeSessionRevision(env: Envelope): number {
  return resolveEnvelopeRevision(env);
}

export function envelopeSource(env: Envelope): string {
  return resolveEnvelopeSource(env);
}

/** M55 session_revision sync — incremental hydrate only, not turn complete. */
export function isSessionRevisionSyncEnvelope(env: Envelope): boolean {
  if (env.type !== "run_status") return false;
  return runStatusFromEnvelope(env)?.status === SESSION_RUN_STATUS.SYNC;
}

/** Check skip_hydrate metadata flag — backend sets this on WS-originated sync events
 *  to prevent redundant full hydrate that causes duplicate messages. */
export function shouldSkipHydrate(env: Envelope): boolean {
  const md = env.metadata as Record<string, unknown> | undefined;
  return md?.skip_hydrate === true;
}

export function isTurnCompleteEnvelope(env: Envelope): boolean {
  if (env.type === "runner_completion") return true;
  if (env.type === "error") return true;
  if (env.type !== "run_status") return false;
  const status = runStatusFromEnvelope(env)?.status;
  if (status === SESSION_RUN_STATUS.SYNC) return false;
  return (
    status === SESSION_RUN_STATUS.COMPLETED ||
    status === SESSION_RUN_STATUS.FAILED ||
    status === SESSION_RUN_STATUS.CANCELLED
  );
}
