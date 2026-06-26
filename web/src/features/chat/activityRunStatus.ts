/**
 * activityRunStatus — Run-status extraction from ActivityEvent payloads.
 *
 * The backend sends run_status events as ActivityEvent with
 * `activity.stage = 'run_status'` on the WS chat channel. The fields
 * previously carried on `envelope.metadata` (status, run_id, error_message,
 * await_kind, await_tool_key, await_tool_call_id, hint) now live on
 * `activity.meta`.
 */
import type { ActivityEvent } from '../../realtime/activityEvent';
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

/**
 * Extract run-status fields from an ActivityEvent whose
 * `activity.stage === 'run_status'`. Returns null for any other stage.
 *
 * Field mapping (envelope → activity):
 *   env.metadata.status             → ev.activity.meta.status
 *   env.metadata.run_id             → ev.activity.meta.run_id
 *   env.metadata.error_message      → ev.activity.meta.error_message
 *   env.metadata.await_kind         → ev.activity.meta.await_kind
 *   env.metadata.await_tool_key     → ev.activity.meta.await_tool_key
 *   env.metadata.await_tool_call_id → ev.activity.meta.await_tool_call_id
 */
export function runStatusFromActivityEvent(ev: ActivityEvent): RunStatusFromWs | null {
  if (ev.activity.stage !== 'run_status') return null;
  const meta = ev.activity.meta ?? {};
  let status = String(meta.status ?? 'idle');
  // TECH-DEBT: legacy escalating→durable mapping, remove after DB migration completes
  if (status === 'escalating') status = 'durable';
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
export function messageQueuedFromActivityEvent(ev: ActivityEvent): boolean {
  if (ev.activity.stage !== 'run_status') return false;
  return String(ev.activity.meta?.hint ?? '') === 'message_queued';
}
