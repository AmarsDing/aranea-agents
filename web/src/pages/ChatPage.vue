<template>
  <div v-if="!coreReady" class="flex flex-center" style="height: 100vh">
    <q-spinner-dots size="40px" color="accent" />
  </div>
  <ChatWorkspaceShell v-else>
    <ChatEntitySidebar
      :search="layout.search"
      :open="layout.leftOpen"
      :agents="entity.displayAgents"
      :spirit-teams="spiritStore.sortedTeams"
      :expanded-team-ids="spiritStore.expandedTeamIds"
      :selected-kind="spiritStore.activePanelMode === 'spirit' ? 'spirit' : entity.selectedEntityKind"
      :selected-agent-id="entity.store.selectedAgent?.id"
      :selected-team-id="spiritStore.activeTeamId"
      :default-agent-id="
        (entity.store.agents.find((a: Agent) => a.agent_key === '__spirit__') || entity.store.agents[0])?.id
      "
      :is-dark="layout.isDark"
      :pulse-team-colors="pulseTeamColors"
      :spirit-mode="spiritStore.activePanelMode === 'spirit'"
      :orchestration-phase="spiritStore.orchestrationPhase"
      :blocked-status="blockedInfo"
      @update:search="layout.search = $event"
      @select-spirit="onSelectSpirit()"
      @select-agent="entity.selectAgent($event)"
      @agent-settings="(id: string) => entity.openSettings('agent', id)"
      @agent-delete="(id: string) => entity.openDelete('agent', id)"
      @agent-reorder="
        (payload: { groupKey: string; ids: string[] }) => entity.onGroupReorder(payload.groupKey, payload.ids)
      "
      @select-spirit-team="spiritStore.selectTeam($event)"
      @toggle-team-expand="spiritStore.toggleTeamExpand($event)"
      @spirit-settings="(id) => entity.openSettings('agent', id)"
      @locate-agent="onSidebarLocateAgent"
      @pause-agent="onSidebarPauseAgent"
      @resume-agent="onSidebarResumeAgent"
      @retry-agent="onSidebarRetryAgent"
      @cancel-agent="onSidebarCancelAgent"
    />

    <ChatSideToggle
      :open="layout.leftOpen"
      :icon="layout.leftOpen ? 'chevron_left' : 'chevron_right'"
      :aria-label="layout.t('chat.collapseList')"
      @toggle="layout.leftOpen = !layout.leftOpen"
    />

    <div class="chat-workspace-main col column no-wrap">
      <q-banner v-if="session.inboundHydrateError" dense rounded class="app-banner-warning q-mx-sm q-mt-sm">
        {{ session.inboundHydrateError }}
      </q-banner>
      <LlmRetryBanner
        v-if="llmRetry"
        :attempt="llmRetry.attempt"
        :max-retries="llmRetry.maxRetries"
        :delay-ms="llmRetry.delayMs"
        :error="llmRetry.error"
      />
      <ChatMessagePanel
        v-model="composer.inputText"
        :dialog-mode="composer.dialogMode"
        :model-provider="composer.modelProvider"
        :panel-mode="spiritStore.activePanelMode"
        :spirit-team="spiritStore.activeTeam"
        :active-member="activeMember"
        :messages="session.displayMessages"
        :attachments="composer.attachments"
        :mode-options="composer.modeOpts"
        :provider-options="composer.provOpts"
        :session-title="session.selectedSessionForUi?.title || layout.t('chat.untitledSession')"
        :session-id="session.selectedSessionForUi?.id"
        :context-ratio="session.selectedSessionForUi?.context_used_ratio ?? 0"
        :context-status="session.selectedSessionForUi?.context_status"
        :context-breakdown="session.contextBreakdown"
        :is-dark="layout.isDark"
        :sending="composer.sending"
        :input-disabled="composer.inputDisabled"
        :is-runner-active="composer.isRunnerActive"
        :is-awaiting-user="composer.isAwaitingUser"
        :ws-replaying="session.wsReplaying"
        :spirit-loading-message="session.spiritLoadingMessage"
        :spirit-status-bar="spiritStatusBar"
        :compress-status="session.compressStatus"
        :show-tool-calls="uiConfig.showToolCalls"
        :session-loading="session.sessionLoading"
        :session-revision="session.sessionRevision"
        :ws-connected="session.wsConnected"
        :is-team-session="entity.selectedEntityKind === 'team'"
        :planner-kind="entity.activePlannerKind"
        :focus-turn-id="session.focusTurnId"
        :session-artifacts="session.sessionArtifacts"
        :session-artifacts-loading="session.sessionArtifactsLoading"
        :file-supported="session.fileSupported"
        :file-accept="session.fileAccept"
        :show-background-jobs="entity.selectedEntityKind === 'agent'"
        :agent-id="entity.store.selectedAgent?.id"
        :jobs-refresh-nonce="session.jobsRefreshNonce"
        :view-mode="spiritStore.viewMode"
        :composer-visible="spiritStore.composerVisible"
        :reasoning-sidebar-open="session.reasoningSidebarOpen"
        :reasoning-sidebar-active="session.reasoningSidebarActive"
        :pending-messages="composer.pendingMessages"
        :agent-map="agentMap"
        :run-status="composer.runStatus"
        :run-agent-name="composer.runMeta?.agentName"
        :run-started-at="composer.runMeta?.startedAt"
        :run-event-count="composer.runMeta?.eventCount"
        :show-enqueue="composer.isRunnerActive"
        @enqueue-message="composer.onEnqueueWhileRunning"
        @update:dialog-mode="composer.onModeChange"
        @update:model-provider="composer.onProviderChange"
        @remove-attachment="composer.removeAttachment"
        @pick-file="composer.pickFile"
        @paste-file="composer.uploadFile"
        @voice="composer.onVoiceClick"
        @send="composer.onSend"
        @stop="composer.stopStreaming"
        @cancel-pending="composer.onCancelPending"
        @interrupt-pending="composer.onInterruptPending"
        @update-pending="composer.onUpdatePending"
        @submit-await-reply="composer.submitAwaitingReply"
        @open-events="session.openSessionEvents"
        @open-artifacts-page="onOpenArtifactsPage"
        @open-artifact="session.openSessionArtifact"
        @attachment-deleted="session.onArtifactDeleted"
        @download-artifact="session.downloadArtifact"
        @focus-turn="session.focusSessionTurn"
        @navigate="onNavigate"
        @cancel-job="composer.cancelBackgroundJob"
        @paste-unsupported="composer.onPasteUnsupported"
        @new-session="session.onNewSession"
        @focus-turn-cleared="session.clearFocusTurn"
        @toggle-reasoning-sidebar="session.toggleReasoningSidebar"
        @pin-reasoning-message="session.pinReasoningMessage"
        @close-reasoning-sidebar="session.toggleReasoningSidebar"
        @a2ui-user-action="composer.submitA2UIUserAction"
        @feedback="composer.onMessageFeedback"
        @retry="composer.retryFailedMessage"
        @dismiss-failed="composer.dismissFailedMessage"
        @regenerate="composer.regenerateMessage"
        @regenerate-v2="composer.regenerateV2Task"
        @resume-task="session.resumeTask"
        @compact="session.onCompactSession"
        @toggle-tool-calls="uiConfig.setShowToolCalls(!uiConfig.showToolCalls)"
        @confirm-activity="session.onConfirmActivity"
        @confirm-activity-grant="session.onConfirmActivityGrant"
        @submit-clarification="session.onSubmitClarification"
        @error-retry="errorBlock.onErrorRetry"
        @error-switch-model="errorBlock.onErrorSwitchModel"
        @error-rephrase="errorBlock.onErrorRephrase"
        @error-check-config="errorBlock.onErrorCheckConfig"
        @error-remove-attachment="errorBlock.onErrorRemoveAttachment"
        @error-relogin="onErrorRelogin"
        @cancel-team="spiritStore.cancelTeam"
        @pause-team="spiritStore.pauseTeam"
        @unpause-team="spiritStore.unpauseTeam"
        @inject-team="(p: { teamId: string; message: string }) => spiritStore.injectTeam(p.teamId, p.message)"
        @archive-team="spiritStore.archiveTeam"
        @select-member="spiritStore.selectMember"
        @select-spirit-team="spiritStore.selectTeam($event)"
        @return-to-team="spiritStore.activeTeamId ? spiritStore.selectTeam(spiritStore.activeTeamId) : undefined"
        @return-to-spirit="onSelectSpirit"
        @status-bar-click-running="onStatusBarClickRunning"
        @status-bar-click-interrupted="onStatusBarClickInterrupted"
        @toggle-view="spiritStore.toggleViewMode()"
        @toggle-composer="spiritStore.toggleComposer()"
        @expand-member="onExpandMember"
        @enter-session="onEnterSession"
        @cancel-agent="onCancelAgent"
        @retry-agent="onRetryAgent"
        @pause-agent="onPauseAgent"
        @resume-agent="onResumeAgent"
        @inject-agent="onInjectAgent"
        @expand="onExpandChildren"
      />
      <input ref="fileRef" type="file" hidden multiple :accept="session.fileAccept" @change="composer.onFileChange" />
    </div>

    <ChatSideToggle
      :open="layout.rightOpen"
      :icon="layout.rightOpen ? 'chevron_right' : 'chevron_left'"
      :aria-label="layout.t('chat.collapseSession')"
      @toggle="layout.rightOpen = !layout.rightOpen"
    />

    <ChatSessionSidebar
      v-if="!showSessionTree"
      :open="layout.rightOpen"
      :sessions="session.displaySessions"
      :inbox-sessions="session.inboxSessions"
      :selected-session-id="session.selectedSessionForUi?.id"
      :is-dark="layout.isDark"
      :favorite-ids="session.favoriteIds"
      @select="session.onSelectSession"
      @new-session="session.onNewSession"
      @rename="session.onRenameSession"
      @toggle-pin="session.onTogglePinSession"
      @toggle-favorite="session.onToggleFavorite"
      @trace="session.openSessionTrace"
      @delete="entity.openDelete"
      @restore="session.onRestoreSession"
      @archive="session.onArchiveSession"
      @detail="session.onSessionDetail"
    />
    <SessionTreeSidebar
      v-else
      :tree-nodes="session.sessionTree.spiritTreeNodes"
      :active-session-id="session.selectedSessionForUi?.id ?? ''"
      :default-expanded="true"
      @select="onSelectSessionTreeNode"
    />

    <template #dialogs>
      <ChatSettingsDialog
        v-model="dialogs.settingsOpen"
        :name="dialogs.editName"
        :provider="dialogs.editProvider"
        :model="dialogs.editModel"
        :title="dialogs.settingsTitle"
        :mode="dialogs.settingsMode"
        :agent-key="dialogs.editKey"
        :saving="dialogs.settingsSaving"
        @update:name="dialogs.editName = $event"
        @update:provider="dialogs.editProvider = $event"
        @update:model="dialogs.editModel = $event"
        @save="dialogs.onSaveSettings"
      />

      <ChatDeleteDialog
        v-model="dialogs.deleteOpen"
        :name-input="dialogs.deleteNameInput"
        :title="dialogs.deleteTitleText"
        :kind="dialogs.deleteKind"
        :expected-name="dialogs.expectedDeleteName"
        :blocked-busy="dialogs.deleteBlockBusy"
        :can-confirm="dialogs.canConfirmDelete"
        :has-name-error="Boolean(dialogs.deleteNameError && dialogs.deleteNameInput)"
        :deleting="dialogs.deleting"
        @update:name-input="dialogs.deleteNameInput = $event"
        @confirm="dialogs.onConfirmDelete"
      />

      <SessionTimelineDialog
        v-model="dialogs.traceOpen"
        :session-id="dialogs.traceSessionId"
        :session-title="dialogs.traceSessionTitle"
        :initial-tab="dialogs.traceInitialTab"
        :stream-deps="dialogs.traceStreamDeps"
        :timeline="dialogs.timeline"
        :timeline-loading="dialogs.timelineLoading"
        :timeline-error="dialogs.timelineError"
        @refresh-trace="dialogs.reloadTimeline()"
      />
    </template>
  </ChatWorkspaceShell>
</template>

<script setup lang="ts">
import ChatDeleteDialog from '../components/chat/ChatDeleteDialog.vue';
import ChatEntitySidebar from '../components/chat/ChatEntitySidebar.vue';
import ChatMessagePanel from '../components/chat/ChatMessagePanel.vue';
import ChatSessionSidebar from '../components/chat/ChatSessionSidebar.vue';
import ChatSideToggle from '../components/chat/ChatSideToggle.vue';
import ChatSettingsDialog from '../components/chat/ChatSettingsDialog.vue';
import ChatWorkspaceShell from '../components/chat/ChatWorkspaceShell.vue';
import LlmRetryBanner from '../components/chat/LlmRetryBanner.vue';
import SessionTimelineDialog from '../components/chat/SessionTimelineDialog.vue';
import SessionTreeSidebar from '../components/chat/SessionTreeSidebar.vue';
import { USER_INPUT_HARD_LIMIT_CHARS } from '../features/chat/composables/useChatSender';
import { computed, watch } from 'vue';
import { useRouter } from 'vue-router';
import { Notify } from 'quasar';
import { useChatWorkspace } from '../features/chat/composables/useChatWorkspace';
import { useScrollToActivity } from '../features/chat/composables/useScrollToActivity';
import { useBlockedStatus } from '../features/chat/composables/useBlockedStatus';
import { useSpiritTeamStore } from '../stores/spirit';
import { useUiConfigStore } from '../stores/uiConfig';
import { useChatRuntimeStore } from '../stores/chat/runtimeStore';
import { useChatActivityStore } from '../stores/chat/activityV2Store';
import { useLlmRetryStore } from '../stores/chat/llmRetryStore';
import type { Agent } from '../features/agents/types';

const SPIRIT_AGENT_KEY = '__spirit__';

const { coreReady, fileRef, layout, entity, session, composer, dialogs, errorBlock } = useChatWorkspace();
const spiritStore = useSpiritTeamStore();
const runtimeStore = useChatRuntimeStore();
const llmRetryStore = useLlmRetryStore();
/** Active LLM retry state for the current session — drives the reconnect banner. */
const llmRetry = computed(() => {
  const sid = session.selectedSessionForUi?.id;
  return sid ? llmRetryStore.retryFor(sid) : null;
});
const { locate } = useScrollToActivity();
const uiConfig = useUiConfigStore();
const blockedStatus = useBlockedStatus(computed(() => session.v2Tasks));
/** Exposed to template — auto-unwraps the ComputedRef<BlockedResult> to its value. */
const blockedInfo = blockedStatus.blockedInfo;
const router = useRouter();
// T5.5: Mobile (<1024px) responsive logic removed — app targets desktop only.

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
    complexityLevel: spiritStore.planCreated?.complexity_level ?? null,
    complexityReason: spiritStore.planCreated?.strategy_reason ?? null,
    checkpointStep: spiritStore.lastCheckpoint?.step ?? null,
    dqScore: spiritStore.lastDqScore?.overall ?? null,
  };
});

const agentMap = computed(() => {
  const map = new Map<string, { displayName: string; agentKey: string }>();
  for (const agent of entity.store.agents) {
    if (agent?.agent_key) {
      map.set(agent.agent_key, {
        displayName: agent.display_name || agent.agent_key,
        agentKey: agent.agent_key,
      });
    }
  }
  return map;
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

/**>/** T8.6: 点击左侧 Agent 卡片定位到中间面板会话 */
function onSidebarLocateAgent(payload: { agentKey: string; teamSessionId: string; teamId: string }) {
  locate(payload.agentKey, payload.teamSessionId, payload.teamId);
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
  const store = useChatActivityStore();
  for (const ms of store.memberSessions.values()) {
    if (ms.SessionID === sessionId) {
      const prev = ms.Status;
      store.upsertMemberSession({ ...ms, Status: status });
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

function onNavigate(route: { name: string; params: Record<string, string> }) {
  router.push(route);
}

/** 跳转制品管理页：自动填充当前会话筛选并切到「会话产物」Tab。 */
function onOpenArtifactsPage() {
  const sid = session.selectedSessionForUi?.id;
  if (!sid) return;
  void router.push({ name: 'artifacts', query: { session: sid } });
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

/**
 * P3-4: ErrorBlock inline action handlers.
 *
 * ErrorBlock emits typed actions based on the resolved `errorCode`. Most
 * handlers are implemented in `useChatWorkspace` (errorBlock) so they can
 * call `$q.notify` directly. The `onErrorRelogin` handler stays here
 * because it only needs `router.push` (no notification).
 */

/** Relogin: redirect to the login page. */
function onErrorRelogin() {
  router.push({ name: 'login' });
}
</script>
