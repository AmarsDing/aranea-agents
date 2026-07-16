/**
 * activityRunStatus — Run-status extraction from WS payloads.
 *
 * Primary path: v2 `system.run_status` (RunStatusEventPayload).
 * Legacy path: ActivityEvent with `activity.stage = 'run_status'` (still used
 * by non-chat consumers that unwrap activity.bridge).
 */
import type { ActivityEvent } from '../../realtime/activityEvent';
import type { RunStatusValue } from './types';
import type { RunStatusEventPayload } from './v2Types';
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

function normalizeRunStatusFields(meta: Record<string, unknown>, fallbacks?: { status?: string; runId?: string }): RunStatusFromWs | null {
  let status = String(meta.status ?? fallbacks?.status ?? 'idle');
  // TECH-DEBT: legacy escalating→durable mapping, remove after DB migration completes
  if (status === 'escalating') status = 'durable';
  if (status === 'background_job') return null;
  return {
    status: status as RunStatusValue,
    runId: String(meta.run_id ?? fallbacks?.runId ?? ''),
    errorMessage: String(meta.error_message ?? ''),
    awaitKind: meta.await_kind != null ? String(meta.await_kind) : undefined,
    awaitToolKey: meta.await_tool_key != null ? String(meta.await_tool_key) : undefined,
    awaitToolCallId: meta.await_tool_call_id != null ? String(meta.await_tool_call_id) : undefined,
  };
}

/** Extract run-status fields from a v2 system.run_status payload. */
export function runStatusFromV2Payload(payload: RunStatusEventPayload): RunStatusFromWs | null {
  const meta = { ...(payload.Meta ?? {}) };
  return normalizeRunStatusFields(meta, { status: payload.Status, runId: payload.RunID });
}

/** True when a follow-up message was accepted into the active run queue (v2). */
export function messageQueuedFromV2Payload(payload: RunStatusEventPayload): boolean {
  return String(payload.Meta?.hint ?? '') === 'message_queued';
}

/**
 * Extract run-status fields from an ActivityEvent whose
 * `activity.stage === 'run_status'`. Returns null for any other stage.
 */
export function runStatusFromActivityEvent(ev: ActivityEvent): RunStatusFromWs | null {
  if (ev.activity.stage !== 'run_status') return null;
  return normalizeRunStatusFields(ev.activity.meta ?? {});
}

/** True when a follow-up message was accepted into the active run queue. */
export function messageQueuedFromActivityEvent(ev: ActivityEvent): boolean {
  if (ev.activity.stage !== 'run_status') return false;
  return String(ev.activity.meta?.hint ?? '') === 'message_queued';
}
