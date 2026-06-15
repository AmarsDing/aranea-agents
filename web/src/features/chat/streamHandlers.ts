import type { Envelope } from './envelope';
import type { UseEnvelopeStreamReturn } from './useEnvelopeStream';
import type { Message } from './types';
import type { Session } from '../session/types';
import { upsertToolMessage, finalizeOrphanToolMessages, toolEventFromMessage } from './envelopeToolCall';
import { patchStreamingMessage } from './streamContentPatch';
import { createMessageBatchWriter } from './messageStoreBatch';
import { shouldSessionWsSkipEnvelope } from './inboundSyncRouting';
import { sessionContextPatchFromEnvelope, isSessionCompressNotice } from './sessionContextPatch';
import type { SessionContextPatch } from './sessionContextPatch';

import { originFromId } from './messageOrigin';
import { formatErrorWithHint } from './errorCodeHints';
import {
  createStreamingMessageFromActivity,
  patchStreamingMessageFromDelta,
  finalizeStreamingMessageFromDone,
  createToolMessageFromActivityStart,
  mergeToolResultFromDone,
} from './activityMessageAdapter';
import { toolEventToMessage } from './toolEventMarkdown';
import type { ActivityStartMeta, ActivityDeltaMeta, ActivityDoneMeta } from './activityTypes';

export function createPlaceholderMessage(id: string, sessionID: string, role: string, content: string): Message {
  return {
    id,
    session_id: sessionID,
    parent_message_id: '',
    turn_id: '',
    turn_number: 0,
    seq_in_turn: 0,
    role,
    content_markdown: content,
    model_name: 'mock',
    token_in: 0,
    token_out: 0,
    latency_ms: 0,
    status: 'ok',
    attachments_count: 0,
    options_json: '',
    error_message: '',
    created_at: new Date().toISOString(),
    origin: originFromId(id, role),
    agent_ref: null,
    team_member: null,
    source_meta: null,
  };
}

export type StreamHandlerCtx = {
  sessionId: string;
  resolveActiveSessionId: () => string | null;
  getMessages: (sessionId: string) => Message[];
  setMessages: (sessionId: string, rows: Message[]) => void;
  markSendingDone: () => void;
  clearSendingTimeout?: () => void;
  onRunAccepted?: () => void;
  onRunStatus: (env: Envelope) => void;
  onErrorNotify: (message: string) => void;
  onOrchestrationNotice?: (message: string) => void;
  onReloadAfterCompletion: (sessionId: string, opts?: { activityFirst?: boolean }) => Promise<void>;
  onSessionContextPatch?: (sessionId: string, patch: SessionContextPatch) => void;
  onCompressNotice?: (sessionId: string, prevRatio: number, newRatio: number) => void;
  getSessionMetrics?: (
    sessionId: string,
  ) => Pick<Session, 'total_tokens' | 'max_context_used_ratio' | 'input_tokens' | 'output_tokens'> | undefined;
  onStreamingPatch?: (sessionId: string, patch: { reasoning?: string; partialText?: string; done?: boolean }) => void;
  onRunActivity?: () => void;
  onFirstByteArrived?: () => void;
  /**
   * Chat-visible execution progress event (orchestration / team / tool /
   * thinking step). Consumers can accumulate these envelopes to drive inline
   * progress cards in the AgentTreeTimeline.
   *
   * See docs/reports/2026-06-10-proposal-execution-progress-inline.md
   */
  onExecutionProgress?: (env: Envelope) => void;
  /** Team-only: resolve member meta for member_* envelopes */
  resolveMemberMeta?: (agentKey: string) => { agent_key: string; name: string; role: string };
  streamIdPrefix?: string;
  /**
   * H-03: Maximum time (ms) to wait for a runner_completion event after the
   * first streaming event arrives. If exceeded, the streaming session is
   * forcefully finalized to prevent messages from being stuck in 'streaming'
   * status indefinitely. Defaults to 5 minutes if not provided.
   */
  streamTimeoutMs?: number;
  /** AF mode: route activity envelopes to timeline handler */
  onActivityEnvelope?: (env: Envelope) => void;
};

function streamRowId(ctx: StreamHandlerCtx, sessionId: string): string {
  const prefix = ctx.streamIdPrefix ?? 'ws-stream';
  return `${prefix}-${sessionId}`;
}

/** Remove the "thinking" placeholder message created on run_status=running.
 * Called when real streaming content (text_delta or activity_start) arrives. */
function removeThinkingPlaceholder(ctx: StreamHandlerCtx, sid: string) {
  const msgs = ctx.getMessages(sid);
  const idx = msgs.findIndex((m) => m.id.startsWith('run-') && m.model_name === 'thinking');
  if (idx >= 0) {
    const updated = [...msgs];
    updated.splice(idx, 1);
    ctx.setMessages(sid, updated);
  }
}

/**
 * Snapshot the current streaming message into a standalone assistant message
 * with a stable ID, so that subsequent LLM rounds (after tool calls) get their
 * own streaming message. This enables the Activity Timeline to display each
 * round of thinking/replying as separate ThinkActivity/SayActivity nodes.
 *
 * Returns the new streamId for the next round.
 */
function snapshotStreamingMessage(
  messages: Message[],
  sessionId: string,
  streamId: string,
): { messages: Message[]; newStreamId: string } {
  const streamIdx = messages.findIndex((m) => m.id === streamId);
  if (streamIdx < 0) return { messages, newStreamId: streamId };

  const streamMsg = messages[streamIdx];
  // Only snapshot if the streaming message has content (reasoning or text)
  const hasContent =
    (streamMsg.content_markdown?.trim() ?? '') !== '' || (streamMsg.reasoning_markdown?.trim() ?? '') !== '';
  if (!hasContent) return { messages, newStreamId: streamId };

  // Generate a stable ID for the snapshot: ws-snap-{sessionId}-{counter}
  // Using a monotonic counter instead of Date.now() to avoid collisions
  // when multiple snapshots occur within the same millisecond.
  snapshotCounter++;
  const snapId = `ws-snap-${sessionId}-${snapshotCounter}`;
  const snapMsg: Message = {
    ...streamMsg,
    id: snapId,
    status: 'ok', // Snapshot is complete — no longer streaming
  };

  // Replace the streaming message with the snapshot
  const next = [...messages];
  next[streamIdx] = snapMsg;
  return { messages: next, newStreamId: streamId };
}

// Monotonic counter for message IDs — avoids Date.now() collisions.
// Used by both legacy snapshot (ws-snap-*) and AF mode (ws-af-*) paths.
// TECH-DEBT: Module-level mutable state. If parallel streaming sessions
// ever need isolated counters, move this into StreamHandlerCtx or the
// bindStreamHandlers closure. For now, shared counter is acceptable because
// uniqueness (not isolation) is the only requirement.
let snapshotCounter = 0;

function patchMessages(ctx: StreamHandlerCtx, sessionId: string, streamId: string, env: Envelope, isDone: boolean) {
  ctx.setMessages(sessionId, patchStreamingEnvelope(ctx.getMessages(sessionId), sessionId, streamId, env, isDone));
}

function markPendingUserFailed(messages: Message[], pendingId: string, errorMessage: string): Message[] {
  return messages.map((m) =>
    m.id === pendingId
      ? {
          ...m,
          status: 'failed',
          error_message: errorMessage,
        }
      : m,
  );
}

/**
 * Mark all active streaming assistant messages (ws-stream-*) as failed.
 * Called when an error event arrives during streaming, ensuring that
 * in-progress assistant messages don't remain stuck in 'streaming' status.
 */
function markStreamingMessagesFailed(messages: Message[], sessionId: string, errorMessage: string): Message[] {
  return messages.map((m) => {
    if (m.role !== 'assistant' || m.status !== 'streaming') return m;
    if (m.origin?.kind !== 'streaming') return m;
    if (m.session_id !== sessionId) return m;
    return {
      ...m,
      status: 'failed',
      error_message: errorMessage,
    };
  });
}

function latestPendingUserId(messages: Message[]): string {
  for (let i = messages.length - 1; i >= 0; i--) {
    const id = messages[i]?.id ?? '';
    if (id.startsWith('pending-user-')) return id;
  }
  return '';
}

/** Shared streaming row patch for WS handlers and inbound sync. */
export function patchStreamingEnvelope(
  messages: Message[],
  sessionId: string,
  streamId: string,
  env: Envelope,
  isDone: boolean,
): Message[] {
  const cur = messages;
  const exists = cur.some((m) => m.id === streamId);
  let next = cur;
  if (!exists) {
    next = [
      ...cur,
      {
        ...createPlaceholderMessage(streamId, sessionId, 'assistant', ''),
        status: 'streaming',
      },
    ];
  }
  return patchStreamingMessage(next, streamId, {
    text: isDone ? undefined : env.content?.text,
    reasoning: isDone ? undefined : env.content?.reasoning,
    replaceText: isDone ? env.content?.text : undefined,
    replaceReasoning: isDone ? env.content?.reasoning : undefined,
    status: isDone ? 'ok' : 'streaming',
  });
}

function withSessionFilter(
  ctx: StreamHandlerCtx,
  handler: (env: Envelope, sid: string) => void,
): (env: Envelope) => void {
  return (env: Envelope) => {
    if (shouldSessionWsSkipEnvelope(env)) return;
    const sid = env.session_id || ctx.sessionId;
    const active = ctx.resolveActiveSessionId();
    if (sid !== ctx.sessionId || (active && sid !== active)) return;
    handler(env, sid);
  };
}

export function bindStreamHandlers(
  stream: UseEnvelopeStreamReturn,
  ctx: StreamHandlerCtx,
  opts?: { batched?: boolean },
): () => void {
  const batched = opts?.batched ?? false;
  const writer = batched
    ? createMessageBatchWriter(
        () => ctx.getMessages(ctx.sessionId),
        (rows) => ctx.setMessages(ctx.sessionId, rows),
      )
    : null;

  let afMode = false; // AF mode: activity events drive messageStore

  // Structural AF transition: once activity_start(kind=task) is received,
  // unsubscribe all legacy handlers from the dispatcher. This eliminates the
  // runtime `if (afMode) return` check on every event and makes the AF
  // transition structural — legacy handlers are physically removed, not
  // short-circuited. This prevents any possibility of dual-path processing.
  const legacyUnsubs: (() => void)[] = [];
  function activateAFMode() {
    if (afMode) return;
    afMode = true;
    for (const unsub of legacyUnsubs) unsub();
    legacyUnsubs.length = 0;
  }

  // H-03: Stream timeout protection. If runner_completion never arrives
  // (e.g., WS disconnect), force-finalize all streaming/tool_running messages
  // after the timeout expires. The timer starts on the first streaming event
  // and is cleared when runner_completion or error arrives.
  const STREAM_TIMEOUT_MS = ctx.streamTimeoutMs ?? 5 * 60 * 1_000;
  let streamTimeoutId: ReturnType<typeof setTimeout> | null = null;
  let streamStarted = false;

  function startStreamTimeout() {
    if (streamStarted) return;
    streamStarted = true;
    streamTimeoutId = setTimeout(() => {
      const sid = ctx.resolveActiveSessionId() ?? ctx.sessionId;
      if (!sid) return;
      writer?.flushSync();
      let rows = ctx.getMessages(sid);
      // Finalize orphan tool messages
      rows = finalizeOrphanToolMessages(rows, '流式响应超时，已自动结束');
      // Mark streaming assistant messages as failed
      rows = markStreamingMessagesFailed(rows, sid, '流式响应超时，已自动结束');
      ctx.setMessages(sid, rows);
      ctx.markSendingDone();
      ctx.onErrorNotify('流式响应超时，已自动结束');
    }, STREAM_TIMEOUT_MS);
  }

  function clearStreamTimeout() {
    if (streamTimeoutId !== null) {
      clearTimeout(streamTimeoutId);
      streamTimeoutId = null;
    }
  }

  function patch(sessionId: string, streamId: string, env: Envelope, isDone: boolean) {
    if (writer) {
      writer.update((cur) => patchStreamingEnvelope(cur, sessionId, streamId, env, isDone));
      return;
    }
    patchMessages(ctx, sessionId, streamId, env, isDone);
  }

  function applySessionContextPatch(sessionId: string, env: Envelope) {
    if (!ctx.onSessionContextPatch && !ctx.onCompressNotice) return;
    const prev = ctx.getSessionMetrics?.(sessionId);
    const patch = sessionContextPatchFromEnvelope(env, prev);
    if (patch) {
      ctx.onSessionContextPatch?.(sessionId, patch);
    }
    if (isSessionCompressNotice(env) && ctx.onCompressNotice) {
      const prevRatio = prev?.max_context_used_ratio ?? 0;
      const newRatio = patch?.context_used_ratio ?? 0;
      ctx.onCompressNotice(sessionId, prevRatio, newRatio);
    }
  }

  stream.onType(
    'context_usage',
    withSessionFilter(ctx, (_env, sid) => {
      applySessionContextPatch(sid, _env);
    }),
  );

  legacyUnsubs.push(
    stream.onType(
      'text_delta',
      withSessionFilter(ctx, (env, sid) => {
      if (!env.content?.text && !env.content?.reasoning) return;
      ctx.onRunActivity?.();
      ctx.onFirstByteArrived?.();
      // Remove placeholder message now that real content is arriving.
      removeThinkingPlaceholder(ctx, sid);
      startStreamTimeout();
      ctx.onStreamingPatch?.(sid, {
        reasoning: env.content?.reasoning,
        partialText: env.content?.text,
      });
      patch(sid, streamRowId(ctx, sid), env, false);
    }),
  ),
  );

  legacyUnsubs.push(
    stream.onType(
    'text_done',
    withSessionFilter(ctx, (env, sid) => {

      ctx.onStreamingPatch?.(sid, {
        reasoning: env.content?.reasoning,
        partialText: env.content?.text,
        done: true,
      });

      // The backend resets stream builders when tool_call is detected,
      // so text_done now contains only the CURRENT round's content (not
      // accumulated across all rounds). This means we can safely use
      // replaceText/replaceReasoning to set the authoritative final content
      // for the current ws-stream-* message.
      // ws-snap-* messages already contain earlier rounds' content and are
      // unaffected by this text_done event.
      const streamId = streamRowId(ctx, sid);
      patch(sid, streamId, env, true);
      applySessionContextPatch(sid, env);
    }),
  ),
  );

  legacyUnsubs.push(
    stream.onType(
    'tool_call',
    withSessionFilter(ctx, (env, sid) => {

      if (!env.tool_call) return;
      ctx.onRunActivity?.();

      // Snapshot the current streaming message before the tool call.
      // This converts the in-progress ws-stream-* message into a standalone
      // assistant message (ws-snap-*), so that the next LLM round gets its own
      // streaming message. This is the key mechanism for separating multiple
      // rounds of thinking/replying in the Activity Timeline.
      const streamId = streamRowId(ctx, sid);

      if (writer) {
        // C-02 fix: perform snapshot inside the writer.update callback so that
        // the snapshot is computed from the writer's pending state (which may
        // include unflushed text_delta updates), not from the stale store.
        writer.update((cur) => {
          const { messages: snapped } = snapshotStreamingMessage(cur, sid, streamId);
          return upsertToolMessage(snapped, sid, env, 'before');
        });
        return;
      }
      // Non-batched path: read directly from store (no pending state to lose)
      const cur = ctx.getMessages(sid);
      const { messages: snapped } = snapshotStreamingMessage(cur, sid, streamId);
      ctx.setMessages(sid, upsertToolMessage(snapped, sid, env, 'before'));
    }),
  ),
  );

  legacyUnsubs.push(
    stream.onType(
    'tool_result',
    withSessionFilter(ctx, (env, sid) => {

      if (!env.tool_call) return;
      if (writer) {
        writer.update((cur) => upsertToolMessage(cur, sid, env, 'after'));
        return;
      }
      ctx.setMessages(sid, upsertToolMessage(ctx.getMessages(sid), sid, env, 'after'));
    }),
  ),
  );

  stream.onType('run_status', (env: Envelope) => {
    if (env.session_id && env.session_id !== ctx.sessionId) return;
    ctx.onRunActivity?.();
    ctx.onRunStatus(env);
    const status = String(env.metadata?.status ?? '');
    if (status === 'running' || status === 'accepted') {
      ctx.clearSendingTimeout?.();
      ctx.onRunAccepted?.();

      // Create a placeholder assistant message so the user sees "正在思考"
      // immediately instead of staring at a blank screen during the BUILD
      // phase (0-15s). The placeholder is replaced when the first
      // activity_start or text_delta arrives.
      const sid = ctx.sessionId;
      const placeholderId = `run-${env.metadata?.run_id ?? 'pending'}`;
      const existing = ctx.getMessages(sid);
      if (!existing.some((m) => m.id === placeholderId)) {
        const placeholder: Message = {
          ...createPlaceholderMessage(placeholderId, sid, 'assistant', ''),
          status: 'streaming',
          model_name: 'thinking',
          origin: { kind: 'assistant' },
        };
        ctx.setMessages(sid, [...existing, placeholder]);
      }
    } else if (status === 'failed' || status === 'cancelled' || status === 'completed') {
      // Remove the thinking placeholder on terminal status.
      // Under normal flow, text_delta or activity_start already removed it,
      // but guard against edge cases (e.g., empty LLM response).
      removeThinkingPlaceholder(ctx, ctx.sessionId);
    }
  });

  if (ctx.resolveMemberMeta) {
    stream.onType('member_message_start', (env: Envelope) => {
      const sid = ctx.resolveActiveSessionId();
      if (!sid || !env.author) return;
      const msgId = `member-${env.author}`;
      const meta = ctx.resolveMemberMeta!(env.author);
      const newMsg: Message = {
        ...createPlaceholderMessage(msgId, sid, 'assistant', ''),
        status: 'streaming',
        model_name: `team/${meta.role || 'member'}`,
        options_json: JSON.stringify({ team_member: meta }),
        origin: { kind: 'team_member', agentKey: env.author },
        team_member: { agent_id: '', name: meta.name, role: meta.role },
      };
      // Use batch writer when available for consistency with member_delta/done.
      // member_message_start needs immediate rendering, but the writer's
      // RAF flush is fast enough (~16ms) that the delay is imperceptible.
      if (writer && sid === ctx.sessionId) {
        writer.update((cur) => {
          if (cur.some((m) => m.id === msgId)) return cur;
          return [...cur, newMsg];
        });
        return;
      }
      const cur = ctx.getMessages(sid);
      if (cur.some((m) => m.id === msgId)) return;
      ctx.setMessages(sid, [...cur, newMsg]);
    });

    stream.onType('member_delta', (env: Envelope) => {
      const sid = ctx.resolveActiveSessionId();
      if (!sid || !env.author) return;
      const msgId = `member-${env.author}`;
      // M-01 fix: use batch writer when available to avoid racing with
      // other streaming updates on the same session.
      if (writer && sid === ctx.sessionId) {
        writer.update((cur) =>
          patchStreamingMessage(cur, msgId, {
            text: env.content?.text,
            reasoning: env.content?.reasoning,
          }),
        );
        return;
      }
      ctx.setMessages(
        sid,
        patchStreamingMessage(ctx.getMessages(sid), msgId, {
          text: env.content?.text,
          reasoning: env.content?.reasoning,
        }),
      );
    });

    stream.onType('member_message_done', (env: Envelope) => {
      const sid = ctx.resolveActiveSessionId();
      if (!sid || !env.author) return;
      const msgId = `member-${env.author}`;
      if (writer && sid === ctx.sessionId) {
        writer.update((cur) =>
          patchStreamingMessage(cur, msgId, {
            replaceText: env.content?.text,
            replaceReasoning: env.content?.reasoning,
            status: 'ok',
          }),
        );
        return;
      }
      ctx.setMessages(
        sid,
        patchStreamingMessage(ctx.getMessages(sid), msgId, {
          replaceText: env.content?.text,
          replaceReasoning: env.content?.reasoning,
          status: 'ok',
        }),
      );
    });
  }

  legacyUnsubs.push(
    stream.onType('runner_completion', async (env: Envelope) => {
    clearStreamTimeout();
    ctx.markSendingDone();
    writer?.flushSync();
    const sid = ctx.resolveActiveSessionId() ?? ctx.sessionId;
    if (!sid) return;
    // Finalize orphan tool messages but keep ws-stream rows as fallback until
    // the server-persisted messages arrive via onReloadAfterCompletion.
    // The merge logic in mergeSessionMessages will deduplicate ws-stream rows
    // against server messages by matching content + role + session_id, so there
    // is no visible duplication. This prevents the brief flicker where the
    // assistant reply disappears between removing ws-stream rows and loading
    // the persisted version.
    const finalized = finalizeOrphanToolMessages(ctx.getMessages(sid));
    ctx.setMessages(sid, finalized);
    applySessionContextPatch(sid, env);
    if (shouldSessionWsSkipEnvelope(env)) {
      return;
    }
    try {
      await ctx.onReloadAfterCompletion(sid);
    } catch {
      /* caller may surface errors */
    }
  }),
  );

  legacyUnsubs.push(
    stream.onType('error', async (env: Envelope) => {
    clearStreamTimeout();
    const errType = env.error?.type ?? '';
    if (errType.startsWith('flow_')) return;
    const hint = env.error?.hint?.trim();
    const msg = env.error?.message ?? 'stream failed';
    const errorCode = env.error?.code;
    ctx.onErrorNotify(hint ? `${formatErrorWithHint(msg, errorCode)} — ${hint}` : formatErrorWithHint(msg, errorCode));
    const sid = ctx.resolveActiveSessionId() ?? ctx.sessionId;
    writer?.flushSync();
    if (sid) {
      // H-02 fix: mark both the pending user message and any active
      // streaming assistant message as failed, so neither gets stuck
      // in a non-terminal status.
      let rows = ctx.getMessages(sid);
      const pendingUserId = env.request_id?.startsWith('pending-user-') ? env.request_id : latestPendingUserId(rows);
      if (pendingUserId) {
        rows = markPendingUserFailed(rows, pendingUserId, msg);
      }
      rows = markStreamingMessagesFailed(rows, sid, msg);
      ctx.setMessages(sid, rows);
    }
    ctx.markSendingDone();
    if (sid && !shouldSessionWsSkipEnvelope(env)) {
      try {
        await ctx.onReloadAfterCompletion(sid);
      } catch {
        /* caller may surface errors */
      }
    }
  }),
  );

  stream.onType('intent_pass', (env: Envelope) => {
    const meta = env.metadata as Record<string, unknown> | undefined;
    const kind = typeof meta?.intent_kind === 'string' ? meta.intent_kind : '';
    const target = typeof meta?.target_agent === 'string' ? meta.target_agent : '';
    const parts = ['编排：意图识别'];
    if (kind) parts.push(kind);
    if (target) parts.push(`→ ${target}`);
    ctx.onOrchestrationNotice?.(parts.join(' · '));
  });

  stream.onType('transfer', (env: Envelope) => {
    const from = env.transfer?.from_agent?.trim() ?? '';
    const to = env.transfer?.to_agent?.trim() ?? '';
    if (!from && !to) return;
    ctx.onOrchestrationNotice?.(`编排：正在转接 ${from || '?'} → ${to || '?'}`);
  });

  if (ctx.onExecutionProgress) {
    stream.onType(
      'execution_progress',
      withSessionFilter(ctx, (env) => {
        ctx.onExecutionProgress?.(env);
      }),
    );
  }

  // === AF (Activity-First) Handlers ===
  // When the backend enables ActivityProjector (AF mode), these handlers drive
  // the messageStore exclusively. AF mode is detected by receiving
  // activity_start(kind=task) — the root turn activity.

  stream.onType('activity_start', (env: Envelope) => {
    if (shouldSessionWsSkipEnvelope(env)) return;
    const sid = env.session_id || ctx.sessionId;
    const active = ctx.resolveActiveSessionId();
    if (sid !== ctx.sessionId || (active && sid !== active)) return;
    const md = (env.metadata ?? {}) as Record<string, unknown>;
    const kind = String(md.kind ?? '');

    // Detect AF mode on root task start — structural transition
    if (kind === 'task') {
      activateAFMode(); // Unsubscribes all legacy handlers from dispatcher
      ctx.onRunActivity?.();
      ctx.onFirstByteArrived?.();

      // Remove the placeholder message created on run_status=running.
      // The real streaming message will be created by the AF handlers below.
      removeThinkingPlaceholder(ctx, sid);
    }

    if (!afMode) return; // Only process in AF mode

    const rawActivityId = String(md.activity_id ?? '');
    if (!rawActivityId) return; // Guard: activity_id must be present
    const activityId = `actv-${rawActivityId}`;

    switch (kind) {
      case 'thinking': {
        // AF mode: Each Activity gets its own message with a unique ID
        // derived from the backend's activity_id. This ensures that
        // activity_delta and activity_done events can always find the
        // correct message, even across multiple ReAct rounds.
        const newMsg = createStreamingMessageFromActivity(activityId, sid, md as unknown as ActivityStartMeta);
        if (writer && sid === ctx.sessionId) {
          writer.update((cur) => [...cur, newMsg]);
        } else {
          const msgs = ctx.getMessages(sid);
          ctx.setMessages(sid, [...msgs, newMsg]);
        }
        startStreamTimeout();
        // No onStreamingPatch in AF mode — each Activity has its own
        // message, so delta events update it directly via activity_id.
        break;
      }
      case 'reply': {
        // Create a new streaming message for this reply Activity
        const newMsg = createStreamingMessageFromActivity(activityId, sid, md as unknown as ActivityStartMeta);
        if (writer && sid === ctx.sessionId) {
          writer.update((cur) => [...cur, newMsg]);
        } else {
          const msgs = ctx.getMessages(sid);
          ctx.setMessages(sid, [...msgs, newMsg]);
        }
        startStreamTimeout();
        ctx.onFirstByteArrived?.();
        // No onStreamingPatch in AF mode — each Activity has its own
        // message, so delta events update it directly via activity_id.
        break;
      }
      case 'action': {
        // Create tool message for this action Activity
        const toolMsg = createToolMessageFromActivityStart(sid, md as unknown as ActivityStartMeta);
        if (writer && sid === ctx.sessionId) {
          writer.update((cur) => [...cur, toolMsg]);
        } else {
          const msgs = ctx.getMessages(sid);
          ctx.setMessages(sid, [...msgs, toolMsg]);
        }
        break;
      }
      case 'error': {
        clearStreamTimeout();
        const errMsg = String(md.content ?? 'stream failed');
        const errType = String(md.error_type ?? '');
        if (!errType.startsWith('flow_')) {
          ctx.onErrorNotify?.(errMsg);
          const msgs = ctx.getMessages(sid);
          const withFailed = markStreamingMessagesFailed(msgs, sid, errMsg);
          const pendingId = latestPendingUserId(withFailed);
          const final = pendingId ? markPendingUserFailed(withFailed, pendingId, errMsg) : withFailed;
          ctx.setMessages(sid, final);
        }
        ctx.markSendingDone();
        break;
      }
    }

    // Also update activity timeline
    ctx.onActivityEnvelope?.(env);
  });

  stream.onType('activity_delta', (env: Envelope) => {
    if (!afMode) return;
    if (shouldSessionWsSkipEnvelope(env)) return;
    const sid = env.session_id || ctx.sessionId;
    const active = ctx.resolveActiveSessionId();
    if (sid !== ctx.sessionId || (active && sid !== active)) return;
    const md = (env.metadata ?? {}) as Record<string, unknown>;
    const field = String(md.delta_field ?? '');
    const chunk = String(md.delta_chunk ?? '');
    if (!chunk) return;

    // Use activity_id to find the correct message — each Activity has
    // its own unique message ID (actv-<uuid>), so delta events always
    // target the right message regardless of ReAct round.
    const rawActivityId = String(md.activity_id ?? '');
    if (!rawActivityId) return; // Guard: activity_id must be present
    const activityId = `actv-${rawActivityId}`;

    ctx.onRunActivity?.();

    // No onStreamingPatch in AF mode — each Activity has its own message,
    // so delta events update it directly via activity_id lookup above.

    if (writer && sid === ctx.sessionId) {
      writer.update((cur) => {
        const idx = cur.findIndex((m) => m.id === activityId);
        if (idx < 0) return cur;
        const patched = patchStreamingMessageFromDelta(cur[idx], md as unknown as ActivityDeltaMeta);
        const next = [...cur];
        next[idx] = patched;
        return next;
      });
    } else {
      const msgs = ctx.getMessages(sid);
      const idx = msgs.findIndex((m) => m.id === activityId);
      if (idx >= 0) {
        const patched = patchStreamingMessageFromDelta(msgs[idx], md as unknown as ActivityDeltaMeta);
        const next = [...msgs];
        next[idx] = patched;
        ctx.setMessages(sid, next);
      }
    }

    ctx.onActivityEnvelope?.(env);
  });

  stream.onType('activity_done', async (env: Envelope) => {
    if (!afMode) return;
    if (shouldSessionWsSkipEnvelope(env)) return;
    const sid = env.session_id || ctx.sessionId;
    const active = ctx.resolveActiveSessionId();
    if (sid !== ctx.sessionId || (active && sid !== active)) return;
    const md = (env.metadata ?? {}) as Record<string, unknown>;
    const kind = String(md.kind ?? '');
    const rawActivityId = String(md.activity_id ?? '');
    const activityId = rawActivityId ? `actv-${rawActivityId}` : '';

    switch (kind) {
      case 'thinking': {
        // Finalize the thinking message using its activity_id
        if (!activityId) break;
        writer?.flushSync();
        const msgs = ctx.getMessages(sid);
        const idx = msgs.findIndex((m) => m.id === activityId);
        if (idx >= 0) {
          const finalized = finalizeStreamingMessageFromDone(msgs[idx], md as unknown as ActivityDoneMeta);
          const next = [...msgs];
          next[idx] = finalized;
          ctx.setMessages(sid, next);
        }
        break;
      }
      case 'reply': {
        // Finalize the reply message using its activity_id
        if (!activityId) break;
        writer?.flushSync();
        const msgs = ctx.getMessages(sid);
        const idx = msgs.findIndex((m) => m.id === activityId);
        if (idx >= 0) {
          const finalized = finalizeStreamingMessageFromDone(msgs[idx], md as unknown as ActivityDoneMeta);
          const next = [...msgs];
          next[idx] = finalized;
          ctx.setMessages(sid, next);
        }
        break;
      }
      case 'action': {
        // Update tool message with result
        const toolCallId = String(md.tool_call_id ?? '');
        const actId = `act-${toolCallId}`;
        if (writer && sid === ctx.sessionId) {
          writer.update((cur) => {
            const idx = cur.findIndex((m) => m.id === actId);
            if (idx < 0) return cur;
            const existing = toolEventFromMessage(cur[idx]);
            if (!existing) return cur;
            const merged = mergeToolResultFromDone(existing, md as unknown as ActivityDoneMeta);
            const updated = toolEventToMessage(sid, merged);
            updated.id = actId;
            const next = [...cur];
            next[idx] = { ...cur[idx], ...updated, id: actId };
            return next;
          });
        } else {
          const msgs = ctx.getMessages(sid);
          const idx = msgs.findIndex((m) => m.id === actId);
          if (idx >= 0) {
            const existing = toolEventFromMessage(msgs[idx]);
            if (existing) {
              const merged = mergeToolResultFromDone(existing, md as unknown as ActivityDoneMeta);
              const updated = toolEventToMessage(sid, merged);
              updated.id = actId;
              const next = [...msgs];
              next[idx] = { ...msgs[idx], ...updated, id: actId };
              ctx.setMessages(sid, next);
            }
          }
        }
        break;
      }
      case 'task': {
        // Equivalent to runner_completion — but in AF mode, Activity events
        // already provide complete per-round data. Skip message reload to
        // prevent the server's merged assistant message from replacing the
        // correctly separated streaming state (which would cause UI jumping).
        clearStreamTimeout();
        ctx.markSendingDone();
        writer?.flushSync();
        const finalized = finalizeOrphanToolMessages(ctx.getMessages(sid));
        ctx.setMessages(sid, finalized);
        // Apply usage if present
        const usage = md.usage as Record<string, unknown> | undefined;
        if (usage) {
          applySessionContextPatch(sid, env);
        }
        if (shouldSessionWsSkipEnvelope(env)) return;
        try {
          await ctx.onReloadAfterCompletion?.(sid, { activityFirst: true });
        } catch {
          /* caller may surface errors */
        }
        break;
      }
    }

    ctx.onActivityEnvelope?.(env);
  });

  stream.onType('activity_child_start', (env: Envelope) => {
    if (!afMode) return;
    ctx.onActivityEnvelope?.(env);
  });

  // Return a cleanup function that clears the stream timeout and disposes
  // the batch writer. Callers should invoke this when the stream is
  // disconnected or the component is unmounted to prevent the timeout
  // from firing on stale state or the writer from flushing after unmount.
  return () => {
    clearStreamTimeout();
    writer?.dispose();
  };
}
