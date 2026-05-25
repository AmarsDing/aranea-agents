/**
 * Chat session list store — manages session CRUD, selection, and team sessions.
 * Split from the monolithic useChatStore for single-responsibility.
 */
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
import { formatSessionTime } from "../../features/chat/composables/chatWorkspaceUtils";
import type { ChatEntityKind } from "../../components/chat/types";

export type TeamSessionRow = Session & { at: string };

function sessionListSortKey(session: Session): number {
  const pinned = session.pinned_at?.trim();
  if (pinned) {
    const t = new Date(pinned).getTime();
    if (Number.isFinite(t)) return t;
  }
  return 0;
}

function sortSessionsForDisplay(rows: Session[]): Session[] {
  return [...rows].sort((a, b) => {
    const pinDiff = sessionListSortKey(b) - sessionListSortKey(a);
    if (pinDiff !== 0) return pinDiff;
    const aMsg = new Date(a.last_message_at || a.updated_at || a.created_at).getTime();
    const bMsg = new Date(b.last_message_at || b.updated_at || b.created_at).getTime();
    if (aMsg !== bMsg) return bMsg - aMsg;
    return new Date(b.updated_at || b.created_at).getTime() - new Date(a.updated_at || a.created_at).getTime();
  });
}

function mergeSessionMetrics(session: Session, patch: SessionContextPatch): Session {
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
    sessions.value = sessions.value.filter((s) => s.id !== id);
    if (selectedSession.value?.id === id) {
      selectedSession.value = sessions.value[0] ?? null;
    }
  }

  async function removeTeamSessionLocal(teamId: string, sessionId: string) {
    await deleteSession(sessionId);
    teamSessions.value[teamId] = (teamSessions.value[teamId] ?? []).filter(
      (session) => session.id !== sessionId
    );
    if (teamSelectedSessionId.value === sessionId) {
      teamSelectedSessionId.value = teamSessions.value[teamId]?.[0]?.id ?? null;
    }
  }

  async function setSessionPinnedLocal(id: string, pinned: boolean) {
    const updated = pinned ? await pinSession(id) : await unpinSession(id);
    sessions.value = sortSessionsForDisplay(
      sessions.value.map((session) => (session.id === id ? updated : session))
    );
    if (selectedSession.value?.id === id) {
      selectedSession.value = updated;
    }
    return updated;
  }

  async function setTeamSessionPinnedLocal(teamId: string, id: string, pinned: boolean) {
    const updated = pinned ? await pinSession(id) : await unpinSession(id);
    teamSessions.value[teamId] = sortSessionsForDisplay(
      (teamSessions.value[teamId] ?? []).map((session) => (session.id === id ? updated : session))
    ).map(withTeamAt);
    return updated;
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
    await clearAgentSessions(agentId);
    sessions.value = [];
    selectedSession.value = null;
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
  };
});
