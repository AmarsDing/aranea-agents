import { computed, ref } from 'vue';
import { defineStore } from 'pinia';
import {
  archiveSession,
  clearAgentSessions,
  compactSession,
  createSession,
  deleteSession,
  getCompressStatus,
  getSession,
  listTeamSessions,
  pinSession,
  restoreSession,
  searchSessions,
  unpinSession,
  updateSessionTitle,
} from '../../features/session/api';
import type { Session, CompactSessionResult, CompressStatus } from '../../features/session/types';
import type { SessionContextPatch } from '../../features/chat/sessionContextPatch';
import { reconcilePatchFromServer } from '../../features/chat/sessionContextPatch';
import { formatSessionTime } from '../../features/chat/composables/chatWorkspaceUtils';
import type { ChatEntityKind } from '../../components/chat/types';
import { emitSessionMutation, onSessionMutation } from '../sessionMutationBus';
import { sortSessionsForDisplay } from '../../features/session/sessionSort';
import { CHAT_SESSION_MAX_LIMIT, CHAT_SESSION_PAGE_SIZE } from '../../features/constants/queryLimits';

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

  // ── 会话列表分页（agent 侧栏滚动加载）─────────────────────────────
  // total 来自服务端；listOffset 追踪已消费的服务端行数（含去重丢弃的），
  // 与 sessions.length 可能因本地新建/去重而短暂不一致，hasMore 以 offset 为准。
  const sessionsTotal = ref(0);
  const sessionsLoading = ref(false);
  const sessionsLoadingMore = ref(false);
  const sessionListKeyword = ref('');
  const listOffset = ref(0);
  const sessionsHasMore = computed(() => listOffset.value < sessionsTotal.value);

  let _currentAgentId: string | null = null;
  // P0 #4: 异步请求代际令牌。每次 resetForXxxSwitch 递增，使在途的
  // loadAgentSessions/loadTeamSessions 在 await 返回后能识别自己已过期，
  // 避免快速切换 Agent↔Team 时旧请求污染 selectedSession / teamSessions。
  let _loadGeneration = 0;

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
    _loadGeneration++; // P0 #4: 作废所有在途的 loadTeamSessions 请求
    entityKind.value = 'agent';
    selectedTeamId.value = null;
    teamSelectedSessionId.value = null;
  }

  function resetForTeamSwitch(teamId: string) {
    _loadGeneration++; // P0 #4: 作废所有在途的 loadAgentSessions 请求
    entityKind.value = 'team';
    selectedTeamId.value = teamId;
    selectedSession.value = null;
    teamSelectedSessionId.value = null;
    // team 列表一次性加载，不展示 agent 列表的分页/搜索状态
    sessionsTotal.value = 0;
    listOffset.value = 0;
    sessionListKeyword.value = '';
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
    // 切换 agent 时清空搜索词，避免把上一个 agent 的过滤条件带过来
    if (agentId !== _currentAgentId) {
      sessionListKeyword.value = '';
    }
    _currentAgentId = agentId;
    error.value = null;
    // P0 #4: 捕获当前代际，await 后校验。若用户在请求在途时切换了 entityKind，
    // 该请求视为过期，不修改 selectedSession / sessions，避免竞态污染。
    const myGeneration = _loadGeneration;
    const keyword = sessionListKeyword.value.trim() || undefined;
    try {
      if (opts?.refreshOnly) {
        // 刷新已加载窗口：一次性取回覆盖当前已加载条数的首页，按 id 合并，
        // 保留已分页加载的更旧会话，避免 refresh 事件把列表截回第一页。
        const windowLimit = Math.min(Math.max(sessions.value.length, CHAT_SESSION_PAGE_SIZE), CHAT_SESSION_MAX_LIMIT);
        const result = await searchSessions({
          agent_id: agentId,
          root_only: true,
          keyword,
          limit: windowLimit,
          offset: 0,
        });
        if (myGeneration !== _loadGeneration) return; // 已被后续切换作废
        const merged = new Map(sessions.value.map((session) => [session.id, session]));
        for (const row of result.items) {
          // context_budget is a WS-only push field (context_usage meta, never in
          // REST rows): keep the locally-patched value so the SpiritStatusBar
          // breakdown survives the post-turn session reload.
          const local = merged.get(row.id);
          merged.set(row.id, local?.context_budget ? { ...row, context_budget: local.context_budget } : row);
        }
        sessions.value = sortSessionsForDisplay([...merged.values()]);
        sessionsTotal.value = result.total;
        listOffset.value = Math.max(listOffset.value, result.items.length);

        const currentId = selectedSession.value?.id;
        if (currentId) {
          const updated = sessions.value.find((session) => session.id === currentId);
          if (updated) selectedSession.value = updated;
        }
        return;
      }

      sessionsLoading.value = true;
      const result = await searchSessions({
        agent_id: agentId,
        root_only: true,
        keyword,
        limit: CHAT_SESSION_PAGE_SIZE,
        offset: 0,
      });
      if (myGeneration !== _loadGeneration) return; // 已被后续切换作废
      sessions.value = sortSessionsForDisplay(result.items);
      sessionsTotal.value = result.total;
      listOffset.value = result.items.length;

      const selectedID = selectedSession.value?.id;
      if (selectedID) {
        // 选中项不在首页（较旧的会话）时保留原选中，不强制跳到最新
        const found = result.items.find((session) => session.id === selectedID);
        if (found) selectedSession.value = found;
      } else if (result.items.length > 0) {
        selectedSession.value = result.items[0];
      }
    } catch (e: unknown) {
      if (myGeneration !== _loadGeneration) return; // 过期请求的错误不污染 error 状态
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      if (myGeneration === _loadGeneration) {
        sessionsLoading.value = false;
      }
    }
  }

  /** 加载下一页会话（滚动到底触发）。仅 agent 列表分页；team 一次性加载。 */
  async function loadMoreAgentSessions() {
    const agentId = _currentAgentId;
    if (!agentId || entityKind.value !== 'agent') return;
    if (sessionsLoading.value || sessionsLoadingMore.value || !sessionsHasMore.value) return;
    error.value = null;
    const myGeneration = _loadGeneration;
    sessionsLoadingMore.value = true;
    try {
      const result = await searchSessions({
        agent_id: agentId,
        root_only: true,
        keyword: sessionListKeyword.value.trim() || undefined,
        limit: CHAT_SESSION_PAGE_SIZE,
        offset: listOffset.value,
      });
      if (myGeneration !== _loadGeneration) return;
      // 并发删除可能导致空页：直接视为已到底，避免反复触发空请求
      listOffset.value = result.items.length > 0 ? listOffset.value + result.items.length : result.total;
      sessionsTotal.value = result.total;
      const existing = new Set(sessions.value.map((session) => session.id));
      const fresh = result.items.filter((session) => !existing.has(session.id));
      if (fresh.length > 0) {
        sessions.value = sortSessionsForDisplay([...sessions.value, ...fresh]);
      }
    } catch (e: unknown) {
      if (myGeneration !== _loadGeneration) return;
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      if (myGeneration === _loadGeneration) {
        sessionsLoadingMore.value = false;
      }
    }
  }

  /** 按关键词过滤当前 agent 的会话列表（服务端过滤，重置分页）。 */
  async function searchAgentSessions(keyword: string) {
    sessionListKeyword.value = keyword;
    if (_currentAgentId && entityKind.value === 'agent') {
      await loadAgentSessions(_currentAgentId);
    }
  }

  async function loadTeamSessions(teamId: string) {
    error.value = null;
    const myGeneration = _loadGeneration;
    try {
      const rows = await listTeamSessions(teamId);
      if (myGeneration !== _loadGeneration) return; // P0 #4: 已被后续切换作废
      const prevById = new Map((teamSessions.value[teamId] ?? []).map((s) => [s.id, s]));
      teamSessions.value[teamId] = sortSessionsForDisplay(rows).map((row) => {
        // Preserve the WS-only context_budget across list reloads (see
        // loadAgentSessions refreshOnly merge for the same guard).
        const local = prevById.get(row.id);
        return withTeamAt(local?.context_budget ? { ...row, context_budget: local.context_budget } : row);
      });
    } catch (e: unknown) {
      if (myGeneration !== _loadGeneration) return;
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
      sessionsTotal.value += 1;
      listOffset.value += 1;
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
      sessionsTotal.value = 0;
      listOffset.value = 0;
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
    const hadAgentSession = sessions.value.some((s) => s.id === id);
    sessions.value = sessions.value.filter((s) => s.id !== id);
    if (hadAgentSession) {
      sessionsTotal.value = Math.max(0, sessionsTotal.value - 1);
      listOffset.value = Math.max(0, listOffset.value - 1);
    }
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
    sessionsTotal,
    sessionsLoading,
    sessionsLoadingMore,
    sessionListKeyword,
    sessionsHasMore,
    loadMoreAgentSessions,
    searchAgentSessions,
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
