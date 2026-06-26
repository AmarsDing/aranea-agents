/**
 * activityToolCall — Tool-call event extraction from ActivityEvent payloads,
 * plus pure helpers for merging / upserting / finalising tool activity rows
 * in the message store.
 *
 * The backend sends tool-call events as ActivityEvent with
 * `activity.kind = 'action'` on the WS chat channel. The fields previously
 * carried on `envelope.tool_call` (name, arguments_json, result_json, etc.)
 * now live as direct fields on `activity` (tool_name, tool_arguments,
 * tool_result, tool_call_id, tool_category, tool_duration_ms,
 * tool_error_code) or in `activity.meta`.
 */
import type { ActivityEvent, ActivityStatus } from '../../realtime/activityEvent';
import type { ActivityKind, ToolUseEvent } from './types';
import type { Message } from './types';
import { MESSAGE_STATUS } from '../../domain/types';
import { canonicalToolStatus } from './lib/statusMap';
import { activityMessageId } from './lib/activityMessageId';
import { isToolUseEvent } from './lib/isToolUseEvent';
import { toolEventToMessage } from './toolEventMarkdown';

/**
 * Maximum byte length for an individual `arguments_json` / `result_json` payload
 * we will decode. Anything larger is truncated to a preview that mirrors the
 * backend `biz.redactActivityJSON` (512 bytes) so the chat UI and the
 * Observatory chrome stay consistent and a single LLM payload can't blow up
 * the browser (SEC-04-style payload-bomb protection).
 */
const ACTIVITY_JSON_PREVIEW_LIMIT = 512;

function truncatePreview(raw: string): string {
  const trimmed =
    raw.length > ACTIVITY_JSON_PREVIEW_LIMIT ? raw.slice(0, ACTIVITY_JSON_PREVIEW_LIMIT) + '…[truncated]' : raw;
  return trimmed;
}

function parseJSONRecord(raw: string | undefined): Record<string, unknown> {
  if (!raw?.trim()) return {};
  const preview = truncatePreview(raw);
  try {
    const v = JSON.parse(preview) as unknown;
    if (v !== null && typeof v === 'object' && !Array.isArray(v)) {
      return v as Record<string, unknown>;
    }
    // Preserve arrays and primitive JSON values verbatim under a known key so
    // downstream consumers (Vue props, JSON.stringify in buildToolSection) keep
    // their structure. Without this, LLM-emitted array arguments were collapsed
    // to { value: [...] } and lost their shape in the UI.
    if (Array.isArray(v)) return { __array: v };
    if (v === null) return { __null: true };
    return { __value: v };
  } catch {
    return { __raw: preview };
  }
}

// Re-export the canonical implementation from ./lib/activityMessageId.
export { activityMessageId };

// Kept as a thin wrapper so existing call sites (and test mocks) that import
// `normalizeToolStatus` continue to work. The single source of truth now lives
// in ./lib/statusMap.
function normalizeToolStatus(status: string): ToolUseEvent['status'] {
  return canonicalToolStatus(status);
}

/** Strip the wrapper keys produced by {@link parseJSONRecord} so callers see the
 *  original JSON value (array, null, primitive) instead of the synthetic object. */
function unwrapParsed(record: Record<string, unknown>): unknown {
  const keys = Object.keys(record);
  if (keys.length === 1) {
    if (keys[0] === '__array') return record.__array;
    if (keys[0] === '__value') return record.__value;
    if (keys[0] === '__null') return null;
  }
  return record;
}

/**
 * Map an {@link ActivityStatus} (from the ActivityEvent payload) to the
 * wire-level status string that {@link canonicalToolStatus} understands.
 *
 * ActivityStatus values are semantically richer than the legacy wire statuses
 * (calling/running/success/error/blocked/cancelled), so we collapse the
 * finer-grained states into the closest wire equivalent.
 */
const ACTIVITY_STATUS_TO_WIRE: Record<ActivityStatus, string> = {
  pending: 'running',
  running: 'running',
  tool_running: 'running',
  tool_blocked: 'blocked',
  completed: 'success',
  failed: 'failed',
  partial_failure: 'failed',
  cancelled: 'cancelled',
  interrupted: 'interrupted',
};

function wireStatusFromActivity(status: ActivityStatus | undefined, phase: 'before' | 'after'): string {
  if (status && ACTIVITY_STATUS_TO_WIRE[status]) return ACTIVITY_STATUS_TO_WIRE[status];
  return phase === 'before' ? 'running' : 'success';
}

/**
 * Derive the tool-call phase from an {@link ActivityEventType}.
 *
 *   created              → before (tool invocation started)
 *   streaming / updated  → before (arguments / result streaming)
 *   completed / failed / cancelled → after (terminal state)
 */
function phaseFromActivityEvent(event: ActivityEvent['event']): 'before' | 'after' {
  switch (event) {
    case 'completed':
    case 'failed':
    case 'cancelled':
      return 'after';
    default:
      return 'before';
  }
}

// ── ActivityEvent-based functions (new) ────────────────────────────────

/**
 * Build a {@link ToolUseEvent} from an ActivityEvent whose
 * `activity.kind === 'action'`. Returns null if the event is not a tool
 * action or is missing identifying fields.
 *
 * Field mapping (envelope.tool_call → activity):
 *   tc.name             → activity.tool_name
 *   tc.id               → activity.tool_call_id
 *   tc.arguments_json   → activity.tool_arguments
 *   tc.result_json      → activity.tool_result
 *   tc.status           → activity.status (mapped via ACTIVITY_STATUS_TO_WIRE)
 *   tc.duration_ms      → activity.tool_duration_ms
 *   tc.error_code       → activity.tool_error_code
 *   tc.activity_kind    → activity.meta.activity_kind (fallback: 'tool')
 *   tc.display_label    → activity.label (fallback: activity.meta.display_label)
 *   tc.icon_key         → activity.meta.icon_key
 *   tc.summary          → activity.meta.summary
 *   tc.started_at       → activity.meta.started_at
 *   tc.finished_at      → activity.timestamp (fallback: activity.meta.finished_at)
 *   tc.is_long_running  → activity.meta.is_long_running
 *   tc.agent_id         → activity.meta.agent_id
 *   tc.agent_key        → activity.agent_key
 *   tc.agent_name       → activity.agent_name
 *   tc.run_id           → activity.meta.run_id
 *   tc.trace_id         → activity.meta.trace_id
 */
export function activityToToolEvent(ev: ActivityEvent, phase: 'before' | 'after'): ToolUseEvent | null {
  const act = ev.activity;
  if (act.kind !== 'action') return null;
  const toolName = act.tool_name || '';
  const toolCallId = act.tool_call_id || '';
  if (!toolName && !toolCallId) return null;

  const args = unwrapParsed(parseJSONRecord(act.tool_arguments)) as ToolUseEvent['arguments'];
  const resultRecord = parseJSONRecord(act.tool_result);
  const result = unwrapParsed(resultRecord) as Record<string, unknown> | null | undefined;

  const meta = act.meta ?? {};
  // activity_kind: prefer meta.activity_kind (legacy compatibility), else
  // derive from tool_category, else default to 'tool'.
  const activityKindRaw = typeof meta.activity_kind === 'string' ? meta.activity_kind : '';
  const kind = (activityKindRaw || 'tool') as ActivityKind;

  const wireStatus = wireStatusFromActivity(act.status, phase);
  const status = normalizeToolStatus(wireStatus);
  const isFailure = status === 'failed';

  const resultErrorStr =
    typeof result === 'object' && result !== null && typeof (result as { error?: unknown }).error === 'string'
      ? (result as { error: string }).error
      : undefined;
  const resultI18nKey =
    typeof result === 'object' && result !== null && typeof (result as { i18n_key?: unknown }).i18n_key === 'string'
      ? (result as { i18n_key: string }).i18n_key
      : undefined;
  const errorMessage = resultErrorStr ?? (isFailure && act.tool_error_code ? act.tool_error_code : undefined);

  const displayLabel =
    typeof act.label === 'string' && act.label.length > 0
      ? act.label
      : typeof meta.display_label === 'string'
        ? meta.display_label
        : toolName;

  const startedAt = typeof meta.started_at === 'string' ? meta.started_at : '';
  const finishedAt = act.timestamp || (typeof meta.finished_at === 'string' ? meta.finished_at : '');

  return {
    id: toolCallId || act.id,
    phase,
    status,
    agent_id: typeof meta.agent_id === 'string' ? meta.agent_id : '',
    agent_key: act.agent_key || '',
    agent_name: act.agent_name || act.agent_key || 'Agent',
    tool_name: toolName || 'tool',
    tool_label: displayLabel,
    arguments: args,
    result:
      phase === 'after' || (result && typeof result === 'object' && Object.keys(result).length > 0)
        ? (result as Record<string, unknown>)
        : undefined,
    error: errorMessage,
    occurred_at: finishedAt || startedAt || act.timestamp || new Date().toISOString(),
    duration_ms: typeof act.tool_duration_ms === 'number' ? act.tool_duration_ms : undefined,
    is_long_running: typeof meta.is_long_running === 'boolean' ? meta.is_long_running : undefined,
    activity_kind: kind,
    display_label: displayLabel || undefined,
    icon_key: typeof meta.icon_key === 'string' ? meta.icon_key : undefined,
    summary: typeof meta.summary === 'string' ? meta.summary : undefined,
    started_at: startedAt || undefined,
    finished_at: finishedAt || undefined,
    error_code: act.tool_error_code || undefined,
    i18n_key: resultI18nKey,
    run_id: typeof meta.run_id === 'string' ? meta.run_id : undefined,
    trace_id: typeof meta.trace_id === 'string' ? meta.trace_id : undefined,
  };
}

/** Upsert a tool activity row derived from an ActivityEvent into the message list. */
export function upsertToolMessageFromActivity(
  messages: Message[],
  sessionId: string,
  ev: ActivityEvent,
): Message[] {
  const phase = phaseFromActivityEvent(ev.event);
  const event = activityToToolEvent(ev, phase);
  if (!event) return messages;
  const row = toolEventToMessage(sessionId, event);
  const messageId = activityMessageId(event);
  row.id = messageId;
  const idx = messages.findIndex((m) => m.id === messageId || (event.id && m.id === `act-${event.id}`));
  if (idx >= 0) {
    const priorMeta = toolEventFromMessage(messages[idx]);
    const merged = priorMeta ? mergeToolEvents(priorMeta, event) : event;
    const nextRow = toolEventToMessage(sessionId, merged);
    nextRow.id = messageId;
    const next = [...messages];
    next[idx] = { ...messages[idx], ...nextRow, id: messageId };
    return next;
  }
  return [...messages, row];
}

export function mergeToolEvents(existing: ToolUseEvent, incoming: ToolUseEvent): ToolUseEvent {
  return {
    ...existing,
    ...incoming,
    id: incoming.id || existing.id,
    phase: incoming.phase || existing.phase,
    status: incoming.status || existing.status,
    tool_name: incoming.tool_name || existing.tool_name,
    tool_label: incoming.display_label || incoming.tool_label || existing.tool_label,
    display_label: incoming.display_label || existing.display_label,
    icon_key: incoming.icon_key || existing.icon_key,
    summary: incoming.summary || existing.summary,
    activity_kind: incoming.activity_kind || existing.activity_kind,
    arguments: Object.keys(incoming.arguments ?? {}).length > 0 ? incoming.arguments : existing.arguments,
    result: incoming.result ?? existing.result,
    error: incoming.error || existing.error,
    duration_ms: incoming.duration_ms ?? existing.duration_ms,
    is_long_running: incoming.is_long_running ?? existing.is_long_running,
    started_at: incoming.started_at || existing.started_at,
    finished_at: incoming.finished_at || existing.finished_at,
    error_code: incoming.error_code || existing.error_code,
    i18n_key: incoming.i18n_key || existing.i18n_key,
    run_id: incoming.run_id || existing.run_id,
    trace_id: incoming.trace_id || existing.trace_id,
    agent_key: incoming.agent_key || existing.agent_key,
    agent_id: incoming.agent_id || existing.agent_id,
    agent_name: incoming.agent_name || existing.agent_name,
    occurred_at: incoming.occurred_at || existing.occurred_at,
  };
}

/** Mark in-flight tool activity rows as cancelled when run_status=cancelled. */
export function cancelRunningToolMessages(messages: Message[], reason = '用户已停止生成'): Message[] {
  return patchOrphanToolMessages(messages, reason, 'cancelled', MESSAGE_STATUS.TOOL_CANCELLED, [
    MESSAGE_STATUS.TOOL_RUNNING,
    MESSAGE_STATUS.TOOL_BLOCKED,
  ]);
}

/** Mark orphan in-flight tool rows when a turn ends without tool_result. */
export function finalizeOrphanToolMessages(
  messages: Message[],
  reason = 'Turn 已完成，未收到工具结果',
  statuses: string[] = [MESSAGE_STATUS.TOOL_RUNNING, MESSAGE_STATUS.TOOL_BLOCKED],
): Message[] {
  return patchOrphanToolMessages(messages, reason, 'failed', MESSAGE_STATUS.TOOL_FAILED, statuses);
}

function patchOrphanToolMessages(
  messages: Message[],
  reason: string,
  eventStatus: ToolUseEvent['status'],
  messageStatus: string,
  statuses: string[],
): Message[] {
  const allowed = new Set(statuses);
  let changed = false;
  const next = messages.map((msg) => {
    if (!allowed.has(msg.status || '')) return msg;
    const event = toolEventFromMessage(msg);
    if (!event) return msg;
    changed = true;
    const patched: ToolUseEvent = {
      ...event,
      status: eventStatus,
      phase: 'after',
      error: event.error || reason,
      finished_at: new Date().toISOString(),
    };
    const row = toolEventToMessage(msg.session_id, patched);
    row.id = msg.id;
    return { ...msg, ...row, id: msg.id, status: messageStatus };
  });
  return changed ? next : messages;
}

/**
 * Cache for {@link toolEventFromMessage} parsed results, keyed by `message.id`.
 *
 * Only used for the `JSON.parse(options_json)` fallback path — messages with
 * `tool_event` already set take the fast path and bypass this cache entirely.
 *
 * Invalidation: {@link clearToolEventCache} is called from `messageStore` on
 * session switch / message reload / message clear, so stale entries from a
 * previous server load cannot leak into a new session's view.
 */
const toolEventCache = new Map<string, ToolUseEvent | null>();

export function toolEventFromMessage(message: Message): ToolUseEvent | null {
  // `tool_event` is typed as `unknown` on the cross-domain Message facade.
  // Run the candidate through isToolUseEvent so the cast is structural, not
  // a blind `as ToolUseEvent` (regression: corrupted / partial objects were
  // leaking through as "valid" tool events).
  if (isToolUseEvent(message.tool_event)) {
    return message.tool_event;
  }
  // Cache lookup (only for the JSON.parse fallback path). Skip cache when
  // message.id is missing — those rows are ephemeral and cannot be safely
  // keyed.
  const cacheKey = message.id || '';
  if (cacheKey) {
    const cached = toolEventCache.get(cacheKey);
    if (cached !== undefined) {
      return cached;
    }
  }
  let result: ToolUseEvent | null = null;
  try {
    const raw = JSON.parse(message.options_json || '{}') as { tool_event?: unknown };
    if (isToolUseEvent(raw.tool_event)) {
      result = raw.tool_event;
    }
  } catch {
    result = null;
  }
  if (cacheKey) {
    toolEventCache.set(cacheKey, result);
  }
  return result;
}

/**
 * Clear the {@link toolEventCache}. Call from `messageStore` on session switch,
 * message reload, or message clear to prevent stale parsed results from a
 * previous server load (server may update `options_json` for the same id).
 *
 * @param messageIds When provided, removes only the given ids; otherwise
 *                   clears the entire cache.
 */
export function clearToolEventCache(messageIds?: string[]): void {
  if (!messageIds) {
    toolEventCache.clear();
    return;
  }
  for (const id of messageIds) {
    toolEventCache.delete(id);
  }
}
