import type { Envelope } from './envelope';
import type { UseEnvelopeStreamReturn } from './useEnvelopeStream';
import type { Message } from './types';
import type { Session } from '../session/types';
import { finalizeOrphanToolMessages } from './envelopeToolCall';
import { patchStreamingMessage } from './streamContentPatch';
import { createMessageBatchWriter } from './messageStoreBatch';
import { shouldSessionWsSkipEnvelope } from './inboundSyncRouting';
import { sessionContextPatchFromEnvelope, isSessionCompressNotice } from './sessionContextPatch';
import type { SessionContextPatch } from './sessionContextPatch';

/** H-03: Default stream timeout (ms) when ctx.streamTimeoutMs is not provided. */
const CHAT_STREAM_TIMEOUT_DEFAULT_MS = 10 * 60 * 1000;

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
  onReloadAfterCompletion: (sessionId: string) => Promise<void>;
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

  /** AF-correlation: 用后端 Activity 的 turn_id 回填 pending-user 占位消息的 turn_id。
   *
   * pending-user 占位消息创建时 turn_id=''（见 useChatSender.createPlaceholderMessage），
   * 而后端 Activity.TurnID = userMsg.ID（见 chat_orchestrator_durable.go:82,97）。
   * useConversationTimeline 通过 turn_id 将 Activity 记录关联到 UserTurn：
   *   turnId = userMessage?.turn_id || assistantMessage?.turn_id || ''
   * 若 pending-user 的 turn_id 始终为空，则 turnActivities=undefined，agentWork.activities=[]，
   * EventStream 的 v-if="agentWork.activities.length" 不渲染，思考和回复 UI 不显示。
   *
   * 此函数在 activity_start(kind=task) 到达时调用，用 md.turn_id 更新最新的 pending-user
   * 占位消息，使其与 Activity 记录的 turnId 匹配，完成关联闭环。 */
  function backfillPendingUserTurnId(sid: string, turnId: string) {
    if (!turnId) return;
    const msgs = ctx.getMessages(sid);
    const pendingIdx = msgs.findIndex((m) => m.id.startsWith('pending-user-') && m.turn_id === '');
    if (pendingIdx < 0) return;
    writer?.flushSync();
    const updated = [...msgs];
    updated[pendingIdx] = { ...updated[pendingIdx], turn_id: turnId };
    ctx.setMessages(sid, updated);
  }

  // H-03: Stream idle-timeout protection. If no streaming event arrives for
  // STREAM_TIMEOUT_MS (e.g., backend stuck, WS disconnect without reconnect),
  // force-finalize all streaming/tool_running messages. The timer resets on
  // each activity_delta so actively-streaming turns never time out (No-Timeout
  // principle). Terminal events (runner_completion/error/activity_done) clear it.
  const STREAM_TIMEOUT_MS = ctx.streamTimeoutMs ?? CHAT_STREAM_TIMEOUT_DEFAULT_MS;
  let streamTimeoutId: ReturnType<typeof setTimeout> | null = null;

  function resetStreamTimeout() {
    if (streamTimeoutId !== null) {
      clearTimeout(streamTimeoutId);
    }
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
  // the AF path: activity_start(kind=reply) with meta.member_id carries the
  // member metadata directly. The UI resolves member name/role from the
  // Activity record rather than reconstructing synthetic message rows.
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

  // run_heartbeat touches run activity for stall detection.
  stream.onType(
    'run_heartbeat',
    withSessionFilter(ctx, () => {
      ctx.onRunActivity?.();
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
  // Activity-First (AF): the backend projects runtime events into semantic
  // Activity units and pushes them via activity_start/delta/done/child_start.
  // The messageStore no longer reconstructs intermediate assistant/tool rows
  // from these envelopes; rendering is driven by useActivityTimeline, which
  // consumes the same envelopes through ctx.onActivityEnvelope.
  //
  // This block only retains control-flow side effects (placeholder cleanup,
  // pending-user turn_id backfill, error handling, completion reload).

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
      removeThinkingPlaceholder(sid);

      // AF-correlation: 用 md.turn_id 回填 pending-user 占位消息的 turn_id，
      // 使 useConversationTimeline 能将后续 Activity 记录关联到此 UserTurn。
      const turnId = typeof md.turn_id === 'string' ? md.turn_id : '';
      backfillPendingUserTurnId(sid, turnId);
    }

    if (kind === 'error') {
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
    }

    ctx.onActivityEnvelope?.(env);
  });

  stream.onType('activity_delta', (env: Envelope) => {
    if (shouldSessionWsSkipEnvelope(env)) return;
    const sid = env.session_id || ctx.sessionId;
    const active = ctx.resolveActiveSessionId();
    if (sid !== ctx.sessionId || (active && sid !== active)) return;
    const md = (env.metadata ?? {}) as Record<string, unknown>;

    // P3 fallback: backfill pending-user turn_id if activity_start(task) was
    // missed/lost. activity_delta metadata carries turn_id (see
    // buildActivityEnvelope). Without this, useConversationTimeline cannot
    // associate Activity records to UserTurn, causing thinking/reply UI to
    // not render.
    const turnId = String(md.turn_id ?? '');
    if (turnId) {
      backfillPendingUserTurnId(sid, turnId);
    }

    ctx.onRunActivity?.();
    // H-03: Reset idle timer on each delta so actively-streaming turns
    // never time out (No-Timeout principle).
    resetStreamTimeout();

    ctx.onActivityEnvelope?.(env);
  });

  stream.onType('activity_done', async (env: Envelope) => {
    if (shouldSessionWsSkipEnvelope(env)) return;
    const sid = env.session_id || ctx.sessionId;
    const active = ctx.resolveActiveSessionId();
    if (sid !== ctx.sessionId || (active && sid !== active)) return;
    const md = (env.metadata ?? {}) as Record<string, unknown>;
    const kind = String(md.kind ?? '');

    // P3 fallback: backfill pending-user turn_id if activity_start(task) was
    // missed/lost. activity_done metadata carries turn_id (see
    // buildActivityEnvelope).
    const turnId = String(md.turn_id ?? '');
    if (turnId) {
      backfillPendingUserTurnId(sid, turnId);
    }

    if (kind === 'task') {
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
        await ctx.onReloadAfterCompletion?.(sid);
      } catch {
        /* caller may surface errors */
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
