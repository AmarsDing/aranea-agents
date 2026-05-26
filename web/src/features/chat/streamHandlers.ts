import type { Envelope } from "./envelope";
import type { UseEnvelopeStreamReturn } from "./useEnvelopeStream";
import type { Message } from "./types";
import type { Session } from "../session/types";
import { dropPendingUserPlaceholders } from "./mergeSessionMessages";
import { upsertToolMessage, finalizeOrphanToolMessages } from "./envelopeToolCall";
import { patchStreamingMessage } from "./streamContentPatch";
import { createMessageBatchWriter } from "./messageStoreBatch";
import { shouldSessionWsSkipEnvelope } from "./inboundSyncRouting";
import { inferAssistantStreamTurnIndex, realignEphemeralTurnIndexes } from "./streamTurnIndex";
import type { IntentPassResult } from "./types";
import { sessionContextPatchFromEnvelope } from "./sessionContextPatch";
import type { SessionContextPatch } from "./sessionContextPatch";

export function createPlaceholderMessage(
  id: string,
  sessionID: string,
  role: string,
  content: string
): Message {
  return {
    id,
    session_id: sessionID,
    parent_message_id: "",
    turn_index: 1,
    role,
    content_markdown: content,
    model_name: "mock",
    token_in: 0,
    token_out: 0,
    latency_ms: 0,
    status: "ok",
    attachments_count: 0,
    options_json: "",
    error_message: "",
    created_at: new Date().toISOString(),
  };
}

export type StreamHandlerCtx = {
  sessionId: string;
  resolveActiveSessionId: () => string | null;
  getMessages: (sessionId: string) => Message[];
  setMessages: (sessionId: string, rows: Message[]) => void;
  markSendingDone: () => void;
  onRunStatus: (env: Envelope) => void;
  onErrorNotify: (message: string) => void;
  onOrchestrationNotice?: (message: string) => void;
  onReloadAfterCompletion: (sessionId: string) => Promise<void>;
  onSessionContextPatch?: (sessionId: string, patch: SessionContextPatch) => void;
  getSessionMetrics?: (
    sessionId: string
  ) => Pick<Session, "total_tokens" | "max_context_used_ratio" | "input_tokens" | "output_tokens"> | undefined;
  setLastIntentPass: (value: IntentPassResult | null) => void;
  onStreamingPatch?: (sessionId: string, patch: { reasoning?: string; partialText?: string; done?: boolean }) => void;
  /** Team-only: resolve member meta for member_* envelopes */
  resolveMemberMeta?: (agentKey: string) => { agent_key: string; name: string; role: string };
  streamIdPrefix?: string;
};

function streamRowId(ctx: StreamHandlerCtx, sessionId: string): string {
  const prefix = ctx.streamIdPrefix ?? "ws-stream";
  return `${prefix}-${sessionId}`;
}

function patchMessages(
  ctx: StreamHandlerCtx,
  sessionId: string,
  streamId: string,
  env: Envelope,
  isDone: boolean
) {
  ctx.setMessages(
    sessionId,
    patchStreamingEnvelope(ctx.getMessages(sessionId), sessionId, streamId, env, isDone)
  );
}

/** Shared streaming row patch for WS handlers and inbound sync. */
export function patchStreamingEnvelope(
  messages: Message[],
  sessionId: string,
  streamId: string,
  env: Envelope,
  isDone: boolean
): Message[] {
  const cur = messages;
  const exists = cur.some((m) => m.id === streamId);
  let next = cur;
  if (!exists) {
    const turnIndex = inferAssistantStreamTurnIndex(cur);
    next = [
      ...cur,
      {
        ...createPlaceholderMessage(streamId, sessionId, "assistant", ""),
        turn_index: turnIndex,
        status: "streaming",
      },
    ];
  }
  const patched = patchStreamingMessage(next, streamId, {
    text: isDone ? undefined : env.content?.text,
    reasoning: isDone ? undefined : env.content?.reasoning,
    replaceText: isDone ? env.content?.text : undefined,
    replaceReasoning: isDone ? env.content?.reasoning : undefined,
    status: isDone ? "ok" : "streaming",
  });
  return realignEphemeralTurnIndexes(patched);
}

export function bindStreamHandlers(
  stream: UseEnvelopeStreamReturn,
  ctx: StreamHandlerCtx,
  opts?: { batched?: boolean }
): void {
  const batched = opts?.batched ?? false;
  const writer = batched
    ? createMessageBatchWriter(
        () => ctx.getMessages(ctx.sessionId),
        (rows) => ctx.setMessages(ctx.sessionId, rows)
      )
    : null;

  function patch(sessionId: string, streamId: string, env: Envelope, isDone: boolean) {
    if (writer) {
      writer.update((cur) => patchStreamingEnvelope(cur, sessionId, streamId, env, isDone));
      return;
    }
    patchMessages(ctx, sessionId, streamId, env, isDone);
  }

  function applySessionContextPatch(sessionId: string, env: Envelope) {
    if (!ctx.onSessionContextPatch) return;
    const prev = ctx.getSessionMetrics?.(sessionId);
    const patch = sessionContextPatchFromEnvelope(env, prev);
    if (patch) {
      ctx.onSessionContextPatch(sessionId, patch);
    }
  }

  stream.onType("context_usage", (env: Envelope) => {
    if (shouldSessionWsSkipEnvelope(env)) return;
    const sid = env.session_id || ctx.sessionId;
    const active = ctx.resolveActiveSessionId();
    if (sid !== ctx.sessionId || (active && sid !== active)) return;
    applySessionContextPatch(sid, env);
  });

  stream.onType("text_delta", (env: Envelope) => {
    if (shouldSessionWsSkipEnvelope(env)) return;
    const sid = env.session_id || ctx.sessionId;
    const active = ctx.resolveActiveSessionId();
    if (sid !== ctx.sessionId || (active && sid !== active)) return;
    if (!env.content?.text && !env.content?.reasoning) return;
    ctx.onStreamingPatch?.(sid, {
      reasoning: env.content?.reasoning,
      partialText: env.content?.text,
    });
    patch(sid, streamRowId(ctx, sid), env, false);
  });

  stream.onType("text_done", (env: Envelope) => {
    if (shouldSessionWsSkipEnvelope(env)) return;
    const sid = env.session_id || ctx.sessionId;
    const active = ctx.resolveActiveSessionId();
    if (sid !== ctx.sessionId || (active && sid !== active)) return;
    ctx.onStreamingPatch?.(sid, {
      reasoning: env.content?.reasoning,
      partialText: env.content?.text,
      done: true,
    });
    patch(sid, streamRowId(ctx, sid), env, true);
    applySessionContextPatch(sid, env);
  });

  stream.onType("tool_call", (env: Envelope) => {
    if (shouldSessionWsSkipEnvelope(env)) return;
    const sid = env.session_id || ctx.sessionId;
    const active = ctx.resolveActiveSessionId();
    if (sid !== ctx.sessionId || (active && sid !== active) || !env.tool_call) return;
    if (writer) {
      writer.update((cur) => upsertToolMessage(cur, sid, env, "before"));
      return;
    }
    ctx.setMessages(sid, upsertToolMessage(ctx.getMessages(sid), sid, env, "before"));
  });

  stream.onType("tool_result", (env: Envelope) => {
    if (shouldSessionWsSkipEnvelope(env)) return;
    const sid = env.session_id || ctx.sessionId;
    const active = ctx.resolveActiveSessionId();
    if (sid !== ctx.sessionId || (active && sid !== active) || !env.tool_call) return;
    if (writer) {
      writer.update((cur) => upsertToolMessage(cur, sid, env, "after"));
      return;
    }
    ctx.setMessages(sid, upsertToolMessage(ctx.getMessages(sid), sid, env, "after"));
  });

  stream.onType("run_status", (env: Envelope) => {
    if (env.session_id && env.session_id !== ctx.sessionId) return;
    ctx.onRunStatus(env);
  });

  if (ctx.resolveMemberMeta) {
    stream.onType("member_message_start", (env: Envelope) => {
      const sid = ctx.resolveActiveSessionId();
      if (!sid || !env.author) return;
      const msgId = `member-${env.author}`;
      const cur = ctx.getMessages(sid);
      if (cur.some((m) => m.id === msgId)) return;
      const meta = ctx.resolveMemberMeta!(env.author);
      ctx.setMessages(sid, [
        ...cur,
        {
          ...createPlaceholderMessage(msgId, sid, "assistant", ""),
          status: "streaming",
          model_name: `team/${meta.role || "member"}`,
          options_json: JSON.stringify({ team_member: meta }),
        },
      ]);
    });

    stream.onType("member_delta", (env: Envelope) => {
      const sid = ctx.resolveActiveSessionId();
      if (!sid || !env.author) return;
      const msgId = `member-${env.author}`;
      ctx.setMessages(
        sid,
        patchStreamingMessage(ctx.getMessages(sid), msgId, {
          text: env.content?.text,
          reasoning: env.content?.reasoning,
        })
      );
    });

    stream.onType("member_message_done", (env: Envelope) => {
      const sid = ctx.resolveActiveSessionId();
      if (!sid || !env.author) return;
      const msgId = `member-${env.author}`;
      ctx.setMessages(
        sid,
        patchStreamingMessage(ctx.getMessages(sid), msgId, {
          replaceText: env.content?.text,
          replaceReasoning: env.content?.reasoning,
          status: "ok",
        })
      );
    });
  }

  stream.onType("runner_completion", async (env: Envelope) => {
    ctx.markSendingDone();
    writer?.flushSync();
    const sid = ctx.resolveActiveSessionId() ?? ctx.sessionId;
    if (!sid) return;
    const finalized = dropPendingUserPlaceholders(
      finalizeOrphanToolMessages(ctx.getMessages(sid))
    );
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
  });

  stream.onType("error", (env: Envelope) => {
    const errType = env.error?.type ?? "";
    if (errType.startsWith("flow_")) return;
    const hint = env.error?.hint?.trim();
    const msg = env.error?.message ?? "stream failed";
    ctx.onErrorNotify(hint ? `${msg} — ${hint}` : msg);
    const sid = ctx.resolveActiveSessionId() ?? ctx.sessionId;
    if (sid && env.request_id?.startsWith("pending-user-")) {
      ctx.setMessages(
        sid,
        ctx.getMessages(sid).filter((m) => m.id !== env.request_id)
      );
    }
    ctx.markSendingDone();
  });

  stream.onType("intent_pass", (env: Envelope) => {
    ctx.setLastIntentPass(env.metadata as IntentPassResult);
    const meta = env.metadata as Record<string, unknown> | undefined;
    const kind = typeof meta?.intent_kind === "string" ? meta.intent_kind : "";
    const target = typeof meta?.target_agent === "string" ? meta.target_agent : "";
    const parts = ["编排：意图识别"];
    if (kind) parts.push(kind);
    if (target) parts.push(`→ ${target}`);
    ctx.onOrchestrationNotice?.(parts.join(" · "));
  });

  stream.onType("transfer", (env: Envelope) => {
    const from = env.transfer?.from_agent?.trim() ?? "";
    const to = env.transfer?.to_agent?.trim() ?? "";
    if (!from && !to) return;
    ctx.onOrchestrationNotice?.(`编排：正在转接 ${from || "?"} → ${to || "?"}`);
  });
}
