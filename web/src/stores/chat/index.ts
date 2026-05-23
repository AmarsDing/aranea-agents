import { computed, ref } from "vue";
import { defineStore } from "pinia";
import {
  clearAgentSessions,
  createSession,
  deleteSession,
  listSessionChatMessages as listMessages,
  listSessionChatMessagesAfterRevision,
  listSessions,
  listTeamSessions,
  updateSessionTitle,
  type Session,
} from "../../features/session/api";
import {
  awaitUserReply,
  getRunStatus,
  stopGeneration,
} from "../../features/chat/api";
import type {
  IntentPassResult,
  Message,
  RunStatus,
} from "../../features/chat/types";
import type { ChatEntityKind } from "../../components/chat/types";
import { mergeSessionMessages } from "../../features/chat/mergeSessionMessages";
import { formatSessionTime } from "../../features/chat/composables/chatWorkspaceUtils";

export type TeamSessionRow = Session & { at: string };

export const useChatStore = defineStore("chat", () => {
  const entityKind = ref<ChatEntityKind>("agent");
  const selectedTeamId = ref<string | null>(null);
  const teamSelectedSessionId = ref<string | null>(null);

  const sessions = ref<Session[]>([]);
  const selectedSession = ref<Session | null>(null);
  const teamSessions = ref<Record<string, TeamSessionRow[]>>({});

  const messagesBySession = ref<Record<string, Message[]>>({});
  const sessionRevisionBySession = ref<Record<string, number>>({});
  const lastIntentPass = ref<IntentPassResult | null>(null);
  const wsConnectedBySession = ref<Record<string, boolean>>({});

  function currentSessionId(): string | null {
    if (entityKind.value === "team") return teamSelectedSessionId.value;
    return selectedSession.value?.id ?? null;
  }

  /** Prefer `setMessages(sessionId, rows)` for explicit session targeting. */
  const messages = computed({
    get(): Message[] {
      const sid = currentSessionId();
      return sid ? (messagesBySession.value[sid] ?? []) : [];
    },
    set(rows: Message[]) {
      const sid = currentSessionId();
      if (sid) messagesBySession.value[sid] = rows;
    },
  });

  const wsConnected = computed(() => {
    const sid = currentSessionId();
    return sid ? (wsConnectedBySession.value[sid] ?? false) : false;
  });

  function getMessages(sessionId: string): Message[] {
    return messagesBySession.value[sessionId] ?? [];
  }

  function setMessages(sessionId: string, rows: Message[]) {
    messagesBySession.value[sessionId] = rows;
  }

  function setWsConnected(sessionId: string, connected: boolean) {
    wsConnectedBySession.value[sessionId] = connected;
  }

  function clearSessionMessages(sessionId?: string) {
    const sid = sessionId ?? currentSessionId();
    if (sid) messagesBySession.value[sid] = [];
  }

  function resetForAgentSwitch() {
    entityKind.value = "agent";
    selectedTeamId.value = null;
    teamSelectedSessionId.value = null;
  }

  function resetForTeamSwitch(teamId: string) {
    entityKind.value = "team";
    selectedTeamId.value = teamId;
    selectedSession.value = null;
    teamSelectedSessionId.value = null;
  }

  async function loadAgentSessions(agentId: string, opts?: { refreshOnly?: boolean }) {
    if (!agentId) return;
    const rows = await listSessions(agentId);
    sessions.value = rows;

    if (opts?.refreshOnly) {
      const currentId = selectedSession.value?.id;
      if (currentId) {
        const updated = rows.find((session) => session.id === currentId);
        if (updated) selectedSession.value = updated;
      }
      return;
    }

    const selectedID = selectedSession.value?.id;
    if (selectedID) {
      selectedSession.value =
        rows.find((session) => session.id === selectedID) ?? rows[0] ?? null;
    } else if (!selectedSession.value && rows.length > 0) {
      selectedSession.value = rows[0];
    }
  }

  async function loadTeamSessions(teamId: string) {
    const rows = await listTeamSessions(teamId);
    teamSessions.value[teamId] = rows.map((session) => ({
      ...session,
      at: formatSessionTime(session.last_message_at || session.updated_at || session.created_at),
    }));
  }

  async function loadMessages(opts?: {
    sessionId?: string;
    replace?: boolean;
    afterRevision?: number;
    dropStaleInFlight?: boolean;
  }) {
    const sid = opts?.sessionId ?? currentSessionId();
    if (!sid) return;

    const local = getMessages(sid);
    const mergeOpts = opts?.dropStaleInFlight ? { dropStaleInFlight: true } : undefined;

    if (opts?.afterRevision != null && opts.afterRevision > 0) {
      const { items, currentRevision } = await listSessionChatMessagesAfterRevision(
        sid,
        opts.afterRevision
      );
      sessionRevisionBySession.value[sid] = currentRevision;
      if (items.length > 0) {
        setMessages(sid, mergeSessionMessages(items, local, mergeOpts));
      }
      return;
    }

    const { items: server, currentRevision } = await listMessages(sid);
    sessionRevisionBySession.value[sid] = currentRevision;
    if (opts?.replace || local.length === 0) {
      setMessages(sid, mergeSessionMessages(server, [], mergeOpts));
      return;
    }
    setMessages(sid, mergeSessionMessages(server, local, mergeOpts));
  }

  async function addAgentSession(
    agentId: string,
    title: string,
    options?: { dialog_mode?: string; default_provider?: string; default_model?: string }
  ) {
    if (!agentId) return null;
    const created = await createSession({ agent_id: agentId, title, ...options });
    sessions.value.unshift(created);
    selectedSession.value = created;
    setMessages(created.id, []);
    return created;
  }

  async function addTeamSession(
    teamId: string,
    title: string,
    options?: { dialog_mode?: string; default_provider?: string; default_model?: string }
  ) {
    const created = await createSession({
      owner_type: "team",
      team_id: teamId,
      title,
      ...options,
    });
    teamSessions.value[teamId] = [
      {
        ...created,
        at: formatSessionTime(created.last_message_at || created.updated_at || created.created_at),
      },
      ...(teamSessions.value[teamId] ?? []),
    ];
    teamSelectedSessionId.value = created.id;
    setMessages(created.id, []);
    return created;
  }

  async function removeSessionLocal(id: string) {
    await deleteSession(id);
    sessions.value = sessions.value.filter((s) => s.id !== id);
    if (selectedSession.value?.id === id) {
      selectedSession.value = sessions.value[0] ?? null;
    }
    delete messagesBySession.value[id];
    delete sessionRevisionBySession.value[id];
    delete wsConnectedBySession.value[id];
  }

  async function removeTeamSessionLocal(teamId: string, sessionId: string) {
    await deleteSession(sessionId);
    teamSessions.value[teamId] = (teamSessions.value[teamId] ?? []).filter(
      (session) => session.id !== sessionId
    );
    delete messagesBySession.value[sessionId];
    delete sessionRevisionBySession.value[sessionId];
    delete wsConnectedBySession.value[sessionId];
    if (teamSelectedSessionId.value === sessionId) {
      teamSelectedSessionId.value = teamSessions.value[teamId]?.[0]?.id ?? null;
    }
  }

  async function renameSessionLocal(id: string, title: string) {
    const updated = await updateSessionTitle(id, title);
    sessions.value = sessions.value.map((session) => (session.id === id ? updated : session));
    if (selectedSession.value?.id === id) {
      selectedSession.value = updated;
    }
    return updated;
  }

  async function renameTeamSessionLocal(teamId: string, id: string, title: string) {
    const updated = await updateSessionTitle(id, title);
    teamSessions.value[teamId] = (teamSessions.value[teamId] ?? []).map((session) =>
      session.id === id
        ? {
            ...updated,
            at: formatSessionTime(updated.last_message_at || updated.updated_at || updated.created_at),
          }
        : session
    );
    return updated;
  }

  async function clearAllAgentSessions(agentId: string) {
    if (!agentId) return;
    const sessionIds = sessions.value.map((session) => session.id);
    await clearAgentSessions(agentId);
    sessions.value = [];
    selectedSession.value = null;
    for (const sid of sessionIds) {
      delete messagesBySession.value[sid];
      delete sessionRevisionBySession.value[sid];
      delete wsConnectedBySession.value[sid];
    }
  }

  function clearTeamSessions(teamId: string) {
    const sessionIds = (teamSessions.value[teamId] ?? []).map((session) => session.id);
    teamSessions.value[teamId] = [];
    teamSelectedSessionId.value = null;
    for (const sid of sessionIds) {
      delete messagesBySession.value[sid];
      delete sessionRevisionBySession.value[sid];
      delete wsConnectedBySession.value[sid];
    }
  }

  function clearTeamMessageCache() {
    for (const key of Object.keys(messagesBySession.value)) {
      delete messagesBySession.value[key];
    }
  }

  async function fetchRunStatus(sessionId: string): Promise<RunStatus> {
    return getRunStatus(sessionId);
  }

  async function submitAwaitReply(sessionId: string, reply: string, runId?: string): Promise<boolean> {
    return awaitUserReply(sessionId, reply, runId);
  }

  async function stop(sessionId: string) {
    return stopGeneration(sessionId);
  }

  function findSessionById(sessionId: string): Session | TeamSessionRow | undefined {
    const fromAgent = sessions.value.find((s) => s.id === sessionId);
    if (fromAgent) return fromAgent;
    if (selectedTeamId.value) {
      const fromTeam = teamSessions.value[selectedTeamId.value]?.find((s) => s.id === sessionId);
      if (fromTeam) return fromTeam;
    }
    for (const rows of Object.values(teamSessions.value)) {
      const hit = rows.find((s) => s.id === sessionId);
      if (hit) return hit;
    }
    return undefined;
  }

  return {
    entityKind,
    selectedTeamId,
    teamSelectedSessionId,
    sessions,
    selectedSession,
    teamSessions,
    messagesBySession,
    messages,
    sessionRevisionBySession,
    lastIntentPass,
    wsConnectedBySession,
    wsConnected,
    currentSessionId,
    getMessages,
    setMessages,
    setWsConnected,
    clearSessionMessages,
    resetForAgentSwitch,
    resetForTeamSwitch,
    loadAgentSessions,
    loadTeamSessions,
    loadMessages,
    addAgentSession,
    addTeamSession,
    removeSessionLocal,
    removeTeamSessionLocal,
    renameSessionLocal,
    renameTeamSessionLocal,
    clearAllAgentSessions,
    clearTeamSessions,
    clearTeamMessageCache,
    fetchRunStatus,
    submitAwaitReply,
    stop,
    findSessionById,
  };
});
