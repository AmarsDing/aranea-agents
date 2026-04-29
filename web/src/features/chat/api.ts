/**
 * Chat 域对外 API 门面（vue-design：`features/<domain>/api.ts`）。
 *
 * - **会话（Kratos）**：`../session/api` → `createSessionService()` → `/v1/sessions`
 * - **消息 / SSE / options（遗留 REST）**：仍经 `clientLegacy` → `legacyRestApi` `/api/v1/chat/*`，待 `chat/v1`
 */
export {
  archiveSession,
  clearAgentSessions,
  createSession,
  deleteSession,
  getSession,
  getSessionTimeline,
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

export {
  listChatOptions,
  listMessages,
  sendMessage,
  sendMessageStream,
  type ChatOption,
  type Message,
  type SendMessageOptions,
  type SendMessageResult,
  type SendMessageStreamCallbacks,
  type ToolUseEvent
} from "../../services/clientLegacy";
