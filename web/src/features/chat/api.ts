/**
 * Chat 域：`features/session/api`（Kratos `session/v1`）+ 本文件网关 **`/v1/chat/*`**
 *（**`cmd/admin`**：**`chat/v1` HTTP** + SSE； unary / options / stream 在执行层仍经由 **`LEGACY_REST_ORIGIN`** 转发 **`/api/v1/chat/*`**，直至原生 chat Phase 完成）。
 */
import { getBackendOrigin } from "../../config/runtime";
import { kratosApi } from "../../services/axiosHandler";
import type {
  ChatOption,
  Message,
  SendMessageOptions,
  SendMessageResult,
  SendMessageStreamCallbacks,
  ToolUseEvent
} from "./types";

export {
  archiveSession,
  clearAgentSessions,
  createSession,
  deleteSession,
  getSession,
  getSessionTimeline,
  listSessionChatMessages,
  listSessionChatMessages as listMessages,
  listSessions,
  listTeamSessions,
  searchSessions,
  updateSessionTitle,
  type Session,
  type SessionListResult,
  type SessionSearchQuery,
  type SessionTimeline,
  type SessionTimelineItem,
  type SessionTimelineSummary
} from "../session/api";

export type {
  ChatOption,
  Message,
  SendMessageOptions,
  SendMessageResult,
  SendMessageStreamCallbacks,
  ToolUseEvent
} from "./types";

export async function sendMessage(payload: {
  session_id: string;
  agent_key?: string;
  team_id?: string;
  content: string;
  options?: SendMessageOptions;
}): Promise<SendMessageResult> {
  const { data } = await kratosApi.post("/v1/chat/messages", payload);
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
  const response = await fetch(`${getBackendOrigin()}/v1/chat/messages/stream`, {
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
  const { data } = await kratosApi.get("/v1/chat/options", { params: type ? { type } : undefined });
  return data.items ?? [];
}
