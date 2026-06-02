import type { Envelope } from "./envelope";
import type { UseEnvelopeStreamReturn } from "./useEnvelopeStream";
import type { Message } from "./types";
import type { Session } from "../session/types";
import { upsertToolMessage, finalizeOrphanToolMessages } from "./envelopeToolCall";
import { patchStreamingMessage } from "./streamContentPatch";
import { createMessageBatchWriter } from "./messageStoreBatch";
import { shouldSessionWsSkipEnvelope } from "./inboundSyncRouting";
import type { IntentPassResult } from "./types";
import { sessionContextPatchFromEnvelope, isSessionCompressNotice } from "./sessionContextPatch";
import type { SessionContextPatch } from "./sessionContextPatch";

import { originFromId } from "./messageOrigin";
import { formatErrorWithHint } from "./errorCodeHints";

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
    turn_id: "",
    turn_number: 0,
    seq_in_turn: 0,
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
  onReloadAfterCompletion: (sessionId: string) => Promise<void>;
  onSessionContextPatch?: (sessionId: string, patch: SessionContextPatch) => void;
  onCompressNotice?: (sessionId: string, prevRatio: number, newRatio: number) => void;
  getSessionMetrics?: (
    sessionId: string
  ) => Pick<Session, "total_tokens" | "max_context_used_ratio" | "input_tokens" | "output_tokens"> | undefined;
  setLastIntentPass: (value: IntentPassResult | null) => void;
  onStreamingPatch?: (sessionId: string, patch: { reasoning?: string; partialText?: string; done?: boolean }) => void;
  onRunActivity?: () => void;
  onFirstByteArrived?: () => void;
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

function markPendingUserFailed(messages: Message[], pendingId: string, errorMessage: string): Message[] {
  return messages.map((m) =>
    m.id === pendingId
      ? {
          ...m,
          status: "failed",
          error_message: errorMessage,
        }
      : m
  );
}

function latestPendingUserId(messages: Message[]): string {
  for (let i = messages.length - 1; i >= 0; i--) {
    const id = messages[i]?.id ?? "";
    if (id.startsWith("pending-user-")) return id;
  }
  return "";
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
    next = [
      ...cur,
      {
        ...createPlaceholderMessage(streamId, sessionId, "assistant", ""),
        status: "streaming",
      },
    ];
  }
  return patchStreamingMessage(next, streamId, {
    text: isDone ? undefined : env.content?.text,
    reasoning: isDone ? undefined : env.content?.reasoning,
    replaceText: isDone ? env.content?.text : undefined,
    replaceReasoning: isDone ? env.content?.reasoning : undefined,
    status: isDone ? "ok" : "streaming",
  });
}

function withSessionFilter(
  ctx: StreamHandlerCtx,
  handler: (env: Envelope, sid: string) => void
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

  stream.onType("context_usage", withSessionFilter(ctx, (_env, sid) => {
    applySessionContextPatch(sid, _env);
  }));

  stream.onType("text_delta", withSessionFilter(ctx, (env, sid) => {
    if (!env.content?.text && !env.content?.reasoning) return;
    ctx.onRunActivity?.();
    ctx.onFirstByteArrived?.();
    ctx.onStreamingPatch?.(sid, {
      reasoning: env.content?.reasoning,
      partialText: env.content?.text,
    });
    patch(sid, streamRowId(ctx, sid), env, false);
  }));

  stream.onType("text_done", withSessionFilter(ctx, (env, sid) => {
    ctx.onStreamingPatch?.(sid, {
      reasoning: env.content?.reasoning,
      partialText: env.content?.text,
      done: true,
    });
    patch(sid, streamRowId(ctx, sid), env, true);
    applySessionContextPatch(sid, env);
  }));

  stream.onType("tool_call", withSessionFilter(ctx, (env, sid) => {
    if (!env.tool_call) return;
    ctx.onRunActivity?.();
    if (writer) {
      writer.update((cur) => upsertToolMessage(cur, sid, env, "before"));
      return;
    }
    ctx.setMessages(sid, upsertToolMessage(ctx.getMessages(sid), sid, env, "before"));
  }));

  stream.onType("tool_result", withSessionFilter(ctx, (env, sid) => {
    if (!env.tool_call) return;
    if (writer) {
      writer.update((cur) => upsertToolMessage(cur, sid, env, "after"));
      return;
    }
    ctx.setMessages(sid, upsertToolMessage(ctx.getMessages(sid), sid, env, "after"));
  }));

  stream.onType("run_status", (env: Envelope) => {
    if (env.session_id && env.session_id !== ctx.sessionId) return;
    ctx.onRunActivity?.();
    ctx.onRunStatus(env);
    const status = String(env.metadata?.status ?? "");
    if (status === "running" || status === "accepted") {
      ctx.clearSendingTimeout?.();
      ctx.onRunAccepted?.();
    }
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
          origin: { kind: "team_member", agentKey: env.author },
          team_member: { agent_id: "", name: meta.name, role: meta.role },
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
    // Keep pending-user placeholders so the user message stays visible while
    // loadMessages fetches the persisted version.  The merge logic in
    // mergeSessionMessages will replace them with server-persisted rows.
    const finalized = finalizeOrphanToolMessages(ctx.getMessages(sid)).filter((m) => {
      const id = m.id || "";
      // Keep ws-stream rows that received text_done (status="ok") as fallback
      // until server-persisted messages arrive via loadMessages.
      if (id.startsWith("ws-stream-") || id.startsWith("ws-team-stream-")) {
        return m.status === "ok";
      }
      return true;
    });
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

  stream.onType("error", async (env: Envelope) => {
    const errType = env.error?.type ?? "";
    if (errType.startsWith("flow_")) return;
    const hint = env.error?.hint?.trim();
    const msg = env.error?.message ?? "stream failed";
    const errorCode = env.error?.code;
    ctx.onErrorNotify(hint ? `${formatErrorWithHint(msg, errorCode)} — ${hint}` : formatErrorWithHint(msg, errorCode));
    const sid = ctx.resolveActiveSessionId() ?? ctx.sessionId;
    writer?.flushSync();
    if (sid) {
      const pendingUserId = env.request_id?.startsWith("pending-user-")
        ? env.request_id
        : latestPendingUserId(ctx.getMessages(sid));
      if (pendingUserId) {
        ctx.setMessages(
          sid,
          markPendingUserFailed(ctx.getMessages(sid), pendingUserId, msg)
        );
      }
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
