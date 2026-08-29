import { nextTick, ref, type Ref } from 'vue';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { useSessionStore } from '../../../stores/session';
import { useTeamsStore } from '../../../stores/teams';
import { usePlatformStore } from '../../../stores/platform';
import type { PlatformResourceTreeNode } from '../../platform/types';
import type { ChatEntityKind, TeamRow } from '../../../components/chat/types';
import type { Agent } from '../../agents/types';
import type { Session } from '../../session/types';
import { hydrateAgentSettings } from '../agentPlannerSettings';
import { hydrateSessionForChannelFocus } from '../channelFocusLoad';
import type { ChatFocusCoordinator, FocusSessionOptions } from '../chatFocusCoordinator';
export type { FocusSessionOptions } from '../chatFocusCoordinator';
import type { useAppStore } from '../../../stores/app';
import type { useChatSessionStore } from '../../../stores/chat/sessionStore';
import type { useChatMessageStore } from '../../../stores/chat/messageStore';
import type { Message } from '../types';
import type { useChatStreamManager } from './useChatStreamManager';

type AppStore = ReturnType<typeof useAppStore>;
type SessionStore = ReturnType<typeof useChatSessionStore>;
type MessageStore = ReturnType<typeof useChatMessageStore>;
type StreamManager = ReturnType<typeof useChatStreamManager>;

export type EntityNavDeps = {
  appStore: AppStore;
  sessionStore: SessionStore;
  messageStore: MessageStore;
  streamManager: StreamManager;
  focusCoordinator: ChatFocusCoordinator;
  displayAgents: Ref<Agent[]>;
  displayTeams: Ref<TeamRow[]>;
  dialogMode: Ref<string>;
  selectedProviderModel: Ref<{ provider?: string; model?: string } | undefined>;
  makeSessionTitle: (content: string) => string;
  t: (key: string, ...args: unknown[]) => string;
};

export function useChatEntityNav(deps: EntityNavDeps) {
  const $q = useQuasar();
  const router = useRouter();
  const taxonomyTree = ref<PlatformResourceTreeNode[]>([]);

  function sessionOwnerIsTeam(session: Session) {
    return session.owner_type === 'team' || Boolean(session.team_id?.trim());
  }

  async function resolveSessionById(sessionId: string): Promise<Session | null> {
    const hit = deps.sessionStore.findSessionById(sessionId);
    if (hit) return hit;
    try {
      const sessionStore = useSessionStore();
      return await sessionStore.fetchSession(sessionId);
    } catch {
      return null;
    }
  }

  async function loadTeamSessions(teamID: string) {
    await deps.sessionStore.loadTeamSessions(teamID);
  }

  async function loadTeams() {
    try {
      const teamsStore = useTeamsStore();
      await teamsStore.loadTeams();
      const rows = teamsStore.teams;
      deps.displayTeams.value = rows.map((team) => ({
        id: team.id,
        team_key: team.team_key,
        display_name: team.display_name,
        status: team.status,
        isDefault: team.is_default,
        isWorking: /work|run|busy|ing/i.test(team.status || ''),
        definition_json: team.definition_json,
      }));
    } catch {
      deps.displayTeams.value = [];
    }
  }

  async function loadTaxonomyTree() {
    const platformStore = usePlatformStore();
    await platformStore.loadTaxonomyTree();
    taxonomyTree.value = platformStore.taxonomyTree;
  }

  function channelFocusLoadDeps() {
    return {
      getMessages: (sid: string) => deps.messageStore.getMessages(sid),
      loadMessages: (opts: { sessionId: string; replace?: boolean; dropStaleInFlight?: boolean }) =>
        deps.messageStore.loadMessages(opts),
      setMessages: (sid: string, rows: Message[]) => deps.messageStore.setMessages(sid, rows),
      ensureChatStream: (sid: string) => deps.streamManager.ensureChatStream(sid),
    };
  }

  async function selectAgent(
    agent: Agent,
    options?: {
      sessionId?: string;
      skipMessageReload?: boolean;
      /** Deferred skip flag for merged concurrent channel focus (read immediately before hydrate). */
      resolveSkipMessageReload?: () => boolean;
    },
  ) {
    const sameAgent = deps.sessionStore.entityKind === 'agent' && deps.appStore.selectedAgent?.id === agent.id;

    if (deps.sessionStore.entityKind === 'team') {
      deps.streamManager.disconnectTeamStream();
      deps.messageStore.clearAllMessages();
    }
    if (!sameAgent) {
      deps.sessionStore.resetForAgentSwitch();
    }
    const resolved = await hydrateAgentSettings(agent);
    deps.appStore.selectedAgent = resolved;
    deps.appStore.upsertAgent(resolved);
    if (!sameAgent) {
      await deps.sessionStore.loadAgentSessions(resolved.id);
    } else {
      await deps.sessionStore.loadAgentSessions(resolved.id, { refreshOnly: true });
    }
    await nextTick();
    const preferredId = options?.sessionId?.trim();
    let picked =
      preferredId != null && preferredId !== ''
        ? (deps.sessionStore.sessions.find((item) => item.id === preferredId) ?? null)
        : (deps.sessionStore.sessions[0] ?? null);
    if (preferredId && !picked) {
      const fetched = await resolveSessionById(preferredId);
      if (fetched && fetched.agent_id?.trim() === resolved.id) {
        if (!deps.sessionStore.sessions.some((item) => item.id === preferredId)) {
          deps.sessionStore.sessions = [fetched, ...deps.sessionStore.sessions];
        }
        picked = fetched;
      }
    }
    deps.sessionStore.selectedSession = picked ?? deps.sessionStore.sessions[0] ?? null;
    if (deps.sessionStore.selectedSession) {
      const sid = deps.sessionStore.selectedSession.id;
      const skipReload = options?.resolveSkipMessageReload?.() ?? options?.skipMessageReload ?? false;
      // Sync URL so route watch and alreadyFocused checks stay consistent.
      const targetQuery: Record<string, string> = { session: sid, agent: resolved.id };
      const route = router.currentRoute.value;
      const routeSession = typeof route.query.session === 'string' ? route.query.session.trim() : '';
      const routeAgent = typeof route.query.agent === 'string' ? route.query.agent.trim() : '';
      if (route.name === 'chat' && (routeSession !== sid || routeAgent !== resolved.id)) {
        await deps.focusCoordinator.withRouteWatchSuppressed(async () => {
          await router.replace({ name: 'chat', query: targetQuery });
        });
      }
      await hydrateSessionForChannelFocus(channelFocusLoadDeps(), sid, skipReload);
    }
  }

  async function selectTeam(team: TeamRow, options?: { sessionId?: string }) {
    if (deps.sessionStore.selectedTeamId && deps.sessionStore.selectedTeamId !== team.id) {
      deps.streamManager.disconnectTeamStream();
      deps.messageStore.clearAllMessages();
    }
    deps.sessionStore.resetForTeamSwitch(team.id);
    await loadTeamSessions(team.id);
    const preferredId = options?.sessionId?.trim();
    const teamList = deps.sessionStore.teamSessions[team.id] ?? [];
    deps.sessionStore.teamSelectedSessionId = preferredId
      ? (teamList.find((item) => item.id === preferredId)?.id ?? preferredId)
      : (teamList[0]?.id ?? null);
    if (deps.sessionStore.teamSelectedSessionId) {
      await deps.messageStore.loadMessages({ sessionId: deps.sessionStore.teamSelectedSessionId, replace: true });
      deps.streamManager.ensureTeamStream(deps.sessionStore.teamSelectedSessionId);
    }
  }

  async function focusAgentSessionView(sessionId: string, agentId: string, options?: FocusSessionOptions) {
    const route = router.currentRoute.value;
    const routeSession = typeof route.query.session === 'string' ? route.query.session.trim() : '';
    const routeAgent = typeof route.query.agent === 'string' ? route.query.agent.trim() : '';
    const alreadyFocused =
      route.name === 'chat' &&
      routeSession === sessionId &&
      routeAgent === agentId &&
      deps.sessionStore.entityKind === 'agent' &&
      deps.appStore.selectedAgent?.id === agentId &&
      deps.sessionStore.selectedSession?.id === sessionId;

    if (alreadyFocused) {
      const session = deps.sessionStore.sessions.find((item) => item.id === sessionId);
      if (session) deps.sessionStore.selectedSession = session;
      await hydrateSessionForChannelFocus(channelFocusLoadDeps(), sessionId, options?.skipMessageReload);
      return;
    }

    const query = { session: sessionId, agent: agentId };
    const needsRouteSync = route.name !== 'chat' || routeSession !== sessionId || routeAgent !== agentId;
    await deps.focusCoordinator.withRouteWatchSuppressed(async () => {
      if (needsRouteSync) {
        if (route.name === 'chat') {
          await router.replace({ name: 'chat', query });
        } else {
          await router.push({ name: 'chat', query });
        }
      }
      // Route watch may not fire when query is unchanged; always focus in store.
      await focusSessionById(sessionId, agentId, options);
    });
  }

  async function onSelectSession(sessionId: string) {
    const resolved = await resolveSessionById(sessionId);
    if (!resolved) return;

    if (sessionOwnerIsTeam(resolved)) {
      const teamId = resolved.team_id?.trim();
      if (!teamId) return;
      const team = deps.displayTeams.value.find((item) => item.id === teamId);
      if (!team) {
        $q.notify({ type: 'warning', message: '找不到该会话所属的 Team' });
        return;
      }
      if (deps.sessionStore.entityKind !== 'team' || deps.sessionStore.selectedTeamId !== teamId) {
        await selectTeam(team, { sessionId });
        deps.streamManager.ensureTeamStream(sessionId);
        return;
      }
      deps.sessionStore.teamSelectedSessionId = sessionId;
      await deps.messageStore.loadMessages({ sessionId, replace: true });
      deps.streamManager.ensureTeamStream(sessionId);
      return;
    }

    const agentId =
      resolved.agent_id?.trim() ||
      (deps.sessionStore.sessions.some((item) => item.id === sessionId)
        ? deps.appStore.selectedAgent?.id?.trim()
        : '') ||
      '';
    if (!agentId) return;
    const agent =
      deps.appStore.agents.find((item) => item.id === agentId) ??
      deps.displayAgents.value.find((item) => item.id === agentId);
    if (!agent) {
      $q.notify({ type: 'warning', message: '找不到该会话所属的 Agent' });
      return;
    }
    await focusAgentSessionView(sessionId, agentId);
  }

  async function onRenameSession(payload: { id: string; title: string }) {
    const title = payload.title.trim();
    if (!title) return;
    try {
      await deps.sessionStore.renameSessionByKind(payload.id, title);
    } catch {
      $q.notify({ type: 'negative', message: '重命名失败，请重试' });
    }
  }

  async function onTogglePinSession(payload: { id: string; pinned: boolean }) {
    try {
      await deps.sessionStore.setSessionPinnedByKind(payload.id, payload.pinned);
    } catch {
      $q.notify({ type: 'negative', message: payload.pinned ? '置顶失败，请重试' : '取消置顶失败，请重试' });
    }
  }

  async function onRestoreSession(sessionId: string) {
    try {
      const sessionStore = useSessionStore();
      await sessionStore.restore(sessionId);
      if (deps.sessionStore.entityKind === 'team' && deps.sessionStore.selectedTeamId) {
        const teamId = deps.sessionStore.selectedTeamId;
        const sessions = deps.sessionStore.teamSessions[teamId] ?? [];
        deps.sessionStore.teamSessions[teamId] = sessions.map((s) =>
          s.id === sessionId ? { ...s, archived_at: '' } : s,
        );
      }
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '恢复会话失败' });
    }
  }

  async function onArchiveSession(sessionId: string) {
    try {
      const sessionStore = useSessionStore();
      await sessionStore.archive(sessionId);
      if (deps.sessionStore.entityKind === 'team' && deps.sessionStore.selectedTeamId) {
        const teamId = deps.sessionStore.selectedTeamId;
        const sessions = deps.sessionStore.teamSessions[teamId] ?? [];
        deps.sessionStore.teamSessions[teamId] = sessions.map((s) =>
          s.id === sessionId ? { ...s, archived_at: new Date().toISOString() } : s,
        );
      }
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '归档会话失败' });
    }
  }

  function onSessionDetail(sessionId: string) {
    router.push({ name: 'session-detail', params: { sessionId } });
  }

  async function onNewSession(title?: string) {
    if (deps.sessionStore.entityKind === 'agent' && deps.appStore.selectedAgent) {
      const selectedModel = deps.selectedProviderModel.value;
      await deps.sessionStore.addAgentSession(deps.appStore.selectedAgent.id, title || deps.t('chat.untitledSession'), {
        dialog_mode: deps.dialogMode.value,
        default_provider: selectedModel?.provider || deps.appStore.selectedAgent.provider,
        default_model: selectedModel?.model || deps.appStore.selectedAgent.model,
      });
      if (deps.sessionStore.selectedSession) {
        await deps.messageStore.loadMessages({ sessionId: deps.sessionStore.selectedSession.id });
        deps.streamManager.ensureChatStream(deps.sessionStore.selectedSession.id);
      }
      return;
    }

    if (deps.sessionStore.entityKind === 'team' && deps.sessionStore.selectedTeamId) {
      const selectedModel = deps.selectedProviderModel.value;
      const created = await deps.sessionStore.addTeamSession(
        deps.sessionStore.selectedTeamId,
        title || deps.t('chat.untitledSession'),
        {
          dialog_mode: deps.dialogMode.value,
          default_provider: selectedModel?.provider || '',
          default_model: selectedModel?.model || '',
        },
      );
      if (created) {
        deps.streamManager.ensureTeamStream(created.id);
      }
      return;
    }

    // No valid entity selected — notify the user instead of silently skipping
    $q.notify({
      type: 'warning',
      message: deps.t('chat.noEntityForNewSession', '请先选择一个 Agent 或团队以创建会话'),
    });
  }

  /**
   * Fork-from-Turn（79-runtime-governance R6）：以 turn 为分叉点派生新会话
   * 并跳转。源会话只读零影响；失败（非根会话 / turn 不存在等）提示错误。
   */
  async function onForkSessionTurn(turn: { ID: string; SessionID: string }) {
    const srcId = turn.SessionID?.trim();
    const turnId = turn.ID?.trim();
    if (!srcId || !turnId) return;
    try {
      const created = await deps.sessionStore.forkSessionAction(srcId, turnId);
      $q.notify({ type: 'positive', message: deps.t('chat.forkSuccess', '已创建分支会话'), timeout: 1500 });
      await onSelectSession(created.id);
    } catch {
      $q.notify({ type: 'negative', message: deps.t('chat.forkFailed', '创建分支会话失败，请重试') });
    }
  }

  async function openSettings(kind: ChatEntityKind, id: string) {
    if (kind === 'agent') {
      await router.push(`/agents/${id}/settings`);
      return;
    }
    if (kind === 'team') {
      await router.push({ name: 'team', query: { edit: id } });
    }
  }

  async function focusSessionById(sessionId: string, agentId?: string, options?: FocusSessionOptions) {
    const sid = sessionId.trim();
    if (!sid) return;
    const key = deps.focusCoordinator.focusKey(sid, agentId);
    await deps.focusCoordinator.runFocusOnce(key, options, async (resolveSkipReload) => {
      const session = await resolveSessionById(sid);
      if (!session) {
        $q.notify({ type: 'warning', message: deps.t('chat.sessionNotFound') });
        return;
      }
      const aid = agentId?.trim() || session.agent_id?.trim() || deps.appStore.selectedAgent?.id?.trim();
      if (!aid) return;
      const agent =
        deps.appStore.agents.find((a) => a.id === aid) ?? deps.displayAgents.value?.find((a) => a.id === aid);
      if (!agent) {
        $q.notify({ type: 'warning', message: deps.t('chat.sessionAgentNotFound') });
        return;
      }
      await selectAgent(agent, { sessionId: sid, resolveSkipMessageReload: resolveSkipReload });
    });
  }

  return {
    taxonomyTree,
    loadTaxonomyTree,
    loadTeams,
    loadTeamSessions,
    selectAgent,
    selectTeam,
    focusAgentSessionView,
    focusSessionById,
    onSelectSession,
    onRenameSession,
    onTogglePinSession,
    onRestoreSession,
    onArchiveSession,
    onSessionDetail,
    onNewSession,
    onForkSessionTurn,
    openSettings,
    updateTeam: async (id: string, payload: object) => {
      const teamsStore = useTeamsStore();
      return teamsStore.editTeam(id, payload) as unknown as Promise<import('../../../components/chat/types').TeamRow>;
    },
  };
}
