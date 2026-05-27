/**
 * Chat store facade — re-exports the split stores and provides a backward-compatible
 * useChatStore that delegates to the three focused stores:
 *   - useChatSessionStore  (session CRUD, selection)
 *   - useChatMessageStore  (messages, revision, merge)
 *   - useChatRuntimeStore  (WS state, run status, stop)
 *
 * Existing consumers that import useChatStore continue to work.
 *
 * @deprecated New code should import the specific sub-store directly:
 *   - `import { useChatSessionStore } from "./sessionStore"`
 *   - `import { useChatMessageStore } from "./messageStore"`
 *   - `import { useChatRuntimeStore } from "./runtimeStore"`
 */
import { computed } from "vue";
import { defineStore } from "pinia";
import { useChatSessionStore, type TeamSessionRow } from "./sessionStore";
import { useChatMessageStore } from "./messageStore";
import { useChatRuntimeStore } from "./runtimeStore";
import type { Session } from "../../features/session/api";
import type { SessionContextPatch } from "../../features/chat/sessionContextPatch";
import type { IntentPassResult, Message, RunStatus } from "../../features/chat/types";
import type { ChatEntityKind } from "../../components/chat/types";

export type { TeamSessionRow };

export const useChatStore = defineStore("chat", () => {
  const session = useChatSessionStore();
  const message = useChatMessageStore();
  const runtime = useChatRuntimeStore();

  // --- Session delegates ---
  const entityKind = computed({
    get: () => session.entityKind,
    set: (kind: ChatEntityKind) => { session.entityKind = kind; },
  });
  const selectedTeamId = computed({
    get: () => session.selectedTeamId,
    set: (teamId: string | null) => { session.selectedTeamId = teamId; },
  });
  const teamSelectedSessionId = computed({
    get: () => session.teamSelectedSessionId,
    set: (sessionId: string | null) => { session.teamSelectedSessionId = sessionId; },
  });
  const sessions = computed({
    get: () => session.sessions,
    set: (rows: Session[]) => { session.sessions = rows; },
  });
  const selectedSession = computed({
    get: () => session.selectedSession,
    set: (row: Session | null) => { session.selectedSession = row; },
  });
  const teamSessions = computed({
    get: () => session.teamSessions,
    set: (rows: Record<string, TeamSessionRow[]>) => { session.teamSessions = rows; },
  });

  function currentSessionId() { return session.currentSessionId(); }
  function resetForAgentSwitch() { session.resetForAgentSwitch(); }
  function resetForTeamSwitch(teamId: string) { session.resetForTeamSwitch(teamId); }
  function loadAgentSessions(agentId: string, opts?: { refreshOnly?: boolean }) {
    return session.loadAgentSessions(agentId, opts);
  }
  function loadTeamSessions(teamId: string) { return session.loadTeamSessions(teamId); }
  function addAgentSession(
    agentId: string, title: string,
    options?: { dialog_mode?: string; default_provider?: string; default_model?: string }
  ) { return session.addAgentSession(agentId, title, options); }
  function addTeamSession(
    teamId: string, title: string,
    options?: { dialog_mode?: string; default_provider?: string; default_model?: string }
  ) { return session.addTeamSession(teamId, title, options); }
  async function removeSessionLocal(id: string) {
    await session.removeSessionLocal(id);
    message.deleteSessionMessages(id);
    runtime.deleteSessionRuntime(id);
  }
  async function removeTeamSessionLocal(teamId: string, sessionId: string) {
    await session.removeTeamSessionLocal(teamId, sessionId);
    message.deleteSessionMessages(sessionId);
    runtime.deleteSessionRuntime(sessionId);
  }
  function renameSessionLocal(id: string, title: string) { return session.renameSessionLocal(id, title); }
  function renameTeamSessionLocal(teamId: string, id: string, title: string) {
    return session.renameTeamSessionLocal(teamId, id, title);
  }
  function setSessionPinnedLocal(id: string, pinned: boolean) {
    return session.setSessionPinnedLocal(id, pinned);
  }
  function setTeamSessionPinnedLocal(teamId: string, id: string, pinned: boolean) {
    return session.setTeamSessionPinnedLocal(teamId, id, pinned);
  }
  async function clearAllAgentSessions(agentId: string) {
    const sessionIds = session.sessions.map((s) => s.id);
    await session.clearAllAgentSessions(agentId);
    for (const sid of sessionIds) {
      message.deleteSessionMessages(sid);
      runtime.deleteSessionRuntime(sid);
    }
  }
  function clearTeamSessions(teamId: string) {
    const sessionIds = (session.teamSessions[teamId] ?? []).map((s) => s.id);
    session.clearTeamSessions(teamId);
    for (const sid of sessionIds) {
      message.deleteSessionMessages(sid);
      runtime.deleteSessionRuntime(sid);
    }
  }
  function findSessionById(sessionId: string) { return session.findSessionById(sessionId); }
  function patchSessionMetricsLocal(sessionId: string, patch: SessionContextPatch) {
    session.patchSessionMetricsLocal(sessionId, patch);
  }

  // --- Message delegates ---
  const messagesBySession = computed(() => message.messagesBySession);
  const messages = computed({
    get: () => message.messages as Message[],
    set: (rows: Message[]) => { message.messages = rows; },
  });
  const sessionRevisionBySession = computed(() => message.sessionRevisionBySession);
  const lastIntentPass = computed(() => message.lastIntentPass);

  function getMessages(sessionId: string) { return message.getMessages(sessionId); }
  function setMessages(sessionId: string, rows: Message[]) { message.setMessages(sessionId, rows); }
  function clearSessionMessages(sessionId?: string) { message.clearSessionMessages(sessionId); }
  function clearTeamMessageCache() { message.clearAllMessages(); }
  function loadMessages(opts?: {
    sessionId?: string; replace?: boolean; afterRevision?: number; dropStaleInFlight?: boolean;
  }) { return message.loadMessages(opts); }

  // --- Runtime delegates ---
  const wsConnectedBySession = computed(() => runtime.wsConnectedBySession);
  const wsConnected = computed(() => runtime.wsConnected);

  function setWsConnected(sessionId: string, connected: boolean) {
    runtime.setWsConnected(sessionId, connected);
  }
  function fetchRunStatus(sessionId: string): Promise<RunStatus> {
    return runtime.fetchRunStatus(sessionId);
  }
  function submitAwaitReply(sessionId: string, reply: string, runId?: string): Promise<boolean> {
    return runtime.submitAwaitReply(sessionId, reply, runId);
  }
  function stop(sessionId: string) { return runtime.stop(sessionId); }

  return {
    // Session
    entityKind,
    selectedTeamId,
    teamSelectedSessionId,
    sessions,
    selectedSession,
    teamSessions,
    currentSessionId,
    resetForAgentSwitch,
    resetForTeamSwitch,
    loadAgentSessions,
    loadTeamSessions,
    addAgentSession,
    addTeamSession,
    removeSessionLocal,
    removeTeamSessionLocal,
    renameSessionLocal,
    renameTeamSessionLocal,
    setSessionPinnedLocal,
    setTeamSessionPinnedLocal,
    clearAllAgentSessions,
    clearTeamSessions,
    findSessionById,
    patchSessionMetricsLocal,
    // Message
    messagesBySession,
    messages,
    sessionRevisionBySession,
    lastIntentPass,
    getMessages,
    setMessages,
    clearSessionMessages,
    clearTeamMessageCache,
    loadMessages,
    // Runtime
    wsConnectedBySession,
    wsConnected,
    setWsConnected,
    fetchRunStatus,
    submitAwaitReply,
    stop,
  };
});

// Re-export sub-stores for direct access
export { useChatSessionStore } from "./sessionStore";
export { useChatMessageStore } from "./messageStore";
export { useChatRuntimeStore } from "./runtimeStore";
