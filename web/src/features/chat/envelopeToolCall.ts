import type { Envelope } from './envelope';
import type { ActivityKind, ToolUseEvent } from './types';
import type { Message } from './types';
import { MESSAGE_STATUS } from '../../domain/types';
import { classifyActivityKind } from './activityPresentation';
import { canonicalToolStatus, messageStatusFromWire } from './lib/statusMap';
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
  const trimmed = raw.length > ACTIVITY_JSON_PREVIEW_LIMIT
    ? raw.slice(0, ACTIVITY_JSON_PREVIEW_LIMIT) + '…[truncated]'
    : raw;
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

/** Build a {@link ToolUseEvent} from a tool_call / tool_result Envelope. */
export function envelopeToToolEvent(env: Envelope, phase: 'before' | 'after'): ToolUseEvent | null {
  const tc = env.tool_call;
  if (!tc?.name && !tc?.id) return null;
  const args = unwrapParsed(parseJSONRecord(tc.arguments_json)) as ToolUseEvent['arguments'];
  const resultRecord = parseJSONRecord(tc.result_json);
  const result = unwrapParsed(resultRecord) as Record<string, unknown> | null | undefined;
  const toolName = tc.name || 'tool';
  const kind = (tc.activity_kind || classifyActivityKind(toolName)) as ActivityKind;
  // Resolve the user-facing error message with the following precedence:
  //   1) result.error (string body)            — most descriptive
  //   2) error_code, but only if the tool actually failed
  //   3) undefined
  // The status check covers both 'failed' and 'error' (backend tool_invocation_recorder.go
  // emits "error" — see internal/agent/tool_invocation_recorder.go:40). Without the
  // multi-value check, the error_code fallback was unreachable and the user saw
  // an empty error section.
  const status = normalizeToolStatus(tc.status || (phase === 'before' ? 'running' : 'success'));
  const isFailure = status === 'failed';
  const resultErrorStr = typeof result === 'object' && result !== null && typeof (result as { error?: unknown }).error === 'string'
    ? (result as { error: string }).error
    : undefined;
  const resultI18nKey = typeof result === 'object' && result !== null && typeof (result as { i18n_key?: unknown }).i18n_key === 'string'
    ? (result as { i18n_key: string }).i18n_key
    : undefined;
  const errorMessage = resultErrorStr ?? (isFailure && tc.error_code ? tc.error_code : undefined);
  return {
    id: tc.id || env.id,
    phase,
    status,
    agent_id: tc.agent_id || '',
    agent_key: tc.agent_key || env.author || '',
    agent_name: tc.agent_name || env.author || 'Agent',
    tool_name: toolName,
    tool_label: tc.display_label || tc.name,
    arguments: args,
    result: phase === 'after' || (result && typeof result === 'object' && Object.keys(result).length > 0) ? (result as Record<string, unknown>) : undefined,
    error: errorMessage,
    occurred_at: tc.finished_at || tc.started_at || env.timestamp || new Date().toISOString(),
    duration_ms: tc.duration_ms,
    is_long_running: tc.is_long_running,
    activity_kind: kind,
    display_label: tc.display_label,
    icon_key: tc.icon_key,
    summary: tc.summary,
    started_at: tc.started_at,
    finished_at: tc.finished_at,
    error_code: tc.error_code,
    i18n_key: resultI18nKey,
    run_id: tc.run_id,
    trace_id: tc.trace_id,
  };
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

export function upsertToolMessage(
  messages: Message[],
  sessionId: string,
  env: Envelope,
  phase: 'before' | 'after',
): Message[] {
  const event = envelopeToToolEvent(env, phase);
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

export function toolEventFromMessage(message: Message): ToolUseEvent | null {
  // `tool_event` is typed as `unknown` on the cross-domain Message facade.
  // Run the candidate through isToolUseEvent so the cast is structural, not
  // a blind `as ToolUseEvent` (regression: corrupted / partial objects were
  // leaking through as "valid" tool events).
  if (isToolUseEvent(message.tool_event)) {
    return message.tool_event;
  }
  try {
    const raw = JSON.parse(message.options_json || '{}') as { tool_event?: unknown };
    if (isToolUseEvent(raw.tool_event)) return raw.tool_event;
    return null;
  } catch {
    return null;
  }
}
