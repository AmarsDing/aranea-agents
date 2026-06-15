import type { Message } from './types';
import type { ToolUseEvent, ActivityKind as LegacyActivityKind } from './types';
import type { Activity, ActivityStartMeta, ActivityDeltaMeta, ActivityDoneMeta } from './activityTypes';
import { createPlaceholderMessage } from './streamHandlers';
import { toolEventToMessage } from './toolEventMarkdown';
import { canonicalToolStatus } from './lib/statusMap';
import { activityMessageId } from './lib/activityMessageId';

/**
 * activityMessageAdapter translates Activity event metadata into Message
 * objects for the messageStore. This is the AF (Activity-First) path —
 * when ActivityProjector is active on the backend, the frontend message list
 * is driven exclusively by activity events through this adapter.
 *
 * Design principle: direct Message construction from activity metadata,
 * no intermediate pseudo-Envelope translation.
 */

/** Build a streaming assistant Message for a thinking or reply activity start. */
export function createStreamingMessageFromActivity(
  streamId: string,
  sessionId: string,
  md: ActivityStartMeta,
): Message {
  const msg = createPlaceholderMessage(streamId, sessionId, 'assistant', '');
  msg.status = 'streaming';
  msg.origin = { kind: 'streaming', sessionId };
  if (md.agent_name) msg.model_name = md.agent_name;
  if (md.kind === 'thinking') {
    msg.reasoning_markdown = '';
  }
  return msg;
}

/** Patch a streaming Message with an activity delta chunk. */
export function patchStreamingMessageFromDelta(msg: Message, md: ActivityDeltaMeta): Message {
  // H-01 fix: lifecycle state guard (consistent with streamContentPatch.ts).
  // Once a message reaches a terminal status (ok, failed), incremental
  // appends are rejected. This prevents out-of-order activity_delta events
  // from duplicating content after activity_done has finalized the message.
  // Cross-round deltas are safe: activity_done finalizes the old message,
  // then activity_start creates a new streaming message for the next round.
  const isTerminal = msg.status === 'ok' || msg.status === 'failed';
  if (isTerminal) return msg;

  const chunk = md.delta_chunk ?? '';
  if (!chunk) return msg;

  if (md.delta_field === 'content') {
    return { ...msg, content_markdown: (msg.content_markdown ?? '') + chunk };
  }
  if (md.delta_field === 'reasoning') {
    return { ...msg, reasoning_markdown: (msg.reasoning_markdown ?? '') + chunk };
  }
  return msg;
}

/** Finalize a streaming Message from an activity_done event. */
export function finalizeStreamingMessageFromDone(msg: Message, md: ActivityDoneMeta): Message {
  const updates: Partial<Message> = { status: 'ok' };
  if (md.kind === 'reply' && md.content !== undefined) {
    updates.content_markdown = md.content;
  }
  if (md.kind === 'thinking' && md.reasoning !== undefined) {
    updates.reasoning_markdown = md.reasoning;
  }
  return { ...msg, ...updates };
}

/** Build a ToolUseEvent from an activity_start(kind=action) metadata. */
export function toolEventFromActivityStart(md: ActivityStartMeta): ToolUseEvent {
  const toolCallId = md.tool_call_id ?? '';
  const toolName = md.tool_name ?? '';
  let args: unknown = undefined;
  if (md.tool_arguments) {
    try {
      args = JSON.parse(md.tool_arguments);
    } catch {
      args = { __raw: md.tool_arguments };
    }
  }
  return {
    id: toolCallId,
    phase: 'before',
    status: 'running',
    agent_id: '',
    agent_key: md.agent_key ?? '',
    agent_name: md.agent_name ?? md.agent_key ?? 'Agent',
    tool_name: toolName,
    tool_label: md.label ?? toolName,
    arguments: args,
    result: undefined,
    error: undefined,
    occurred_at: md.timestamp ?? new Date().toISOString(),
    duration_ms: undefined,
    activity_kind: (md.kind === 'action' ? 'tool' : md.kind) as LegacyActivityKind,
    display_label: md.label,
    started_at: md.timestamp,
  };
}

/** Merge tool result data from an activity_done(kind=action) into an existing ToolUseEvent. */
export function mergeToolResultFromDone(existing: ToolUseEvent, md: ActivityDoneMeta): ToolUseEvent {
  let result: unknown = undefined;
  if (md.tool_result) {
    try {
      result = JSON.parse(md.tool_result);
    } catch {
      result = { __raw: md.tool_result };
    }
  }
  const status = md.status === 'failed' ? 'failed' : 'success';
  const canonicalStatus = canonicalToolStatus(status);
  return {
    ...existing,
    phase: 'after',
    status: canonicalStatus,
    result,
    error: md.tool_error_code || undefined,
    duration_ms: md.tool_duration_ms ?? existing.duration_ms,
    finished_at: md.timestamp ?? existing.finished_at,
    error_code: md.tool_error_code || existing.error_code,
  };
}

/** Create a tool Message from an activity_start(kind=action). */
export function createToolMessageFromActivityStart(sessionId: string, md: ActivityStartMeta): Message {
  const event = toolEventFromActivityStart(md);
  return toolEventToMessage(sessionId, event);
}

/** Create a failed Message from an activity_start(kind=error). */
export function createFailedMessageFromError(sessionId: string, md: ActivityStartMeta, streamId: string): Message {
  const errMsg = md.content ?? 'stream failed';
  return {
    ...createPlaceholderMessage(streamId, sessionId, 'assistant', ''),
    status: 'failed',
    error_message: errMsg,
    origin: { kind: 'streaming', sessionId },
  };
}

/**
 * Reconstruct per-round `actv-*` Messages from persisted Activity records.
 *
 * When a session is reloaded from the database, the server returns a single
 * merged assistant ChatMessage per turn (all reasoning concatenated into
 * `reasoning_markdown`, all content into `content_markdown`). This loses the
 * multi-round structure needed for correct interleaved display
 * (thinking → tool → thinking → tool → reply).
 *
 * This function rebuilds individual `actv-*` messages from Activity API data
 * so that the Activity-First timeline can correctly interleave thinking, tools,
 * and reply nodes.
 *
 * Only `thinking`, `reply`, and `action` activities produce messages;
 * `task`, `end`, `error`, `sub_task_board`, `delegate`, `notice` are skipped.
 *
 * **Timestamp ordering**: Activity records store the *start* time. Multiple
 * activities within the same turn may start at the same second (or even the
 * same millisecond). To guarantee correct chronological ordering for turn-based grouping
 * (which sorts by `created_at`), we apply a tiny
 * monotonic offset to each message's `created_at` within a turn:
 *   - Each message gets an additional `N * 1µs` offset (N = sequential index)
 *   - This preserves the original order while keeping timestamps distinct
 *   - The offset is small enough to be invisible in UI display
 *
 * @param activities - Raw Activity records from the API (with turnId)
 * @param sessionId - Current session ID
 * @returns Message[] suitable for injection as local messages in mergeSessionMessages
 */
export function reconstructMessagesFromActivities(
  activities: readonly Activity[],
  sessionId: string,
): Message[] {
  if (!activities.length) return [];

  // Group activities by turnId to apply per-turn sequential offsets
  const byTurn = new Map<string, Activity[]>();
  for (const record of activities) {
    // Skip non-content activities
    if (record.kind === 'task' || record.kind === 'end' || record.kind === 'sub_task_board'
      || record.kind === 'delegate' || record.kind === 'notice') {
      continue;
    }
    const turnId = record.turnId || '';
    const arr = byTurn.get(turnId) ?? [];
    arr.push(record);
    byTurn.set(turnId, arr);
  }

  const messages: Message[] = [];

  for (const [, turnActivities] of byTurn) {
    // Activities are already sorted by timestamp ASC from the API.
    // Apply monotonic microsecond offsets to guarantee stable ordering
    // even when multiple activities share the same timestamp.
    let seqInTurn = 0;

    for (const record of turnActivities) {
      const activityId = `actv-${record.id}`;
      const turnId = record.turnId || '';
      const baseTimestamp = record.timestamp || new Date().toISOString();
      // Apply sequential microsecond offset to guarantee stable ordering.
      // 1µs increments are invisible in UI but sufficient for string sorting.
      const createdAt = addMicroOffset(baseTimestamp, seqInTurn);
      seqInTurn++;

      const agentRef = record.agentKey || record.agentName
        ? { id: '', agent_key: record.agentKey || '', name: record.agentName || '', icon: '' }
        : null;

      if (record.kind === 'thinking') {
        // Thinking activity → assistant message with reasoning_markdown
        // Use streaming_snapshot origin so isInFlightLocalRow returns false
        // (these are finalized snapshots reconstructed from Activity API data,
        // not active in-flight messages).
        messages.push({
          id: activityId,
          session_id: sessionId,
          parent_message_id: '',
          turn_id: turnId,
          turn_number: 0,
          seq_in_turn: 0,
          role: 'assistant',
          content_markdown: '',
          reasoning_markdown: record.reasoning || record.content || '',
          model_name: record.agentName || '',
          token_in: 0,
          token_out: 0,
          latency_ms: record.durationMs ?? 0,
          status: 'ok',
          attachments_count: 0,
          options_json: '',
          error_message: '',
          created_at: createdAt,
          origin: { kind: 'streaming_snapshot', sessionId },
          agent_ref: agentRef,
          team_member: null,
          source_meta: null,
        });
      } else if (record.kind === 'reply') {
        // Reply activity → assistant message with content_markdown
        messages.push({
          id: activityId,
          session_id: sessionId,
          parent_message_id: '',
          turn_id: turnId,
          turn_number: 0,
          seq_in_turn: 0,
          role: 'assistant',
          content_markdown: record.content || '',
          reasoning_markdown: '',
          model_name: record.agentName || '',
          token_in: 0,
          token_out: 0,
          latency_ms: record.durationMs ?? 0,
          status: 'ok',
          attachments_count: 0,
          options_json: '',
          error_message: '',
          created_at: createdAt,
          origin: { kind: 'streaming_snapshot', sessionId },
          agent_ref: agentRef,
          team_member: null,
          source_meta: null,
        });
      } else if (record.kind === 'action') {
        // Action activity → tool message (same as createToolMessageFromActivityStart)
        const toolEvent: ToolUseEvent = {
          id: record.toolCallId || '',
          phase: 'after',
          status: record.status === 'failed' ? 'failed' : 'success',
          agent_id: '',
          agent_key: record.agentKey || '',
          agent_name: record.agentName || record.agentKey || 'Agent',
          tool_name: record.toolName || '',
          tool_label: record.label || record.toolName || '',
          arguments: parseToolArgs(record.toolArguments),
          result: parseToolResult(record.toolResult),
          error: record.toolErrorCode || undefined,
          occurred_at: createdAt,
          duration_ms: record.toolDurationMs ?? record.durationMs ?? undefined,
          activity_kind: 'tool' as LegacyActivityKind,
          display_label: record.label,
          started_at: createdAt,
          finished_at: createdAt,
        };
        const canonicalStatus = canonicalToolStatus(toolEvent.status);
        toolEvent.status = canonicalStatus;
        const toolMsg = toolEventToMessage(sessionId, toolEvent);
        // Override turn_id and created_at from Activity record
        toolMsg.turn_id = turnId;
        toolMsg.created_at = createdAt;
        messages.push(toolMsg);
      } else if (record.kind === 'error') {
        // Error activity → failed assistant message
        messages.push({
          id: activityId,
          session_id: sessionId,
          parent_message_id: '',
          turn_id: turnId,
          turn_number: 0,
          seq_in_turn: 0,
          role: 'assistant',
          content_markdown: '',
          model_name: record.agentName || '',
          token_in: 0,
          token_out: 0,
          latency_ms: record.durationMs ?? 0,
          status: 'failed',
          attachments_count: 0,
          options_json: '',
          error_message: record.content || 'Error',
          created_at: createdAt,
          origin: { kind: 'streaming_snapshot', sessionId },
          agent_ref: agentRef,
          team_member: null,
          source_meta: null,
        });
      }
    }
  }

  return messages;
}

/**
 * Add N microseconds to an ISO 8601 timestamp string.
 * This ensures stable ordering when multiple messages share the same base timestamp.
 * The offset is invisible in UI display (1µs = 0.001ms).
 *
 * JavaScript Date only supports millisecond precision, so we manipulate the
 * fractional seconds portion of the ISO string directly. This works for both
 * second-precision (RFC3339) and nanosecond-precision (RFC3339Nano) formats.
 *
 * Examples:
 *   "2026-06-15T10:30:45Z"       + 1µs → "2026-06-15T10:30:45.000001Z"
 *   "2026-06-15T10:30:45.123Z"   + 1µs → "2026-06-15T10:30:45.123001Z"
 *   "2026-06-15T10:30:45.123456789Z" + 1µs → "2026-06-15T10:30:45.123456790Z"
 */
function addMicroOffset(isoTimestamp: string, offsetCount: number): string {
  if (offsetCount === 0) return isoTimestamp;

  // Offset in microseconds, converted to nanoseconds for string manipulation
  const offsetNs = offsetCount * 1000;

  // Parse: everything before the fractional seconds, the fractional part, and the timezone
  const match = isoTimestamp.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d+))?(Z|[+-]\d{2}:\d{2})?$/);
  if (!match) return isoTimestamp;

  const [, base, fracStr, tz] = match;
  const timezone = tz || 'Z';

  // Parse existing fractional seconds as nanoseconds (pad to 9 digits)
  let existingNs = 0;
  if (fracStr) {
    // Pad to 9 digits (nanosecond precision) and parse as integer
    const padded = fracStr.padEnd(9, '0').slice(0, 9);
    existingNs = parseInt(padded, 10);
  }

  const newNs = existingNs + offsetNs;

  // Handle carry-over to seconds
  if (newNs >= 1_000_000_000) {
    // Rare: offset pushes past the current second. Use Date arithmetic as fallback.
    try {
      const date = new Date(isoTimestamp);
      const extraMs = Math.floor(newNs / 1_000_000);
      date.setTime(date.getTime() + extraMs);
      const remainderNs = newNs % 1_000_000;
      if (remainderNs === 0) return date.toISOString();
      const frac = remainderNs.toString().padStart(6, '0');
      const iso = date.toISOString();
      return iso.replace(/\.\d+Z$/, `.${frac}Z`);
    } catch {
      return isoTimestamp;
    }
  }

  // Format new fractional seconds (at least 6 digits for microsecond precision)
  const newFrac = newNs.toString().padStart(6, '0');
  // Remove trailing zeros for compactness, but keep at least 6 digits
  const trimmedFrac = newFrac.replace(/0+$/, '') || '0';

  return `${base}.${trimmedFrac}${timezone}`;
}

function parseToolArgs(raw?: string): unknown {
  if (!raw) return undefined;
  try { return JSON.parse(raw); } catch { return { __raw: raw }; }
}

function parseToolResult(raw?: string): unknown {
  if (!raw) return undefined;
  try { return JSON.parse(raw); } catch { return { __raw: raw }; }
}
