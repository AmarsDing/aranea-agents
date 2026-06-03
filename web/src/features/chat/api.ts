/**
 * Chat 域：网关 **`/v1/chat/*`**。
 * **`cmd/admin`**：`chat/v1` HTTP，由 admin 进程内 trpc-agent-go 运行时处理 Agent/Team 会话并持久化消息。
 * 流式事件通过 WebSocket + EventBus 推送，不再使用 SSE。
 *
 * Session 相关 API 请从 `features/session/api` 直接导入。
 */
import { createChatService } from '../../services';
import { asRecord, pickI32, pickStr } from '../../shared/wireJson';
import type {
  ChatOption,
  Message,
  SendMessageOptions,
  SendMessageResult,
  ToolUseEvent,
  IntentPassResult,
  RunStatus,
  RunStatusValue,
  EnqueueUserMessageResult,
  ChatBackgroundJobRow,
  PendingMessage,
} from './types';
import { parseMessageOptions } from './parseMessageOptions';

export type {
  ChatOption,
  Message,
  SendMessageOptions,
  SendMessageResult,
  ToolUseEvent,
  IntentPassResult,
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

function wireInboundChatMessage(raw: unknown): Message {
  const r = asRecord(raw);
  if (raw == null || !Object.keys(r).length) {
    return {
      id: '',
      session_id: '',
      parent_message_id: '',
      turn_id: '',
      turn_number: 0,
      seq_in_turn: 0,
      role: '',
      content_markdown: '',
      model_name: '',
      token_in: 0,
      token_out: 0,
      latency_ms: 0,
      status: '',
      attachments_count: 0,
      options_json: '',
      error_message: '',
      created_at: '',
    };
  }
  const options_json = pickStr(r, 'options_json', 'optionsJson');
  return {
    id: pickStr(r, 'id', 'id'),
    session_id: pickStr(r, 'session_id', 'sessionId'),
    parent_message_id: pickStr(r, 'parent_message_id', 'parentMessageId'),
    turn_id: pickStr(r, 'turn_id', 'turnId'),
    turn_number: pickI32(r, 'turn_number', 'turnNumber'),
    seq_in_turn: pickI32(r, 'seq_in_turn', 'seqInTurn'),
    role: pickStr(r, 'role', 'role'),
    content_markdown: pickStr(r, 'content_markdown', 'contentMarkdown'),
    model_name: pickStr(r, 'model_name', 'modelName'),
    token_in: pickI32(r, 'token_in', 'tokenIn'),
    token_out: pickI32(r, 'token_out', 'tokenOut'),
    latency_ms: pickI32(r, 'latency_ms', 'latencyMs'),
    status: pickStr(r, 'status', 'status'),
    attachments_count: pickI32(r, 'attachments_count', 'attachmentsCount'),
    options_json,
    error_message: pickStr(r, 'error_message', 'errorMessage'),
    created_at: pickStr(r, 'created_at', 'createdAt'),
    ...parseMessageOptions(options_json),
  };
}

export async function sendMessage(payload: {
  session_id: string;
  agent_key?: string;
  team_id?: string;
  content: string;
  options?: SendMessageOptions;
}): Promise<SendMessageResult> {
  try {
    const data = await chatService.SendChatMessage({
      sessionId: payload.session_id,
      agentKey: payload.agent_key,
      teamId: payload.team_id,
      content: payload.content,
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
    const d = asRecord(data);
    const um = d.userMessage ?? d.user_message;
    const am = d.agentMessage ?? d.agent_message;
    return {
      user_message: wireInboundChatMessage(um),
      agent_message: wireInboundChatMessage(am),
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
  } catch {
    return false;
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
}): Promise<boolean> {
  try {
    const data = await chatService.SubmitMessageFeedback({
      messageId: payload.message_id,
      sessionId: payload.session_id,
      rating: payload.rating,
      comment: payload.comment,
    });
    return Boolean((data as { accepted?: boolean })?.accepted);
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
    const items = (data as { items?: unknown[] })?.items ?? [];
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
