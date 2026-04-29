/**
 * 遗留 Chat REST：`legacyRestApi` + `/api/v1/chat/*`，待后端 `chat/v1` 迁入 Kratos 后替换实现。
 */
import { legacyRestApi as api } from "../../services/axiosHandler";
import { getBackendBaseURL } from "../../config/runtime";
import type {
  ChatOption,
  Message,
  SendMessageOptions,
  SendMessageResult,
  SendMessageStreamCallbacks,
  ToolUseEvent
} from "./types";

export async function listMessages(sessionID: string): Promise<Message[]> {
  const { data } = await api.get("/chat/messages", { params: { session_id: sessionID } });
  return data.items ?? [];
}

export async function sendMessage(payload: {
  session_id: string;
  agent_key?: string;
  team_id?: string;
  content: string;
  options?: SendMessageOptions;
}): Promise<SendMessageResult> {
  const { data } = await api.post("/chat/messages", payload);
  return data;
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
  const response = await fetch(`${getBackendBaseURL()}/chat/messages/stream`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
    signal: callbacks.signal
  });
  if (!response.ok || !response.body) {
    throw new Error((await response.text()) || `stream request failed: ${response.status}`);
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
    callbacks.onUserMessage?.(parsed as Message);
  } else if (event === "delta") {
    callbacks.onDelta?.(String(parsed.content ?? ""));
  } else if (event === "done") {
    callbacks.onDone?.((parsed as { agent_message?: Message }).agent_message as Message);
  } else if (event === "tool_event") {
    callbacks.onToolEvent?.(parsed as ToolUseEvent);
  } else if (event === "member_message_start") {
    callbacks.onMemberMessageStart?.(parsed as Message);
  } else if (event === "member_delta") {
    callbacks.onMemberDelta?.(String(parsed.message_id ?? ""), String(parsed.content ?? ""));
  } else if (event === "member_message_done") {
    callbacks.onMemberMessageDone?.((parsed as { agent_message?: Message }).agent_message as Message);
  } else if (event === "error") {
    throw new Error(String(parsed.message ?? "stream failed"));
  }
}

export async function listChatOptions(type?: string): Promise<ChatOption[]> {
  const { data } = await api.get("/chat/options", { params: type ? { type } : undefined });
  return data.items ?? [];
}
