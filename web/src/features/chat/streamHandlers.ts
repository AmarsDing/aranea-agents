import type { Envelope } from './envelope';
import type { UseEnvelopeStreamReturn } from './useEnvelopeStream';
import type { Message } from './types';
import type { Session } from '../session/types';
import { finalizeOrphanToolMessages, toolEventFromMessage } from './envelopeToolCall';
import { patchStreamingMessage } from './streamContentPatch';
import { createMessageBatchWriter } from './messageStoreBatch';
import { shouldSessionWsSkipEnvelope } from './inboundSyncRouting';
import { sessionContextPatchFromEnvelope, isSessionCompressNotice } from './sessionContextPatch';
import type { SessionContextPatch } from './sessionContextPatch';

import { originFromId } from './messageOrigin';
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
  onRunActivity?: () => void;
  onFirstByteArrived?: () => void;
  /**
   * Chat-visible execution progress event (orchestration / team / tool /
   * thinking step). Consumers can accumulate these envelopes to drive inline
   * progress cards in the execution progress timeline.
   *
   * See docs/reports/2026-06-10-proposal-execution-progress-inline.md
   */
  onExecutionProgress?: (env: Envelope) => void;
  /**
   * Run heartbeat (P1-7): periodic run progress. Callers use this to update
   * progress UI; the heartbeat also resets the run-stale timer.
   */
  onHeartbeat?: (env: Envelope) => void;
  /** Team-only: resolve member meta for member_* envelopes */
  resolveMemberMeta?: (agentKey: string) => { agent_key: string; name: string; role: string };
  /**
   * H-03: Maximum time (ms) to wait for a runner_completion event after the
   * first streaming event arrives. If exceeded, the streaming session is
   * forcefully finalized to prevent messages from being stuck in 'streaming'
   * status indefinitely. Defaults to 10 minutes if not provided.
   */
  streamTimeoutMs?: number;
  /** Route activity envelopes to timeline handler */
  onActivityEnvelope?: (env: Envelope) => void;
};

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

/**
 * AF-GAP-04: Apply team member metadata to a reply Activity message.
 *
 * When the backend ActivityProjector detects a team member author, it sets
 * `Meta: { member_id: <author> }` on the Activity (see OnMemberMessageDelta
 * in internal/agent/activity_projector.go). The envelope builder surfaces
 * this as `metadata.meta.member_id`.
 *
 * This function mirrors the Legacy `member_message_start` handler's message
 * fields so the AF path produces identical team_member / origin / options_json
 * data, enabling the frontend to render member avatars and role badges without
 * relying on the Legacy member_message_* handlers.
 *
 * If `member_id` is absent or `resolveMemberMeta` is unavailable, the message
 * is left unchanged (coordinator or single-agent reply).
 */
function applyMemberMetaToMessage(
  msg: Message,
  md: Record<string, unknown>,
  ctx: StreamHandlerCtx,
): void {
  const metaObj = md.meta as Record<string, unknown> | undefined;
  const memberId = typeof metaObj?.member_id === 'string' ? metaObj.member_id : '';
  if (!memberId || !ctx.resolveMemberMeta) return;
  const memberMeta = ctx.resolveMemberMeta(memberId);
  msg.model_name = `team/${memberMeta.role || 'member'}`;
  msg.options_json = JSON.stringify({ team_member: memberMeta });
  msg.origin = { kind: 'team_member', agentKey: memberId };
  msg.team_member = { agent_id: '', name: memberMeta.name, role: memberMeta.role };
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

  /** Remove the "thinking" placeholder message created on run_status=running.
   * Called when real streaming content (activity_start) arrives. */
  function removeThinkingPlaceholder(sid: string) {
    const msgs = ctx.getMessages(sid);
    const idx = msgs.findIndex((m) => m.id.startsWith('run-') && m.model_name === 'thinking');
    if (idx >= 0) {
      writer?.flushSync();
      const updated = [...msgs];
      updated.splice(idx, 1);
      ctx.setMessages(sid, updated);
    }
  }

  // H-03: Stream timeout protection. If runner_completion never arrives
  // (e.g., WS disconnect), force-finalize all streaming/tool_running messages
  // after the timeout expires. The timer starts on the first streaming event
  // and is cleared when runner_completion or error arrives.
  const STREAM_TIMEOUT_MS = ctx.streamTimeoutMs ?? 10 * 60 * 1_000;
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
      // activity_start arrives.
      const sid = ctx.sessionId;
      const placeholderId = `run-${env.metadata?.run_id ?? 'pending'}`;
      const existing = ctx.getMessages(sid);
      if (!existing.some((m) => m.id === placeholderId)) {
        const placeholder: Message = {
          ...createPlaceholderMessage(placeholderId, sid, 'assistant', ''),
          status: 'streaming',
          model_name: 'thinking',
          origin: { kind: 'streaming', sessionId: sid },
        };
        writer?.flushSync();
        ctx.setMessages(sid, [...existing, placeholder]);
      }
    } else if (status === 'failed' || status === 'cancelled' || status === 'completed') {
      // Remove the thinking placeholder on terminal status.
      // Under normal flow, activity_start already removed it,
      // but guard against edge cases (e.g., empty LLM response).
      removeThinkingPlaceholder(ctx.sessionId);
    }
  });

  // T7.3d: Legacy member_message_start / member_delta / member_message_done
  // handlers removed. Team member messages are now handled exclusively by
  // the AF path: activity_start(kind=reply) with meta.member_id triggers
  // applyMemberMetaToMessage (see case 'reply' below), which sets the same
  // team_member / origin / options_json fields the Legacy handlers did.
  // The Legacy handlers are no longer needed and have been removed to
  // eliminate dual-path rendering bugs.

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

  // P1-7: run_heartbeat resets the run activity timer (prevents stale
  // detection) and forwards the envelope to the UI for progress rendering.
  stream.onType(
    'run_heartbeat',
    withSessionFilter(ctx, (env) => {
      ctx.onRunActivity?.();
      ctx.onHeartbeat?.(env);
    }),
  );

  // === Fallback handlers for raw error/runner_completion envelopes ===
  // In AF mode, the backend wraps errors into activity_start(kind=error) and
  // turn completion into activity_done(kind=task). However, if the backend
  // sends raw error/runner_completion envelopes (e.g., pre-AF fallback or
  // infrastructure errors), these handlers ensure the frontend doesn't get
  // stuck. They are intentionally lightweight — the Activity handlers above
  // handle the full lifecycle.

  stream.onType('runner_completion', async (env: Envelope) => {
    clearStreamTimeout();
    ctx.markSendingDone();
    writer?.flushSync();
    const sid = ctx.resolveActiveSessionId() ?? ctx.sessionId;
    if (!sid) return;
    const finalized = finalizeOrphanToolMessages(ctx.getMessages(sid));
    ctx.setMessages(sid, finalized);
    applySessionContextPatch(sid, env);
    if (shouldSessionWsSkipEnvelope(env)) return;
    try {
      await ctx.onReloadAfterCompletion(sid);
    } catch {
      /* caller may surface errors */
    }
  });

  stream.onType('error', async (env: Envelope) => {
    clearStreamTimeout();
    const errType = env.error?.type ?? '';
    if (errType.startsWith('flow_')) return;
    const msg = env.error?.message ?? 'stream failed';
    ctx.onErrorNotify(msg);
    const sid = ctx.resolveActiveSessionId() ?? ctx.sessionId;
    writer?.flushSync();
    if (sid) {
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
  });

  // === Activity Handlers ===
  // These handlers drive the messageStore via Activity envelopes.

  stream.onType('activity_start', (env: Envelope) => {
    if (shouldSessionWsSkipEnvelope(env)) return;
    const sid = env.session_id || ctx.sessionId;
    const active = ctx.resolveActiveSessionId();
    if (sid !== ctx.sessionId || (active && sid !== active)) return;
    const md = (env.metadata ?? {}) as Record<string, unknown>;
    const kind = String(md.kind ?? '');

    if (kind === 'task') {
      ctx.onRunActivity?.();
      ctx.onFirstByteArrived?.();

      // Remove the placeholder message created on run_status=running.
      // The real streaming message will be created by the Activity handlers below.
      removeThinkingPlaceholder(sid);
    }

    const rawActivityId = String(md.activity_id ?? '');
    if (!rawActivityId) return; // Guard: activity_id must be present
    const activityId = `actv-${rawActivityId}`;

    switch (kind) {
      case 'thinking': {
        const newMsg = createStreamingMessageFromActivity(activityId, sid, md as unknown as ActivityStartMeta);
        if (writer && sid === ctx.sessionId) {
          writer.update((cur) => [...cur, newMsg]);
        } else {
          const msgs = ctx.getMessages(sid);
          ctx.setMessages(sid, [...msgs, newMsg]);
        }
        startStreamTimeout();
        break;
      }
      case 'reply': {
        const newMsg = createStreamingMessageFromActivity(activityId, sid, md as unknown as ActivityStartMeta);
        // AF-GAP-04: When meta.member_id is present, this reply Activity was
        // produced by a team member (OnMemberMessageDelta/Done on the backend).
        // Apply the same team_member metadata as the Legacy member_message_start
        // handler so the frontend can distinguish member replies from the
        // coordinator's reply (avatar, role badge, options_json).
        applyMemberMetaToMessage(newMsg, md, ctx);
        if (writer && sid === ctx.sessionId) {
          writer.update((cur) => [...cur, newMsg]);
        } else {
          const msgs = ctx.getMessages(sid);
          ctx.setMessages(sid, [...msgs, newMsg]);
        }
        startStreamTimeout();
        ctx.onFirstByteArrived?.();
        break;
      }
      case 'action': {
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
          writer?.flushSync();
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

    ctx.onActivityEnvelope?.(env);
  });

  stream.onType('activity_delta', (env: Envelope) => {
    if (shouldSessionWsSkipEnvelope(env)) return;
    const sid = env.session_id || ctx.sessionId;
    const active = ctx.resolveActiveSessionId();
    if (sid !== ctx.sessionId || (active && sid !== active)) return;
    const md = (env.metadata ?? {}) as Record<string, unknown>;
    const field = String(md.delta_field ?? '');
    const chunk = String(md.delta_chunk ?? '');
    if (!chunk) return;

    const rawActivityId = String(md.activity_id ?? '');
    if (!rawActivityId) return; // Guard: activity_id must be present
    const activityId = `actv-${rawActivityId}`;

    ctx.onRunActivity?.();

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
