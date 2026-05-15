/**
 * Chat 域：网关 **`/v1/chat/*`**。
 * **`cmd/admin`**：`chat/v1` HTTP + SSE，由 admin 进程内 trpc-agent-go 运行时处理 Agent/Team 会话并持久化消息。
 *
 * Session 相关 API 请从 `features/session/api` 直接导入。
 */
import { getBackendOrigin } from "../../config/runtime";
import { kratosApi } from "../../services/axiosHandler";
import { asRecord, pickI32, pickStr } from "../../shared/wireJson";
import type {
  ChatOption,
  Message,
  SendMessageOptions,
  SendMessageResult,
  SendMessageStreamCallbacks,
  ToolUseEvent,
  IntentPassResult
} from "./types";

export type {
  ChatOption,
  Message,
  SendMessageOptions,
  SendMessageResult,
  SendMessageStreamCallbacks,
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

export async function sendMessageStream(
  payload: {
    session_id: string;
    agent_key?: string;
    team_id?: string;
    content: string;
    options?: SendMessageOptions;
  },
  callbacks: SendMessageStreamCallbacks = {}
): Promise<void> {
  const response = await fetch(`${getBackendOrigin()}/v1/chat/messages/stream`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
    signal: callbacks.signal
  });
  if (!response.ok || !response.body) {
    let msg = await response.text();
    try {
      const j = asRecord(JSON.parse(msg) as unknown);
      const m = pickStr(j, "message", "message");
      if (m) msg = m;
    } catch {
      /* keep body */
    }
    throw new Error(msg.trim() || `stream request failed: ${response.status}`);
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const events = buffer.split(/\n\n/);
    buffer = events.pop() ?? "";
    for (const eventBlock of events) {
      handleStreamEvent(eventBlock, callbacks);
    }
  }
  if (buffer.trim()) {
    handleStreamEvent(buffer, callbacks);
  }
}

function handleStreamEvent(block: string, callbacks: SendMessageStreamCallbacks) {
  const lines = block.split(/\r?\n/);
  const event = lines.find((line) => line.startsWith("event:"))?.replace(/^event:\s*/, "").trim();
  const data = lines
    .filter((line) => line.startsWith("data:"))
    .map((line) => line.replace(/^data:\s*/, ""))
    .join("\n");
  if (!event || !data) return;
  const parsed = JSON.parse(data) as Record<string, unknown>;
  if (event === "user_message") {
    callbacks.onUserMessage?.(wireInboundChatMessage(parsed));
  } else if (event === "delta") {
    const c = String(parsed.content ?? "");
    const r = String(parsed.reasoning_content ?? parsed.reasoningContent ?? "");
    const deltaText = `${c}${r}`;
    callbacks.onDelta?.(deltaText);
  } else if (event === "done") {
    const am = parsed.agent_message ?? parsed.agentMessage;
    if (am != null && typeof am === "object") {
      const m = wireInboundChatMessage(am);
      if (String(m.id ?? "").trim()) {
        callbacks.onDone?.(m);
      }
    }
  } else if (event === "tool.call" || event === "tool_event") {
    if (event === "tool.call") {
      callbacks.onToolEvent?.({
        id: String(parsed.tool_call_id ?? ""),
        phase: "before",
        status: "running",
        agent_id: "",
        agent_key: "",
        agent_name: "",
        agent_icon: "",
        tool_name: String(parsed.tool_name ?? ""),
        tool_label: String(parsed.tool_name ?? ""),
        occurred_at: new Date().toISOString()
      });
    } else {
      callbacks.onToolEvent?.(parsed as ToolUseEvent);
    }
  } else if (event === "member_message_start") {
    callbacks.onMemberMessageStart?.(wireInboundChatMessage(parsed));
  } else if (event === "member_delta") {
    callbacks.onMemberDelta?.(String(parsed.message_id ?? parsed.messageId ?? ""), String(parsed.content ?? ""));
  } else if (event === "member_message_done") {
    const am = parsed.agent_message ?? parsed.agentMessage;
    callbacks.onMemberMessageDone?.(wireInboundChatMessage(am));
  } else if (event === "error") {
    throw new Error(String(parsed.message ?? "stream failed"));
  } else if (event === "intent_pass") {
    callbacks.onIntentPass?.(parsed as import("./types").IntentPassResult);
  }
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
