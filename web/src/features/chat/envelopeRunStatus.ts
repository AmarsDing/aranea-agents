import type { Envelope } from './envelope';
import type { RunStatusValue } from './types';
import { AWAIT_KIND_REPLY, AWAIT_KIND_TOOL_CONFIRM } from './awaitConstants';

export type RunStatusFromWs = {
  status: RunStatusValue;
  runId: string;
  errorMessage: string;
  awaitKind?: string;
  awaitToolKey?: string;
  awaitToolCallId?: string;
};

export { AWAIT_KIND_REPLY, AWAIT_KIND_TOOL_CONFIRM };

export function runStatusFromEnvelope(env: Envelope): RunStatusFromWs | null {
  if (env.type !== 'run_status') return null;
  const meta = env.metadata ?? {};
  const status = String(meta.status ?? 'idle');
  if (status === 'background_job') return null;
  return {
    status: status as RunStatusValue,
    runId: String(meta.run_id ?? ''),
    errorMessage: String(meta.error_message ?? ''),
    awaitKind: meta.await_kind != null ? String(meta.await_kind) : undefined,
    awaitToolKey: meta.await_tool_key != null ? String(meta.await_tool_key) : undefined,
    awaitToolCallId: meta.await_tool_call_id != null ? String(meta.await_tool_call_id) : undefined,
  };
}

/** True when a follow-up message was accepted into the active run queue. */
export function messageQueuedFromEnvelope(env: Envelope): boolean {
  if (env.type !== 'run_status') return false;
  return String(env.metadata?.hint ?? '') === 'message_queued';
}
