/**
 * Chat 域对外门面：`features/session/api`（Kratos）+ `legacyRest` + `./types`（遗留 REST `/api/v1/chat/*`，待 `chat/v1`）。
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

export type {
  ChatOption,
  Message,
  SendMessageOptions,
  SendMessageResult,
  SendMessageStreamCallbacks,
  ToolUseEvent
} from "./types";

export { listChatOptions, listMessages, sendMessage, sendMessageStream } from "./legacyRest";
