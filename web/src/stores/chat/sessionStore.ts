import { ref } from 'vue';
import { defineStore } from 'pinia';
import {
  archiveSession,
  clearAgentSessions,
  compactSession,
  createSession,
  deleteSession,
  getCompressStatus,
  getSession,
  listSessions,
  listTeamSessions,
  pinSession,
  restoreSession,
  unpinSession,
  updateSessionTitle,
} from '../../features/session/api';
import type { Session, CompactSessionResult, CompressStatus } from '../../features/session/types';
import type { SessionContextPatch } from '../../features/chat/sessionContextPatch';
import { reconcilePatchFromServer } from '../../features/chat/sessionContextPatch';
import { formatSessionTime } from '../../features/chat/composables/chatWorkspaceUtils';
import type { ChatEntityKind } from '../../components/chat/types';
import { emitSessionMutation, onSessionMutation } from '../sessionSync';
import { sortSessionsForDisplay } from '../../features/session/sessionSort';

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
  patch: SessionContextPatch,
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

export const useChatSessionStore = defineStore('chatSession', () => {
  const entityKind = ref<ChatEntityKind>('agent');
  const selectedTeamId = ref<string | null>(null);
  const teamSelectedSessionId = ref<string | null>(null);

  const sessions = ref<Session[]>([]);
  const selectedSession = ref<Session | null>(null);
  const teamSessions = ref<Record<string, TeamSessionRow[]>>({});
  const error = ref<string | null>(null);

  let _currentAgentId: string | null = null;

  onSessionMutation((mutation) => {
    switch (mutation.type) {
      case 'remove':
        removeSessionById(mutation.id);
        break;
      case 'archive':
        removeSessionById(mutation.id);
        break;
      case 'update':
        updateSessionById(mutation.id, mutation.session);
        break;
      case 'refresh':
        if (_currentAgentId) {
          loadAgentSessions(_currentAgentId, { refreshOnly: true });
        }
        break;
      case 'status_changed':
        patchSessionStatus(mutation.id, mutation.status, mutation.statusReason, mutation.statusChangedAt);
        break;
      case 'agent_removed':
        if (_currentAgentId === mutation.agentId) {
          resetForAgentSwitch();
        }
        break;
    }
  });

  function currentSessionId(): string | null {
    if (entityKind.value === 'team') return teamSelectedSessionId.value;
    return selectedSession.value?.id ?? null;
  }

  function resetForAgentSwitch() {
    entityKind.value = 'agent';
    selectedTeamId.value = null;
    teamSelectedSessionId.value = null;
  }

  function resetForTeamSwitch(teamId: string) {
    entityKind.value = 'team';
    selectedTeamId.value = teamId;
    selectedSession.value = null;
    teamSelectedSessionId.value = null;
  }

  // --- Unified entity-kind-aware methods ---

  /** Load sessions for the current entity kind (agent or team). */
  async function loadSessions(entityId: string, opts?: { refreshOnly?: boolean }) {
    if (entityKind.value === 'team') {
      await loadTeamSessions(entityId);
    } else {
      await loadAgentSessions(entityId, opts);
    }
  }

  /** Add a session for the current entity kind. */
  async function addSession(
    title: string,
    options?: { dialog_mode?: string; default_provider?: string; default_model?: string },
  ) {
    if (entityKind.value === 'team' && selectedTeamId.value) {
      return addTeamSession(selectedTeamId.value, title, options);
    }
    if (_currentAgentId) {
      return addAgentSession(_currentAgentId, title, options);
    }
    return null;
  }

  /** Remove a session for the current entity kind. */
  async function removeSessionByKind(sessionId: string) {
    if (entityKind.value === 'team' && selectedTeamId.value) {
      return removeTeamSessionLocal(selectedTeamId.value, sessionId);
    }
    return removeSessionLocal(sessionId);
  }

  /** Pin/unpin a session for the current entity kind. */
  async function setSessionPinnedByKind(sessionId: string, pinned: boolean) {
    if (entityKind.value === 'team' && selectedTeamId.value) {
      return setTeamSessionPinnedLocal(selectedTeamId.value, sessionId, pinned);
    }
    return setSessionPinnedLocal(sessionId, pinned);
  }

  /** Rename a session for the current entity kind. */
  async function renameSessionByKind(sessionId: string, title: string) {
    if (entityKind.value === 'team' && selectedTeamId.value) {
      return renameTeamSessionLocal(selectedTeamId.value, sessionId, title);
    }
    return renameSessionLocal(sessionId, title);
  }

  /** Clear all sessions for the current entity kind. */
  async function clearSessionsByKind() {
    if (entityKind.value === 'team' && selectedTeamId.value) {
      clearTeamSessions(selectedTeamId.value);
      return;
    }
    if (_currentAgentId) {
      await clearAllAgentSessions(_currentAgentId);
    }
  }

  // --- Original methods (kept for backward compat, prefer unified methods) ---

  async function loadAgentSessions(agentId: string, opts?: { refreshOnly?: boolean }) {
    if (!agentId) return;
    _currentAgentId = agentId;
    error.value = null;
    try {
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
        selectedSession.value = rows.find((session) => session.id === selectedID) ?? rows[0] ?? null;
      } else if (!selectedSession.value && rows.length > 0) {
        selectedSession.value = rows[0];
      }
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  async function loadTeamSessions(teamId: string) {
    error.value = null;
    try {
      const rows = await listTeamSessions(teamId);
      teamSessions.value[teamId] = sortSessionsForDisplay(rows).map(withTeamAt);
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  async function addAgentSession(
    agentId: string,
    title: string,
    options?: { dialog_mode?: string; default_provider?: string; default_model?: string },
  ) {
    if (!agentId) return null;
    error.value = null;
    try {
      const created = await createSession({ agent_id: agentId, title, ...options });
      sessions.value.unshift(created);
      selectedSession.value = created;
      emitSessionMutation({ type: 'update', id: created.id, session: created });
      return created;
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  async function addTeamSession(
    teamId: string,
    title: string,
    options?: { dialog_mode?: string; default_provider?: string; default_model?: string },
  ) {
    error.value = null;
    try {
      const created = await createSession({
        owner_type: 'team',
        team_id: teamId,
        title,
        ...options,
      });
      teamSessions.value[teamId] = [withTeamAt(created), ...(teamSessions.value[teamId] ?? [])];
      teamSelectedSessionId.value = created.id;
      emitSessionMutation({ type: 'update', id: created.id, session: created });
      return created;
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  async function removeSessionLocal(id: string) {
    error.value = null;
    try {
      await deleteSession(id);
      removeSessionById(id);
      emitSessionMutation({ type: 'remove', id });
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  async function removeTeamSessionLocal(teamId: string, sessionId: string) {
    error.value = null;
    try {
      await deleteSession(sessionId);
      teamSessions.value[teamId] = (teamSessions.value[teamId] ?? []).filter((session) => session.id !== sessionId);
      if (teamSelectedSessionId.value === sessionId) {
        teamSelectedSessionId.value = teamSessions.value[teamId]?.[0]?.id ?? null;
      }
      emitSessionMutation({ type: 'remove', id: sessionId });
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  async function setSessionPinnedLocal(id: string, pinned: boolean) {
    error.value = null;
    try {
      const updated = pinned ? await pinSession(id) : await unpinSession(id);
      updateSessionById(id, updated);
      emitSessionMutation({ type: 'update', id, session: updated });
      return updated;
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  async function setTeamSessionPinnedLocal(teamId: string, id: string, pinned: boolean) {
    error.value = null;
    try {
      const updated = pinned ? await pinSession(id) : await unpinSession(id);
      teamSessions.value[teamId] = sortSessionsForDisplay(
        (teamSessions.value[teamId] ?? []).map((session) => (session.id === id ? updated : session)),
      ).map(withTeamAt);
      emitSessionMutation({ type: 'update', id, session: updated });
      return updated;
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  async function renameSessionLocal(id: string, title: string) {
    error.value = null;
    try {
      const updated = await updateSessionTitle(id, title);
      updateSessionById(id, updated);
      emitSessionMutation({ type: 'update', id, session: updated });
      return updated;
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  async function renameTeamSessionLocal(teamId: string, id: string, title: string) {
    error.value = null;
    try {
      const updated = await updateSessionTitle(id, title);
      teamSessions.value[teamId] = (teamSessions.value[teamId] ?? []).map((session) =>
        session.id === id
          ? {
              ...updated,
              at: formatSessionTime(updated.last_message_at || updated.updated_at || updated.created_at),
            }
          : session,
      );
      emitSessionMutation({ type: 'update', id, session: updated });
      return updated;
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  async function clearAllAgentSessions(agentId: string) {
    if (!agentId) return;
    error.value = null;
    try {
      await clearAgentSessions(agentId);
      sessions.value = [];
      selectedSession.value = null;
      emitSessionMutation({ type: 'refresh' });
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
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

  function patchSessionStatus(sessionId: string, status: string, statusReason: string, statusChangedAt: string) {
    const id = sessionId.trim();
    if (!id) return;

    let changed = false;
    const next = sessions.value.map((session) => {
      if (session.id !== id) return session;
      changed = true;
      return {
        ...session,
        status: status as Session['status'],
        status_reason: statusReason as Session['status_reason'],
        status_changed_at: statusChangedAt,
      };
    });
    if (changed) sessions.value = next;

    let teamChanged = false;
    const out: Record<string, TeamSessionRow[]> = {};
    for (const [teamId, rows] of Object.entries(teamSessions.value)) {
      const nextRows = rows.map((session) => {
        if (session.id !== id) return session;
        teamChanged = true;
        return {
          ...session,
          status: status as Session['status'],
          status_reason: statusReason as Session['status_reason'],
          status_changed_at: statusChangedAt,
        };
      });
      out[teamId] = nextRows;
    }
    if (teamChanged) teamSessions.value = out;

    if (selectedSession.value?.id === id) {
      selectedSession.value = {
        ...selectedSession.value,
        status: status as Session['status'],
        status_reason: statusReason as Session['status_reason'],
        status_changed_at: statusChangedAt,
      };
    }
  }

  function reconcileFromServer(sessionId: string, serverSession: Session) {
    const id = sessionId.trim();
    if (!id) return;
    const local = findSessionById(id);
    const patch = reconcilePatchFromServer(
      serverSession,
      local
        ? {
            total_tokens: local.total_tokens,
            max_context_used_ratio: local.max_context_used_ratio,
            input_tokens: local.input_tokens,
            output_tokens: local.output_tokens,
            total_cost_micro_usd: local.total_cost_micro_usd,
            message_count: local.message_count,
            model_call_count: local.model_call_count,
            tool_call_count: local.tool_call_count,
            skill_call_count: local.skill_call_count,
            mcp_call_count: local.mcp_call_count,
            context_used_ratio: local.context_used_ratio,
            context_used_tokens: local.context_used_tokens ?? 0,
          }
        : undefined,
    );
    patchSessionMetricsLocal(id, patch);
  }

  async function fetchAndReconcileSession(sessionId: string): Promise<void> {
    const id = sessionId.trim();
    if (!id) return;
    try {
      const serverSession = await getSession(id);
      reconcileFromServer(id, serverSession);
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
    }
  }

  function removeSessionById(id: string) {
    sessions.value = sessions.value.filter((s) => s.id !== id);
    for (const teamId of Object.keys(teamSessions.value)) {
      teamSessions.value[teamId] = (teamSessions.value[teamId] ?? []).filter((session) => session.id !== id);
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
    sessions.value = sortSessionsForDisplay(sessions.value.map((session) => (session.id === id ? updated : session)));
    for (const teamId of Object.keys(teamSessions.value)) {
      teamSessions.value[teamId] = (teamSessions.value[teamId] ?? []).map((session) =>
        session.id === id ? withTeamAt(updated) : session,
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

  async function compactSessionAction(sessionId: string, preserveInstruction?: string): Promise<CompactSessionResult> {
    const result = await compactSession(sessionId, preserveInstruction);
    if (result.compacted) {
      await fetchAndReconcileSession(sessionId);
    }
    return result;
  }

  // --- Compress status ---
  const compressStatus = ref<CompressStatus>('normal');

  async function fetchCompressStatus(sessionId: string): Promise<CompressStatus> {
    try {
      const status = await getCompressStatus(sessionId);
      compressStatus.value = status;
      return status;
    } catch {
      return compressStatus.value;
    }
  }

  function resetCompressStatus() {
    compressStatus.value = 'normal';
  }

  async function archiveSessionLocal(id: string) {
    error.value = null;
    try {
      await archiveSession(id);
      removeSessionById(id);
      emitSessionMutation({ type: 'archive', id });
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  async function restoreSessionLocal(id: string) {
    error.value = null;
    try {
      const updated = await restoreSession(id);
      updateSessionById(id, updated);
      emitSessionMutation({ type: 'update', id, session: updated });
      return updated;
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  return {
    entityKind,
    selectedTeamId,
    teamSelectedSessionId,
    sessions,
    selectedSession,
    teamSessions,
    error,
    currentSessionId,
    resetForAgentSwitch,
    resetForTeamSwitch,
    // Unified entity-kind-aware methods (prefer these)
    loadSessions,
    addSession,
    removeSessionByKind,
    setSessionPinnedByKind,
    renameSessionByKind,
    clearSessionsByKind,
    // Original methods (for direct agent/team access)
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
    // Shared helpers
    findSessionById,
    patchSessionMetricsLocal,
    patchSessionStatus,
    reconcileFromServer,
    fetchAndReconcileSession,
    removeSessionById,
    updateSessionById,
    refreshFromAdmin,
    compactSessionAction,
    compressStatus,
    fetchCompressStatus,
    resetCompressStatus,
    archiveSessionLocal,
    restoreSessionLocal,
  };
});
