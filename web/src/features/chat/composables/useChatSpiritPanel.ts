import { computed, ref, watch } from 'vue';
import { Notify } from 'quasar';
import { useSpiritTeamStore } from '../../../stores/spirit';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Agent } from '../../agents/types';
import type { MemberSession } from '../v2Types';
import { useScrollToActivity } from './useScrollToActivity';
import { USER_INPUT_HARD_LIMIT_CHARS } from './useChatSender';
import type { useChatWorkspace } from './useChatWorkspace';

const SPIRIT_AGENT_KEY = '__spirit__';

type Workspace = ReturnType<typeof useChatWorkspace>;

/**
 * Spirit/team panel glue for ChatPage: status bar aggregation, team status
 * pulse watch, member session resolution, and all agent/team card actions.
 * Extracted from ChatPage.vue (red line #13: Page script should stay lean);
 * the page only wires the returned handlers to component events.
 */
export function useChatSpiritPanel(workspace: Workspace) {
  const { entity, session, layout } = workspace;
  const spiritStore = useSpiritTeamStore();
  const runtimeStore = useChatRuntimeStore();
  const { locate } = useScrollToActivity();

  const activeMember = computed(() => {
    const team = spiritStore.activeTeam;
    const memberId = spiritStore.activeMemberId;
    if (!team || !memberId) return null;
    return team.members.find((m) => m.agentId === memberId) ?? null;
  });

  /** Show SessionTreeSidebar when in spirit mode with an active team (sub-sessions exist). */
  const showSessionTree = computed(() => spiritStore.activePanelMode === 'spirit' && Boolean(spiritStore.activeTeamId));

  /** Navigate to a session tree node: switch Activity stream and lazy-load if needed. */
  function onSelectSessionTreeNode(sessionId: string) {
    void session.onSelectSession(sessionId);
  }

  const spiritStatusBar = computed(() => {
    const teams = spiritStore.teams;
    const activeTeam = spiritStore.activeTeam;
    const running = teams.filter((t) => t.status === 'running' || t.status === 'pending').length;
    const interrupted = teams.filter((t) => t.status === 'interrupted').length;
    const completedTeams = teams.filter((t) => t.status === 'completed');
    // Prefer active team token usage; fall back to current session composer snapshot.
    // Do not sum all teams in the store — that mixes unrelated runs.
    const totalTokenIn = activeTeam?.tokenIn ?? 0;
    const totalTokenOut = activeTeam?.tokenOut ?? 0;
    const sessionTokens = session.composerUsageSnapshot;
    const tokenUsage =
      totalTokenIn > 0 || totalTokenOut > 0
        ? { in: totalTokenIn, out: totalTokenOut }
        : sessionTokens && (sessionTokens.inputTokens > 0 || sessionTokens.outputTokens > 0)
          ? { in: sessionTokens.inputTokens, out: sessionTokens.outputTokens }
          : null;
    return {
      runningTeamCount: running,
      interruptedTeamCount: interrupted,
      completedTeamCount: completedTeams.length,
      totalTeamCount: teams.length,
      tokenUsage,
      contextRatio: sessionTokens?.contextRatio ?? null,
      contextUsedTokens: sessionTokens?.contextUsedTokens ?? null,
      contextWindow: sessionTokens?.contextWindow ?? null,
      contextBudget: sessionTokens?.contextBudget ?? null,
      complexityLevel: spiritStore.planCreated?.complexity_level ?? null,
      complexityReason: spiritStore.planCreated?.strategy_reason ?? null,
      checkpointStep: spiritStore.lastCheckpoint?.step ?? null,
      dqScore: spiritStore.lastDqScore?.overall ?? null,
    };
  });

  const pulseTeamColors = computed(() => {
    const map = new Map<string, { color: string; durationMs: number }>();
    for (const [teamId, state] of session.spiritPulseStates) {
      map.set(teamId, { color: state.color, durationMs: state.durationMs });
    }
    return map;
  });

  watch(
    () => spiritStore.teams.map((t) => `${t.id}:${t.status}`),
    (newVal, oldVal) => {
      if (!oldVal) return;
      const newEntries = newVal.map((s) => s.split(':'));
      const oldMap = new Map(
        oldVal.map((s) => {
          const [id, status] = s.split(':');
          return [id, status];
        }),
      );
      for (const [id, status] of newEntries) {
        const oldStatus = oldMap.get(id);
        if (oldStatus && oldStatus !== status) {
          session.spiritOnTeamStatusChanged(id, status);
        }
      }
    },
  );

  function onSelectSpirit() {
    spiritStore.returnToSpirit();
    const spiritAgent = entity.store.agents.find((a: Agent) => a.agent_key === SPIRIT_AGENT_KEY);
    if (spiritAgent) {
      // Always re-select when coming from team mode or when agent differs.
      const needsReselect = entity.store.selectedAgent?.id !== spiritAgent.id || entity.selectedEntityKind !== 'agent';
      if (needsReselect) {
        entity.selectAgent(spiritAgent);
      }
    } else {
      // Fallback: select the spirit/first agent if no spirit agent exists
      const fallback = entity.store.agents.find((a: Agent) => a.agent_key === '__spirit__') || entity.store.agents[0];
      if (fallback && (entity.store.selectedAgent?.id !== fallback.id || entity.selectedEntityKind !== 'agent')) {
        entity.selectAgent(fallback);
      }
    }
  }

  /** T8.6: 点击左侧 Agent 卡片定位到中间面板会话 */
  function onSidebarLocateAgent(payload: { agentKey: string; teamSessionId: string; teamId: string }) {
    locate(payload.agentKey, payload.teamSessionId, payload.teamId);
  }

  // ── 左侧成员执行过程弹框（与 GraphStageBlock 成员行点击弹框同一组件） ──
  const activityStore = useChatActivityStore();
  const sidebarMemberDialogOpen = ref(false);
  const sidebarActiveMemberId = ref<string | null>(null);
  // 实时查询（同 GraphStageBlock）：store 以新对象替换方式 upsert memberSession，
  // 点击时存快照会导致弹框中 Status/canInject 过期。
  const sidebarActiveMember = computed<MemberSession | null>(() => {
    const id = sidebarActiveMemberId.value;
    return id ? (activityStore.memberSessions.get(id) ?? null) : null;
  });

  /** 解析左侧成员卡片对应的 MemberSession。
   * 优先 member.chatSessionId == MemberSession.SessionID（精确 1:1）；
   * 回退 AgentKey + TeamRun.SessionID == teamSessionId；
   * 再回退 AgentKey + SpiritSessionID（同 key 多团队时取 Seq 最大者）。
   */
  function resolveSidebarMemberSession(payload: {
    agentKey: string;
    teamSessionId: string;
    teamId: string;
  }): MemberSession | null {
    const team = spiritStore.teams.find((t) => t.id === payload.teamId);
    const member = team?.members.find((m) => m.agentKey === payload.agentKey);
    if (member?.chatSessionId) {
      for (const ms of activityStore.memberSessions.values()) {
        if (ms.SessionID === member.chatSessionId) return ms;
      }
    }
    let best: MemberSession | null = null;
    for (const ms of activityStore.memberSessions.values()) {
      if (ms.AgentKey !== payload.agentKey) continue;
      const run = activityStore.teamRuns.get(ms.TeamRunID);
      if (run && payload.teamSessionId && run.SessionID === payload.teamSessionId) return ms;
      if (team?.spiritSessionId && ms.SpiritSessionID === team.spiritSessionId) {
        if (!best || ms.Seq > best.Seq) best = ms;
      }
    }
    return best;
  }

  /** 点击左侧成员卡片主体：弹出该成员的执行过程弹框（同 graph 成员行点击）。
   *  未命中（如历史会话尚未水合 memberSessions）时仅保留 locate 行为，不弹空框。 */
  function onSidebarSelectMember(payload: { agentKey: string; teamSessionId: string; teamId: string }) {
    const ms = resolveSidebarMemberSession(payload);
    if (!ms) return;
    sidebarActiveMemberId.value = ms.ID;
    sidebarMemberDialogOpen.value = true;
  }

  /** Resolve member chat session ID for Pause/Resume/Cancel (backend expects session_id). */
  function resolveMemberChatSessionId(agentKey: string): string | null {
    const team = spiritStore.teams.find((t) => t.members.some((m) => m.agentKey === agentKey));
    const member = team?.members.find((m) => m.agentKey === agentKey);
    if (!member) return null;
    if (member.chatSessionId) return member.chatSessionId;
    // Fallback: session tree lookup (authoritative when member profile only has catalog agentId).
    const spiritSessionId = team?.spiritSessionId || session.selectedSessionForUi?.id;
    if (spiritSessionId && session.sessionTree?.findMemberSessionId) {
      return session.sessionTree.findMemberSessionId(spiritSessionId, agentKey, team?.teamSessionId);
    }
    return null;
  }

  /** T8.3: 左侧 Agent 卡片暂停/恢复/取消 — 必须传 chat session ID，不能传 catalog agentId */
  function onSidebarPauseAgent(agentKey: string) {
    const sessionId = resolveMemberChatSessionId(agentKey);
    if (sessionId) void spiritStore.pauseAgent(sessionId);
  }

  function onSidebarResumeAgent(agentKey: string) {
    const sessionId = resolveMemberChatSessionId(agentKey);
    if (sessionId) void spiritStore.resumeAgent(sessionId);
  }

  /** 失败重试：调 RetrySession API 重新入队最后一条用户消息（B.5.2）。 */
  function onSidebarRetryAgent(agentKey: string) {
    const sessionId = resolveMemberChatSessionId(agentKey);
    if (sessionId) void spiritStore.retryAgent(sessionId);
  }

  function onSidebarCancelAgent(agentKey: string) {
    const sessionId = resolveMemberChatSessionId(agentKey);
    if (sessionId) void spiritStore.cancelAgent(sessionId);
  }

  /** Phase B-4 / §9.1.3: Handle team-member click to expand that member's session.
   *  Resolves agentKey → agentId via spiritStore.activeTeam.members (preferring
   *  the team identified by payload.teamId when provided — useful when the user
   *  is browsing a non-active team's stage), then calls spiritStore.selectMember.
   *  The panelMode/activeMemberId watchers in useChatWorkspace (Phase B-3)
   *  resolve the member session id from the session tree and lazy-load activities. */
  function onExpandMember(payload: { agentKey: string; agentName?: string; teamId?: string }) {
    const team = payload.teamId
      ? (spiritStore.teams.find((t) => t.id === payload.teamId) ?? spiritStore.activeTeam)
      : spiritStore.activeTeam;
    const member = team?.members.find((m) => m.agentKey === payload.agentKey);
    if (!member) return;
    if (payload.teamId && spiritStore.activeTeamId !== payload.teamId) {
      spiritStore.selectTeam(payload.teamId);
    }
    spiritStore.selectMember(member.agentId);
  }

  /** Phase B-6 / §9.1.3: Handle AgentCard click to navigate into the
   *  child session it represents. Switches the Activity stream to the child
   *  session and lazy-loads its activities (cache-aware — skips the API call
   *  when the session is already cached). */
  function onEnterSession(sessionId: string) {
    void session.onSelectSession(sessionId);
  }

  /** T5.2/T5.3 / §B.7.2: Lazy-load member/child session activities when a
   *  team-card or agent-card expands. Cache-aware — `ensureActivitiesLoaded`
   *  skips sessions that are already cached (T5.4). Unlike `onEnterSession`,
   *  this does NOT switch the current driving session — expanded children
   *  render inline within the parent stream. */
  function onExpandChildren(sessionIds: string[]) {
    for (const sid of sessionIds) {
      if (!sid) continue;
      void session.activityStore.ensureMemberStepsLoaded([sid]);
    }
  }

  /** Phase T3 / §B.5.2: Cancel an in-flight sub-agent run by childSessionId.
   *  Reuses the existing StopGeneration RPC; the activity stream is updated
   *  via WS run_status=cancelled events. */
  async function onCancelAgent(sessionId: string) {
    if (!sessionId) return;
    await spiritStore.cancelAgent(sessionId);
  }

  /** Phase T3 / §B.5.2: Retry a failed/interrupted sub-agent run by
   *  re-enqueuing the last user message in the child session. */
  async function onRetryAgent(sessionId: string) {
    if (!sessionId) return;
    await spiritStore.retryAgent(sessionId);
  }

  /** Patch MemberSession card status by chat SessionID (not entity ID).
   *  Returns the previous status when a matching card was found. */
  function patchMemberSessionStatus(sessionId: string, status: 'paused' | 'running'): string | null {
    for (const ms of activityStore.memberSessions.values()) {
      if (ms.SessionID === sessionId) {
        const prev = ms.Status;
        activityStore.upsertMemberSession({ ...ms, Status: status });
        return prev;
      }
    }
    return null;
  }

  /** §B.5.3: Pause an in-flight sub-agent run by childSessionId.
   *  MVP cancels the active turn and marks the session as paused. */
  async function onPauseAgent(sessionId: string) {
    if (!sessionId) return;
    patchMemberSessionStatus(sessionId, 'paused');
    await spiritStore.pauseAgent(sessionId);
  }

  /** §B.5.3: Resume a paused sub-agent session.
   *  MVP flips the status marker; user injects a new message to resume execution. */
  async function onResumeAgent(sessionId: string) {
    if (!sessionId) return;
    patchMemberSessionStatus(sessionId, 'running');
    await spiritStore.resumeAgent(sessionId);
  }

  /** §B.5.3: Inject a user message into the sub-agent session's pending queue. */
  async function onInjectAgent(payload: { sessionId: string; message: string }) {
    if (!payload.sessionId || !payload.message.trim()) return;
    // 注入内容硬上限（与主输入一致，2026-07-27）：超上限拒绝并提示。
    if (payload.message.trim().length > USER_INPUT_HARD_LIMIT_CHARS) {
      Notify.create({
        type: 'warning',
        message: layout.t('chat.inputTooLong', { limit: USER_INPUT_HARD_LIMIT_CHARS }),
        position: 'top',
      });
      return;
    }
    try {
      await runtimeStore.enqueue(payload.sessionId, payload.message);
      Notify.create({ type: 'positive', message: layout.t('chat.sessionStage.injectSent'), position: 'top' });
    } catch {
      Notify.create({ type: 'warning', message: layout.t('chat.sessionStage.injectFailed'), position: 'top' });
    }
  }

  /** OBS-04: Status bar click handlers — navigate to the first matching team. */
  function onStatusBarClickRunning() {
    const team = spiritStore.teams.find((t) => t.status === 'running');
    if (team) spiritStore.selectTeam(team.id);
  }

  function onStatusBarClickInterrupted() {
    const team = spiritStore.teams.find((t) => t.status === 'interrupted');
    if (team) spiritStore.selectTeam(team.id);
  }

  return {
    activeMember,
    showSessionTree,
    onSelectSessionTreeNode,
    spiritStatusBar,
    pulseTeamColors,
    onSelectSpirit,
    onSidebarLocateAgent,
    onSidebarSelectMember,
    sidebarMemberDialogOpen,
    sidebarActiveMember,
    onSidebarPauseAgent,
    onSidebarResumeAgent,
    onSidebarRetryAgent,
    onSidebarCancelAgent,
    onExpandMember,
    onEnterSession,
    onExpandChildren,
    onCancelAgent,
    onRetryAgent,
    onPauseAgent,
    onResumeAgent,
    onInjectAgent,
    onStatusBarClickRunning,
    onStatusBarClickInterrupted,
  };
}
