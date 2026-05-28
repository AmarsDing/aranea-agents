import { ref } from "vue";
import { defineStore } from "pinia";
import {
  clearAgentSessions,
  createSession,
  deleteSession,
  listSessions,
  listTeamSessions,
  pinSession,
  unpinSession,
  updateSessionTitle,
  type Session,
} from "../../features/session/api";
import type { SessionContextPatch } from "../../features/chat/sessionContextPatch";
import { reconcilePatchFromServer } from "../../features/chat/sessionContextPatch";
import { formatSessionTime } from "../../features/chat/composables/chatWorkspaceUtils";
import type { ChatEntityKind } from "../../components/chat/types";
import { emitSessionMutation, onSessionMutation } from "../sessionSync";
import { sortSessionsForDisplay } from "../../features/session/sessionSort";

export type TeamSessionRow = Session & { at: string };

function mergeSessionMetrics<T extends Session>(session: T, patch: SessionContextPatch): T {
  return {
    ...session,
    ...patch,
  };
}

function patchSessionInList(rows: Session[], sessionId: string, patch: SessionContextPatch): Session[] {
  let changed = false;
  const next = rows.map((session) => {
    if (session.id !== sessionId) return session;
    changed = true;
    return mergeSessionMetrics(session, patch);
  });
  return changed ? next : rows;
}

function patchTeamSessionInMap(
  map: Record<string, TeamSessionRow[]>,
  sessionId: string,
  patch: SessionContextPatch
): Record<string, TeamSessionRow[]> {
  let changed = false;
  const out: Record<string, TeamSessionRow[]> = {};
  for (const [teamId, rows] of Object.entries(map)) {
    const nextRows = rows.map((session) => {
      if (session.id !== sessionId) return session;
      changed = true;
      return mergeSessionMetrics(session, patch);
    });
    out[teamId] = nextRows;
  }
  return changed ? out : map;
}

function withTeamAt(session: Session): TeamSessionRow {
  return {
    ...session,
    at: formatSessionTime(session.last_message_at || session.updated_at || session.created_at),
  };
}

export const useChatSessionStore = defineStore("chatSession", () => {
  const entityKind = ref<ChatEntityKind>("agent");
  const selectedTeamId = ref<string | null>(null);
  const teamSelectedSessionId = ref<string | null>(null);

  const sessions = ref<Session[]>([]);
  const selectedSession = ref<Session | null>(null);
  const teamSessions = ref<Record<string, TeamSessionRow[]>>({});

  let _currentAgentId: string | null = null;

  onSessionMutation((mutation) => {
    switch (mutation.type) {
      case "remove":
        removeSessionById(mutation.id);
        break;
      case "archive":
        removeSessionById(mutation.id);
        break;
      case "update":
        updateSessionById(mutation.id, mutation.session);
        break;
      case "refresh":
        if (_currentAgentId) {
          loadAgentSessions(_currentAgentId, { refreshOnly: true });
        }
        break;
    }
  });

  function currentSessionId(): string | null {
    if (entityKind.value === "team") return teamSelectedSessionId.value;
    return selectedSession.value?.id ?? null;
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
    _currentAgentId = agentId;
    const rows = await listSessions(agentId);
    sessions.value = sortSessionsForDisplay(rows);

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
    teamSessions.value[teamId] = sortSessionsForDisplay(rows).map(withTeamAt);
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
      withTeamAt(created),
      ...(teamSessions.value[teamId] ?? []),
    ];
    teamSelectedSessionId.value = created.id;
    return created;
  }

  async function removeSessionLocal(id: string) {
    await deleteSession(id);
    removeSessionById(id);
    emitSessionMutation({ type: "remove", id });
  }

  async function removeTeamSessionLocal(teamId: string, sessionId: string) {
    await deleteSession(sessionId);
    teamSessions.value[teamId] = (teamSessions.value[teamId] ?? []).filter(
      (session) => session.id !== sessionId
    );
    if (teamSelectedSessionId.value === sessionId) {
      teamSelectedSessionId.value = teamSessions.value[teamId]?.[0]?.id ?? null;
    }
    emitSessionMutation({ type: "remove", id: sessionId });
  }

  async function setSessionPinnedLocal(id: string, pinned: boolean) {
    const updated = pinned ? await pinSession(id) : await unpinSession(id);
    updateSessionById(id, updated);
    emitSessionMutation({ type: "update", id, session: updated });
    return updated;
  }

  async function setTeamSessionPinnedLocal(teamId: string, id: string, pinned: boolean) {
    const updated = pinned ? await pinSession(id) : await unpinSession(id);
    teamSessions.value[teamId] = sortSessionsForDisplay(
      (teamSessions.value[teamId] ?? []).map((session) => (session.id === id ? updated : session))
    ).map(withTeamAt);
    emitSessionMutation({ type: "update", id, session: updated });
    return updated;
  }

  async function renameSessionLocal(id: string, title: string) {
    const updated = await updateSessionTitle(id, title);
    updateSessionById(id, updated);
    emitSessionMutation({ type: "update", id, session: updated });
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
    emitSessionMutation({ type: "update", id, session: updated });
    return updated;
  }

  async function clearAllAgentSessions(agentId: string) {
    if (!agentId) return;
    await clearAgentSessions(agentId);
    sessions.value = [];
    selectedSession.value = null;
    emitSessionMutation({ type: "refresh" });
  }

  function clearTeamSessions(teamId: string) {
    teamSessions.value[teamId] = [];
    teamSelectedSessionId.value = null;
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

  function patchSessionMetricsLocal(sessionId: string, patch: SessionContextPatch) {
    const id = sessionId.trim();
    if (!id || !Object.keys(patch).length) return;

    sessions.value = patchSessionInList(sessions.value, id, patch);
    teamSessions.value = patchTeamSessionInMap(teamSessions.value, id, patch);

    if (selectedSession.value?.id === id) {
      selectedSession.value = mergeSessionMetrics(selectedSession.value, patch);
    }
  }

  function reconcileFromServer(sessionId: string, serverSession: Session) {
    const id = sessionId.trim();
    if (!id) return;
    const patch = reconcilePatchFromServer(serverSession);
    patchSessionMetricsLocal(id, patch);
  }

  function removeSessionById(id: string) {
    sessions.value = sessions.value.filter((s) => s.id !== id);
    for (const teamId of Object.keys(teamSessions.value)) {
      teamSessions.value[teamId] = (teamSessions.value[teamId] ?? []).filter(
        (session) => session.id !== id
      );
    }
    if (selectedSession.value?.id === id) {
      selectedSession.value = sessions.value[0] ?? null;
    }
    if (teamSelectedSessionId.value === id) {
      const teamId = selectedTeamId.value;
      if (teamId) {
        teamSelectedSessionId.value = teamSessions.value[teamId]?.[0]?.id ?? null;
      }
    }
  }

  function updateSessionById(id: string, updated: Session) {
    sessions.value = sortSessionsForDisplay(
      sessions.value.map((session) => (session.id === id ? updated : session))
    );
    for (const teamId of Object.keys(teamSessions.value)) {
      teamSessions.value[teamId] = (teamSessions.value[teamId] ?? []).map((session) =>
        session.id === id ? withTeamAt(updated) : session
      );
    }
    if (selectedSession.value?.id === id) {
      selectedSession.value = updated;
    }
  }

  function refreshFromAdmin() {
    if (_currentAgentId) {
      loadAgentSessions(_currentAgentId, { refreshOnly: true });
    }
  }

  return {
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
    reconcileFromServer,
    removeSessionById,
    updateSessionById,
    refreshFromAdmin,
  };
});
