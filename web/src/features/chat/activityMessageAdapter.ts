import type { Message } from './types';
import type { ToolUseEvent, ActivityKind as LegacyActivityKind } from './types';
import type { ActivityStartMeta, ActivityDeltaMeta, ActivityDoneMeta } from './activityTypes';
import { createPlaceholderMessage } from './streamHandlers';
import { toolEventToMessage } from './toolEventMarkdown';
import { canonicalToolStatus } from './lib/statusMap';

/**
 * activityMessageAdapter translates Activity event metadata into Message
 * objects for the messageStore. This is the AF (Activity-First) path —
 * when SkipEventProjectorWS=true on the backend, the frontend message list
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
