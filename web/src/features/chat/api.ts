/**
 * Chat 域：`features/session/api`（Kratos）+ `legacyRest`（`/v1/chat/*`：发送 / 流式 / options，经网关转发至遗留对话栈）。
 */
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

export { listChatOptions, sendMessage, sendMessageStream } from "./legacyRest";
