/**
 * Chat 域：网关 **`/v1/chat/*`**。
 * **`cmd/admin`**：`chat/v1` HTTP，由 admin 进程内 trpc-agent-go 运行时处理 Agent/Team 会话并持久化消息。
 * 流式事件通过 WebSocket + EventBus 推送，不再使用 SSE。
 *
 * Session 相关 API 请从 `features/session/api` 直接导入。
 */
import { createChatService } from '../../services';
import { asRecord, pickStr } from '../../shared/wireJson';
import type {
  ChatOption,
  SendMessageOptions,
  RunStatus,
  RunStatusValue,
  EnqueueUserMessageResult,
  ChatBackgroundJobRow,
  PendingMessage,
} from './types';
import type { MessageAck } from '../../realtime/command_channel';

export type {
  ChatOption,
  Message,
  SendMessageOptions,
  SendMessageResult,
  ToolUseEvent,
  RunStatus,
  RunStatusValue,
  EnqueueUserMessageResult,
  ChatBackgroundJobRow,
  PendingMessage,
} from './types';

const chatService = createChatService();

export class ChatApiError extends Error {
  constructor(
    message: string,
    readonly cause?: unknown,
  ) {
    super(message);
    this.name = 'ChatApiError';
  }
}

function wrapChatError(err: unknown, fallback: string): never {
  if (err instanceof ChatApiError) throw err;
  const message = err instanceof Error ? err.message : fallback;
  throw new ChatApiError(message, err);
}

/**
 * Send a chat message via the HTTP command channel (B2 channel separation).
 *
 * Calls `SubmitChatMessage` (async, ACK-only) — NOT `SendChatMessage` (sync,
 * full response). The backend starts the turn in a background goroutine and
 * returns immediately with a `MessageAck` (accepted/status only). Full
 * message/state/streaming data arrives via the WS data channel.
 *
 * The `messageId` and `turnId` in the ACK are empty on accept; they are
 * delivered later via WS events (`message.persisted`, `run_status=running`).
 */
export async function sendMessage(payload: {
  session_id: string;
  agent_key?: string;
  team_id?: string;
  content: string;
  // request_id 是提交幂等键（P3）：与 WS user_message 同一约定
  // （pending-user-<uuid>），重试复用，服务端按 session+request_id 去重。
  request_id?: string;
  options?: SendMessageOptions;
}): Promise<MessageAck> {
  try {
    const data = await chatService.SubmitChatMessage({
      sessionId: payload.session_id,
      agentKey: payload.agent_key,
      teamId: payload.team_id,
      content: payload.content,
      requestId: payload.request_id,
      options: payload.options
        ? {
            dialogMode: payload.options.dialog_mode,
            provider: payload.options.provider,
            model: payload.options.model,
            attachments: payload.options.attachments?.map((a) => ({ id: a.id })),
            knowledgeBases: payload.options.knowledge_bases,
          }
        : undefined,
    });
    // B2: ACK-only — messageId/turnId are empty on accept, delivered via WS events.
    return {
      messageId: data.messageId ?? '',
      turnId: data.turnId ?? '',
      status: (data.status as 'queued' | 'accepted') ?? (data.accepted ? 'accepted' : 'rejected'),
    };
  } catch (err) {
    wrapChatError(err, 'sendMessage failed');
  }
}

export async function listChatOptions(type?: string): Promise<ChatOption[]> {
  try {
    const data = await chatService.GetChatOptions({ type });
    return (data.items ?? []).map((o) => ({
      type: o.type ?? '',
      key: o.key ?? '',
      label: o.label ?? '',
      enabled: Boolean(o.enabled),
      sort_order: o.sortOrder ?? 0,
      metadata_json: o.metadataJson ?? '',
    }));
  } catch (err) {
    wrapChatError(err, 'listChatOptions failed');
  }
}

export async function stopGeneration(sessionId: string): Promise<boolean> {
  try {
    const data = await chatService.StopGeneration({ sessionId });
    return !!data?.stopped;
  } catch (err) {
    wrapChatError(err, 'stopGeneration failed');
  }
}

export async function getPendingMessages(sessionId: string): Promise<PendingMessage[]> {
  try {
    const data = await chatService.GetPendingMessages({ sessionId });
    return (data.items ?? []).map((item) => ({
      id: item.id ?? '',
      content: item.content ?? '',
      status: item.status ?? '',
      created_at: item.createdAt ?? '',
    }));
  } catch (err) {
    wrapChatError(err, 'getPendingMessages failed');
  }
}

export async function cancelPendingMessage(sessionId: string, pendingId: string): Promise<boolean> {
  try {
    const data = await chatService.CancelPendingMessage({ sessionId, pendingId });
    return !!data?.cancelled;
  } catch (err) {
    wrapChatError(err, 'cancelPendingMessage failed');
  }
}

export async function interruptAndSendMessage(sessionId: string, pendingEntryId: string): Promise<boolean> {
  try {
    const data = await chatService.InterruptAndSendMessage({ sessionId, pendingId: pendingEntryId });
    return !!data?.sent;
  } catch (err) {
    wrapChatError(err, 'interruptAndSendMessage failed');
  }
}

export async function updatePendingMessage(sessionId: string, pendingId: string, content: string): Promise<boolean> {
  try {
    const data = await chatService.UpdatePendingMessage({ sessionId, pendingId, content });
    return !!data?.updated;
  } catch (err) {
    wrapChatError(err, 'updatePendingMessage failed');
  }
}

/** @deprecated Use enqueueMessage */
export async function enqueueUserMessage(sessionId: string, content: string): Promise<EnqueueUserMessageResult> {
  return enqueueMessage(sessionId, content);
}

/** Unified enqueue entry (HTTP). WS enqueue_message is no longer used from the UI. */
export async function enqueueMessage(sessionId: string, content: string): Promise<EnqueueUserMessageResult> {
  try {
    const data = await chatService.EnqueueUserMessage({ sessionId, content });
    return {
      accepted: !!data?.accepted,
      queued: !!data?.queued,
      pendingId: data?.pendingId ?? '',
    };
  } catch (err) {
    wrapChatError(err, 'enqueueMessage failed');
  }
}

export async function getRunStatus(sessionId: string): Promise<RunStatus> {
  try {
    const data = await chatService.GetRunStatus({ sessionId });
    return {
      runId: data.runId ?? '',
      status: (data.status as RunStatusValue) ?? 'idle',
      errorMessage: data.errorMessage ?? '',
      updatedAt: data.updatedAt ?? '',
      invocationId: data.invocationId ?? undefined,
      agentName: data.agentName ?? undefined,
      startedAt: data.startedAt ?? undefined,
      lastEventAt: data.lastEventAt ?? undefined,
      eventCount: data.eventCount ?? undefined,
      awaitKind: data.awaitKind ?? undefined,
      awaitToolKey: data.awaitToolKey ?? undefined,
      awaitToolCallId: data.awaitToolCallId ?? undefined,
    };
  } catch (err) {
    wrapChatError(err, 'getRunStatus failed');
  }
}

export async function awaitUserReply(sessionId: string, reply: string, runId?: string): Promise<boolean> {
  try {
    const data = await chatService.AwaitUserReply({ sessionId, reply, runId });
    return !!data?.accepted;
  } catch (err) {
    wrapChatError(err, 'awaitUserReply failed');
  }
}

function wireChatBackgroundJob(raw: unknown): ChatBackgroundJobRow {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    source: pickStr(r, 'source', 'source'),
    session_id: pickStr(r, 'session_id', 'sessionId'),
    agent_id: pickStr(r, 'agent_id', 'agentId'),
    status: pickStr(r, 'status', 'status'),
    target_type: pickStr(r, 'target_type', 'targetType'),
    target_id: pickStr(r, 'target_id', 'targetId'),
    graph_id: pickStr(r, 'graph_id', 'graphId') || undefined,
    turn_id: pickStr(r, 'turn_id', 'turnId') || undefined,
    session_run_id: pickStr(r, 'session_run_id', 'sessionRunId') || undefined,
    phase: pickStr(r, 'phase', 'phase') || undefined,
    created_at: pickStr(r, 'created_at', 'createdAt'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
    summary: pickStr(r, 'summary', 'summary') || undefined,
    error_message: pickStr(r, 'error_message', 'errorMessage') || undefined,
    channel_id: pickStr(r, 'channel_id', 'channelId'),
  };
}

export async function submitMessageFeedback(payload: {
  session_id: string;
  message_id: string;
  rating: 'positive' | 'negative';
  comment?: string;
  /** P1-2: JSON snapshot {task_id,input,output} making the review list self-contained. */
  context_json?: string;
}): Promise<boolean> {
  try {
    const data = await chatService.SubmitMessageFeedback({
      messageId: payload.message_id,
      sessionId: payload.session_id,
      rating: payload.rating,
      comment: payload.comment,
      contextJson: payload.context_json,
    });
    return Boolean(data?.accepted);
  } catch (err) {
    wrapChatError(err, 'submitMessageFeedback failed');
  }
}

export async function listChatBackgroundJobs(opts: {
  sessionId?: string;
  agentId?: string;
  status?: string;
  limit?: number;
}): Promise<ChatBackgroundJobRow[]> {
  try {
    const data = await chatService.ListChatBackgroundJobs({
      sessionId: opts.sessionId,
      agentId: opts.agentId,
      status: opts.status,
      limit: opts.limit,
    });
    const items = data.items ?? [];
    return items.map(wireChatBackgroundJob);
  } catch (err) {
    wrapChatError(err, 'listChatBackgroundJobs failed');
  }
}

export async function cancelChatBackgroundJob(id: string, source: string): Promise<boolean> {
  try {
    const data = await chatService.CancelChatBackgroundJob({ id, source });
    return Boolean((data as { cancelled?: boolean })?.cancelled);
  } catch (err) {
    wrapChatError(err, 'cancelChatBackgroundJob failed');
    return false;
  }
}

/** Confirm a tool-blocked activity (approve or reject). */
export async function confirmActivity(sessionId: string, activityId: string, approved: boolean): Promise<boolean> {
  try {
    const data = await chatService.ConfirmActivity({
      sessionId,
      activityId,
      approved,
      // 空 reply 走后端 legacy approved 标志路径（resolveConfirmReply）。
      reply: '',
    });
    return Boolean(data?.accepted);
  } catch (err) {
    wrapChatError(err, 'confirmActivity failed');
  }
}

/** Confirm a tool-blocked activity with a structured grant scope. */
export async function confirmActivityGrant(payload: {
  sessionId: string;
  activityId: string;
  reply: string;
}): Promise<boolean> {
  try {
    const data = await chatService.ConfirmActivity({
      sessionId: payload.sessionId,
      activityId: payload.activityId,
      approved: true,
      reply: payload.reply,
    });
    return Boolean(data?.accepted);
  } catch (err) {
    wrapChatError(err, 'confirmActivityGrant failed');
  }
}

/** Submit clarification answers for a kind=clarify step (awaiting_input → completed). */
export async function submitClarification(payload: {
  sessionId: string;
  stepId: string;
  answers: Array<{ selected: string[]; other: string }>;
}): Promise<boolean> {
  try {
    const data = await chatService.SubmitClarification({
      sessionId: payload.sessionId,
      stepId: payload.stepId,
      answers: payload.answers,
    });
    return Boolean(data?.accepted);
  } catch (err) {
    wrapChatError(err, 'submitClarification failed');
  }
}
