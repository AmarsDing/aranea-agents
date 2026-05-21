/**
 * Chat 域：网关 **`/v1/chat/*`**。
 * **`cmd/admin`**：`chat/v1` HTTP，由 admin 进程内 trpc-agent-go 运行时处理 Agent/Team 会话并持久化消息。
 * 流式事件通过 WebSocket + EventBus 推送，不再使用 SSE。
 *
 * Session 相关 API 请从 `features/session/api` 直接导入。
 */
import { createChatService } from "../../services";
import { asRecord, pickI32, pickStr } from "../../shared/wireJson";
import type {
  ChatOption,
  Message,
  SendMessageOptions,
  SendMessageResult,
  ToolUseEvent,
  IntentPassResult
} from "./types";

export type {
  ChatOption,
  Message,
  SendMessageOptions,
  SendMessageResult,
  ToolUseEvent,
  IntentPassResult
} from "./types";

const chatService = createChatService();

function wireInboundChatMessage(raw: unknown): Message {
  const r = asRecord(raw);
  if (raw == null || !Object.keys(r).length) {
    return {
      id: "",
      session_id: "",
      parent_message_id: "",
      turn_index: 0,
      role: "",
      content_markdown: "",
      model_name: "",
      token_in: 0,
      token_out: 0,
      latency_ms: 0,
      status: "",
      attachments_count: 0,
      options_json: "",
      error_message: "",
      created_at: ""
    };
  }
  return {
    id: pickStr(r, "id", "id"),
    session_id: pickStr(r, "session_id", "sessionId"),
    parent_message_id: pickStr(r, "parent_message_id", "parentMessageId"),
    turn_index: pickI32(r, "turn_index", "turnIndex"),
    role: pickStr(r, "role", "role"),
    content_markdown: pickStr(r, "content_markdown", "contentMarkdown"),
    model_name: pickStr(r, "model_name", "modelName"),
    token_in: pickI32(r, "token_in", "tokenIn"),
    token_out: pickI32(r, "token_out", "tokenOut"),
    latency_ms: pickI32(r, "latency_ms", "latencyMs"),
    status: pickStr(r, "status", "status"),
    attachments_count: pickI32(r, "attachments_count", "attachmentsCount"),
    options_json: pickStr(r, "options_json", "optionsJson"),
    error_message: pickStr(r, "error_message", "errorMessage"),
    created_at: pickStr(r, "created_at", "createdAt")
  };
}

export async function sendMessage(payload: {
  session_id: string;
  agent_key?: string;
  team_id?: string;
  content: string;
  options?: SendMessageOptions;
}): Promise<SendMessageResult> {
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
        }
      : undefined,
  });
  const d = asRecord(data);
  const um = d.userMessage ?? d.user_message;
  const am = d.agentMessage ?? d.agent_message;
  return {
    user_message: wireInboundChatMessage(um),
    agent_message: wireInboundChatMessage(am)
  };
}

export async function listChatOptions(type?: string): Promise<ChatOption[]> {
  const data = await chatService.GetChatOptions({ type });
  return (data.items ?? []).map((o) => ({
    type: o.type ?? "",
    key: o.key ?? "",
    label: o.label ?? "",
    enabled: Boolean(o.enabled),
    sort_order: o.sortOrder ?? 0,
    metadata_json: o.metadataJson ?? ""
  }));
}

export async function stopGeneration(sessionId: string): Promise<boolean> {
  try {
    const data = await chatService.StopGeneration({ sessionId });
    return !!data?.stopped;
  } catch {
    return false;
  }
}

export interface PendingMessage {
  id: string;
  content: string;
  status: string;
  created_at: string;
}

export async function getPendingMessages(sessionId: string): Promise<PendingMessage[]> {
  try {
    const data = await chatService.GetPendingMessages({ sessionId });
    return (data.items ?? []).map((item) => ({
      id: item.id ?? "",
      content: item.content ?? "",
      status: item.status ?? "",
      created_at: item.createdAt ?? "",
    }));
  } catch {
    return [];
  }
}

export async function cancelPendingMessage(sessionId: string, pendingId: string): Promise<boolean> {
  try {
    const data = await chatService.CancelPendingMessage({ sessionId, pendingId });
    return !!data?.cancelled;
  } catch {
    return false;
  }
}

export async function updatePendingMessage(
  sessionId: string,
  pendingId: string,
  content: string
): Promise<boolean> {
  try {
    const data = await chatService.UpdatePendingMessage({ sessionId, pendingId, content });
    return !!data?.updated;
  } catch {
    return false;
  }
}

export interface EnqueueUserMessageResult {
  accepted: boolean;
  queued: boolean;
  pendingId: string;
}

export async function enqueueUserMessage(sessionId: string, content: string): Promise<EnqueueUserMessageResult> {
  try {
    const data = await chatService.EnqueueUserMessage({ sessionId, content });
    return {
      accepted: !!data?.accepted,
      queued: !!data?.queued,
      pendingId: data?.pendingId ?? "",
    };
  } catch {
    return { accepted: false, queued: false, pendingId: "" };
  }
}

export type RunStatusValue = "idle" | "pending" | "running" | "awaiting_user" | "completed" | "failed" | "cancelled";

export interface RunStatus {
  runId: string;
  status: RunStatusValue;
  errorMessage: string;
  updatedAt: string;
  invocationId?: string;
  agentName?: string;
  startedAt?: string;
  lastEventAt?: string;
  eventCount?: number;
  awaitKind?: string;
  awaitToolKey?: string;
  awaitToolCallId?: string;
}

export async function getRunStatus(sessionId: string): Promise<RunStatus> {
  try {
    const data = await chatService.GetRunStatus({ sessionId });
    return {
      runId: data.runId ?? "",
      status: (data.status as RunStatusValue) ?? "idle",
      errorMessage: data.errorMessage ?? "",
      updatedAt: data.updatedAt ?? "",
      invocationId: data.invocationId ?? undefined,
      agentName: data.agentName ?? undefined,
      startedAt: data.startedAt ?? undefined,
      lastEventAt: data.lastEventAt ?? undefined,
      eventCount: data.eventCount ?? undefined,
      awaitKind: data.awaitKind ?? undefined,
      awaitToolKey: data.awaitToolKey ?? undefined,
      awaitToolCallId: data.awaitToolCallId ?? undefined,
    };
  } catch {
    return { runId: "", status: "idle", errorMessage: "", updatedAt: "" };
  }
}

export async function awaitUserReply(sessionId: string, reply: string, runId?: string): Promise<boolean> {
  try {
    const data = await chatService.AwaitUserReply({ sessionId, reply, runId });
    return !!data?.accepted;
  } catch {
    return false;
  }
}
