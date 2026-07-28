import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { Notify } from 'quasar';
import { useSpiritTeamStore } from '../../../stores/spirit';
import { useUiConfigStore } from '../../../stores/uiConfig';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import { USER_INPUT_HARD_LIMIT_CHARS } from './useChatSender';
import type { useChatWorkspace } from './useChatWorkspace';
import type { Agent } from '../../agents/types';
import type { SpiritMember, SpiritStatusBarData, SpiritTeam } from '../../spirit/types';
import type { ComposerUsageSnapshot } from '../composerUsageMetrics';
import type { MemberSession, MemberSessionStatus } from '../v2Types';

export const SPIRIT_AGENT_KEY = '__spirit__';

type ChatWorkspace = ReturnType<typeof useChatWorkspace>;

/** agent_key → display info map for message author rendering. Pure — testable in isolation. */
export function buildAgentMap(agents: Agent[]): Map<string, { displayName: string; agentKey: string }> {
  const map = new Map<string, { displayName: string; agentKey: string }>();
  for (const agent of agents) {
    if (agent?.agent_key) {
      map.set(agent.agent_key, {
        displayName: agent.display_name || agent.agent_key,
        agentKey: agent.agent_key,
      });
    }
  }
  return map;
}

/** Resolve the active member object from the active team + member id. Pure — testable in isolation. */
export function resolveActiveMember(
  team: SpiritTeam | null | undefined,
  memberId: string | null | undefined,
): SpiritMember | null {
  if (!team || !memberId) return null;
  return team.members.find((m) => m.agentId === memberId) ?? null;
}

export type SpiritStatusBarModelInput = {
  teams: SpiritTeam[];
  activeTeam?: SpiritTeam | null;
  usageSnapshot?: ComposerUsageSnapshot | null;
  complexityLevel?: string | null;
  complexityReason?: string | null;
  checkpointStep?: string | null;
  dqScore?: number | null;
};

/**
 * Build the SpiritStatusBar view model. Token usage prefers the active team's
 * counters; falls back to the current session composer snapshot. Pure —
 * testable in isolation.
 */
export function buildSpiritStatusBarModel(input: SpiritStatusBarModelInput): SpiritStatusBarData {
  const { teams, activeTeam, usageSnapshot } = input;
  const running = teams.filter((t) => t.status === 'running' || t.status === 'pending').length;
  const interrupted = teams.filter((t) => t.status === 'interrupted').length;
  const completed = teams.filter((t) => t.status === 'completed').length;
  const totalTokenIn = activeTeam?.tokenIn ?? 0;
  const totalTokenOut = activeTeam?.tokenOut ?? 0;
  const tokenUsage =
    totalTokenIn > 0 || totalTokenOut > 0
      ? { in: totalTokenIn, out: totalTokenOut }
      : usageSnapshot && (usageSnapshot.inputTokens > 0 || usageSnapshot.outputTokens > 0)
        ? { in: usageSnapshot.inputTokens, out: usageSnapshot.outputTokens }
        : null;
  return {
    runningTeamCount: running,
    interruptedTeamCount: interrupted,
    completedTeamCount: completed,
    totalTeamCount: teams.length,
    tokenUsage,
    contextRatio: usageSnapshot?.contextRatio ?? null,
    contextUsedTokens: usageSnapshot?.contextUsedTokens ?? null,
    contextWindow: usageSnapshot?.contextWindow ?? null,
    complexityLevel: input.complexityLevel ?? null,
    complexityReason: input.complexityReason ?? null,
    checkpointStep: input.checkpointStep ?? null,
    dqScore: input.dqScore ?? null,
  };
}

/**
 * Resolve a member's chat session ID for Pause/Resume/Cancel RPCs (backend
 * expects session_id, not the catalog agentId). Prefers the member profile's
 * chatSessionId; falls back to the session tree lookup. Pure — testable in
 * isolation.
 */
export function resolveMemberChatSessionId(args: {
  teams: SpiritTeam[];
  agentKey: string;
  fallbackSpiritSessionId?: string | null;
  findMemberSessionId?: (spiritSessionId: string, agentKey: string, teamSessionId?: string | null) => string | null;
}): string | null {
  const { teams, agentKey, fallbackSpiritSessionId, findMemberSessionId } = args;
  const team = teams.find((t) => t.members.some((m) => m.agentKey === agentKey));
  const member = team?.members.find((m) => m.agentKey === agentKey);
  if (!member) return null;
  if (member.chatSessionId) return member.chatSessionId;
  const spiritSessionId = team?.spiritSessionId || fallbackSpiritSessionId;
  if (spiritSessionId && findMemberSessionId) {
    return findMemberSessionId(spiritSessionId, agentKey, team?.teamSessionId);
  }
  return null;
}

type MemberSessionPatchStore = {
  memberSessions: Map<string, MemberSession>;
  upsertMemberSession: (ms: MemberSession) => void;
};

/**
 * Patch a MemberSession card status by chat SessionID (not entity ID).
 * Returns the previous status when a matching card was found. Pure — testable
 * in isolation with a plain Map stub.
 */
export function patchMemberSessionStatus(
  store: MemberSessionPatchStore,
  sessionId: string,
  status: MemberSessionStatus,
): MemberSessionStatus | null {
  for (const ms of store.memberSessions.values()) {
    if (ms.SessionID === sessionId) {
      const prev = ms.Status;
      store.upsertMemberSession({ ...ms, Status: status });
      return prev;
    }
  }
  return null;
}

/**
 * Shared ChatMessagePanel bindings for chat pages (desktop ChatPage today,
 * mobile MobileChatPage in P1). Extracts the panel-level view models
 * (agentMap / activeMember / spiritStatusBar) and spirit/team/member control
 * handlers so a page only wires template props/events — logic lives here.
 *
 * Note: desktop ChatPage keeps its own inline copies for now; this composable
 * is the canonical implementation for new (mobile) consumers and a future
 * desktop adoption.
 */
export function useChatMessagePanelBindings(workspace: ChatWorkspace) {
  const { t } = useI18n();
  const router = useRouter();
  const spiritStore = useSpiritTeamStore();
  const uiConfig = useUiConfigStore();
  const runtimeStore = useChatRuntimeStore();

  const activeMember = computed(() => resolveActiveMember(spiritStore.activeTeam, spiritStore.activeMemberId));

  const agentMap = computed(() => buildAgentMap(workspace.entity.store.agents));

  const spiritStatusBar = computed(() =>
    buildSpiritStatusBarModel({
      teams: spiritStore.teams,
      activeTeam: spiritStore.activeTeam,
      usageSnapshot: workspace.session.composerUsageSnapshot,
      complexityLevel: spiritStore.planCreated?.complexity_level ?? null,
      complexityReason: spiritStore.planCreated?.strategy_reason ?? null,
      checkpointStep: spiritStore.lastCheckpoint?.step ?? null,
      dqScore: spiritStore.lastDqScore?.overall ?? null,
    }),
  );

  function resolveMemberChatSessionIdBound(agentKey: string): string | null {
    return resolveMemberChatSessionId({
      teams: spiritStore.teams,
      agentKey,
      fallbackSpiritSessionId: workspace.session.selectedSessionForUi?.id,
      findMemberSessionId: workspace.session.sessionTree?.findMemberSessionId,
    });
  }

  /** Return to the spirit panel: reset panel mode and re-select the spirit agent. */
  function onSelectSpirit() {
    spiritStore.returnToSpirit();
    const store = workspace.entity.store;
    const spiritAgent = store.agents.find((a: Agent) => a.agent_key === SPIRIT_AGENT_KEY);
    if (spiritAgent) {
      const needsReselect =
        store.selectedAgent?.id !== spiritAgent.id || workspace.entity.selectedEntityKind !== 'agent';
      if (needsReselect) {
        void workspace.entity.selectAgent(spiritAgent);
      }
      return;
    }
    const fallback = store.agents.find((a: Agent) => a.agent_key === SPIRIT_AGENT_KEY) ?? store.agents[0];
    if (fallback && (store.selectedAgent?.id !== fallback.id || workspace.entity.selectedEntityKind !== 'agent')) {
      void workspace.entity.selectAgent(fallback);
    }
  }

  function onNavigate(route: { name: string; params: Record<string, string> }) {
    void router.push(route);
  }

  /** Jump to the artifacts page with the current session filter applied. */
  function onOpenArtifactsPage() {
    const sid = workspace.session.selectedSessionForUi?.id;
    if (!sid) return;
    void router.push({ name: 'artifacts', query: { session: sid } });
  }

  function onErrorRelogin() {
    void router.push({ name: 'login' });
  }

  /** Status bar click handlers — select the first matching team. */
  function onStatusBarClickRunning() {
    const team = spiritStore.teams.find((team) => team.status === 'running');
    if (team) spiritStore.selectTeam(team.id);
  }

  function onStatusBarClickInterrupted() {
    const team = spiritStore.teams.find((team) => team.status === 'interrupted');
    if (team) spiritStore.selectTeam(team.id);
  }

  /**
   * Team-member click → expand that member's session. Resolves agentKey →
   * agentId via the target team's members, then selects the member; the
   * workspace panel-mode watchers resolve and load the member session.
   */
  function onExpandMember(payload: { agentKey: string; agentName?: string; teamId?: string }) {
    const team = payload.teamId
      ? (spiritStore.teams.find((team) => team.id === payload.teamId) ?? spiritStore.activeTeam)
      : spiritStore.activeTeam;
    const member = team?.members.find((m) => m.agentKey === payload.agentKey);
    if (!member) return;
    if (payload.teamId && spiritStore.activeTeamId !== payload.teamId) {
      spiritStore.selectTeam(payload.teamId);
    }
    spiritStore.selectMember(member.agentId);
  }

  /** AgentCard click → switch the Activity stream to the child session. */
  function onEnterSession(sessionId: string) {
    void workspace.session.onSelectSession(sessionId);
  }

  /** Lazy-load member/child session activities when cards expand (cache-aware). */
  function onExpandChildren(sessionIds: string[]) {
    for (const sid of sessionIds) {
      if (!sid) continue;
      void workspace.session.activityStore.ensureMemberStepsLoaded([sid]);
    }
  }

  /** Cancel an in-flight sub-agent run by childSessionId. */
  async function onCancelAgent(sessionId: string) {
    if (!sessionId) return;
    await spiritStore.cancelAgent(sessionId);
  }

  /** Retry a failed/interrupted sub-agent run by re-enqueuing its last user message. */
  async function onRetryAgent(sessionId: string) {
    if (!sessionId) return;
    await spiritStore.retryAgent(sessionId);
  }

  /** Pause an in-flight sub-agent run; patch the card status optimistically. */
  async function onPauseAgent(sessionId: string) {
    if (!sessionId) return;
    patchMemberSessionStatus(workspace.session.activityStore, sessionId, 'paused');
    await spiritStore.pauseAgent(sessionId);
  }

  /** Resume a paused sub-agent session. */
  async function onResumeAgent(sessionId: string) {
    if (!sessionId) return;
    patchMemberSessionStatus(workspace.session.activityStore, sessionId, 'running');
    await spiritStore.resumeAgent(sessionId);
  }

  /** Inject a user message into a sub-agent session's pending queue. */
  async function onInjectAgent(payload: { sessionId: string; message: string }) {
    if (!payload.sessionId || !payload.message.trim()) return;
    if (payload.message.trim().length > USER_INPUT_HARD_LIMIT_CHARS) {
      Notify.create({
        type: 'warning',
        message: t('chat.inputTooLong', { limit: USER_INPUT_HARD_LIMIT_CHARS }),
        position: 'top',
      });
      return;
    }
    try {
      await runtimeStore.enqueue(payload.sessionId, payload.message);
      Notify.create({ type: 'positive', message: t('chat.sessionStage.injectSent'), position: 'top' });
    } catch {
      Notify.create({ type: 'warning', message: t('chat.sessionStage.injectFailed'), position: 'top' });
    }
  }

  return {
    activeMember,
    agentMap,
    spiritStatusBar,
    uiConfig,
    resolveMemberChatSessionId: resolveMemberChatSessionIdBound,
    handlers: {
      onSelectSpirit,
      onNavigate,
      onOpenArtifactsPage,
      onErrorRelogin,
      onStatusBarClickRunning,
      onStatusBarClickInterrupted,
      onExpandMember,
      onEnterSession,
      onExpandChildren,
      onCancelAgent,
      onRetryAgent,
      onPauseAgent,
      onResumeAgent,
      onInjectAgent,
    },
  };
}
