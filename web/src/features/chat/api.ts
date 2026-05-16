/**
 * Chat 域：网关 **`/v1/chat/*`**。
 * **`cmd/admin`**：`chat/v1` HTTP，由 admin 进程内 trpc-agent-go 运行时处理 Agent/Team 会话并持久化消息。
 * 流式事件通过 WebSocket + EventBus 推送，不再使用 SSE。
 *
 * Session 相关 API 请从 `features/session/api` 直接导入。
 */
import { kratosApi } from "../../services/axiosHandler";
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
  const { data } = await kratosApi.post("/v1/chat/messages", payload);
  const d = asRecord(data);
  const um = d.user_message ?? d.userMessage;
  const am = d.agent_message ?? d.agentMessage;
  return {
    user_message: wireInboundChatMessage(um),
    agent_message: wireInboundChatMessage(am)
  };
}

export async function listChatOptions(type?: string): Promise<ChatOption[]> {
  const { data } = await kratosApi.get("/v1/chat/options", { params: type ? { type } : undefined });
  return data.items ?? [];
}

export async function stopGeneration(sessionId: string): Promise<boolean> {
  try {
    const { data } = await kratosApi.post("/v1/chat/stop", { session_id: sessionId });
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
    const { data } = await kratosApi.get("/v1/chat/pending", { params: { session_id: sessionId } });
    return data.items ?? [];
  } catch {
    return [];
  }
}

export async function cancelPendingMessage(sessionId: string, pendingId: string): Promise<boolean> {
  try {
    const { data } = await kratosApi.post("/v1/chat/pending/cancel", {
      session_id: sessionId,
      pending_id: pendingId
    });
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
    const { data } = await kratosApi.post("/v1/chat/pending/update", {
      session_id: sessionId,
      pending_id: pendingId,
      content
    });
    return !!data?.updated;
  } catch {
    return false;
  }
}
