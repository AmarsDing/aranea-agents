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
      @select-spirit="spirit.onSelectSpirit()"
      @select-agent="entity.selectAgent($event)"
      @agent-settings="(id: string) => entity.openSettings('agent', id)"
      @agent-delete="(id: string) => entity.openDelete('agent', id)"
      @agent-reorder="
        (payload: { groupKey: string; ids: string[] }) => entity.onGroupReorder(payload.groupKey, payload.ids)
      "
      @select-spirit-team="spiritStore.selectTeam($event)"
      @toggle-team-expand="spiritStore.toggleTeamExpand($event)"
      @spirit-settings="(id) => entity.openSettings('agent', id)"
      @locate-agent="spirit.onSidebarLocateAgent"
      @select-member="spirit.onSidebarSelectMember"
      @pause-agent="spirit.onSidebarPauseAgent"
      @resume-agent="spirit.onSidebarResumeAgent"
      @retry-agent="spirit.onSidebarRetryAgent"
      @cancel-agent="spirit.onSidebarCancelAgent"
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
        :kind="llmRetry.kind"
        :attempt="llmRetry.attempt"
        :max-retries="llmRetry.maxRetries"
        :delay-ms="llmRetry.delayMs"
        :error="llmRetry.error"
        :message="llmRetry.message"
        @dismiss="dismissLlmAlert"
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
        :dictating="composer.dictating"
        :dictation-partial="composer.dictationPartial"
        :skill-catalog="skillCatalog"
        :selected-skill-slugs="composer.selectedSkillSlugs"
        @toggle-skill="composer.toggleSkill"
        @clear-skills="composer.clearSkills"
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
        @add-to-eval="evalCase.openFromTask"
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
        @return-to-spirit="spirit.onSelectSpirit"
        @status-bar-click-running="spirit.onStatusBarClickRunning"
        @status-bar-click-interrupted="spirit.onStatusBarClickInterrupted"
        @toggle-view="spiritStore.toggleViewMode()"
        @toggle-composer="spiritStore.toggleComposer()"
        @expand-member="spirit.onExpandMember"
        @enter-session="spirit.onEnterSession"
        @cancel-agent="spirit.onCancelAgent"
        @retry-agent="spirit.onRetryAgent"
        @pause-agent="spirit.onPauseAgent"
        @resume-agent="spirit.onResumeAgent"
        @inject-agent="spirit.onInjectAgent"
        @expand="spirit.onExpandChildren"
      />
      <input ref="fileRef" type="file" hidden multiple :accept="session.fileAccept" @change="composer.onFileChange" />
    </div>

    <!-- 左侧成员卡片点击弹出的成员执行过程弹框（与 graph 成员行点击同一组件） -->
    <MemberSessionDialog
      v-model:open="sidebarMemberDialogOpen"
      :member-session="sidebarActiveMember"
      @pause-agent="spirit.onPauseAgent"
      @inject-agent="spirit.onInjectAgent"
      @expand="spirit.onExpandChildren"
      @confirm-step="session.onConfirmActivityGrant"
    />

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
      @select="spirit.onSelectSessionTreeNode"
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

      <AddEvalCaseDialog
        v-model:open="evalCase.open"
        :mode="evalCase.mode"
        :dataset-id="evalCase.datasetId"
        :dataset-options="evalCase.datasetOptions"
        :datasets-loading="evalCase.datasetsLoading"
        :new-dataset-name="evalCase.newDatasetName"
        :input="evalCase.input"
        :expected-output="evalCase.expectedOutput"
        :rubric="evalCase.rubric"
        :submitting="evalCase.submitting"
        @update:mode="evalCase.mode = $event"
        @update:dataset-id="evalCase.datasetId = $event"
        @update:new-dataset-name="evalCase.newDatasetName = $event"
        @update:input="evalCase.input = $event"
        @update:expected-output="evalCase.expectedOutput = $event"
        @update:rubric="evalCase.rubric = $event"
        @submit="evalCase.submit()"
      />
    </template>
  </ChatWorkspaceShell>
</template>

<script setup lang="ts">
import ChatDeleteDialog from '../components/chat/ChatDeleteDialog.vue';
import AddEvalCaseDialog from '../components/evaluation/AddEvalCaseDialog.vue';
import ChatEntitySidebar from '../components/chat/ChatEntitySidebar.vue';
import ChatMessagePanel from '../components/chat/ChatMessagePanel.vue';
import ChatSessionSidebar from '../components/chat/ChatSessionSidebar.vue';
import ChatSideToggle from '../components/chat/ChatSideToggle.vue';
import ChatSettingsDialog from '../components/chat/ChatSettingsDialog.vue';
import ChatWorkspaceShell from '../components/chat/ChatWorkspaceShell.vue';
import LlmRetryBanner from '../components/chat/LlmRetryBanner.vue';
import MemberSessionDialog from '../components/chat/v2/MemberSessionDialog.vue';
import SessionTimelineDialog from '../components/chat/SessionTimelineDialog.vue';
import SessionTreeSidebar from '../components/chat/SessionTreeSidebar.vue';
import { computed } from 'vue';
import { useRouter } from 'vue-router';
import { useChatWorkspace } from '../features/chat/composables/useChatWorkspace';
import { useChatSpiritPanel } from '../features/chat/composables/useChatSpiritPanel';
import { useBlockedStatus } from '../features/chat/composables/useBlockedStatus';
import { useSpiritTeamStore } from '../stores/spirit';
import { useUiConfigStore } from '../stores/uiConfig';
import { useChatRuntimeStore } from '../stores/chat/runtimeStore';
import { useLlmRetryStore } from '../stores/chat/llmRetryStore';
import type { Agent } from '../features/agents/types';

const workspace = useChatWorkspace();
const { coreReady, fileRef, layout, entity, session, composer, dialogs, errorBlock, evalCase } = workspace;
// Spirit/team 面板编排（状态栏聚合、Agent 卡片动作、成员会话定位）收口于 composable。
const spirit = useChatSpiritPanel(workspace);
// 注意：spirit 是普通对象，模板不会自动解包其嵌套 ref，必须解构为顶层绑定。
const {
  activeMember,
  showSessionTree,
  spiritStatusBar,
  pulseTeamColors,
  sidebarMemberDialogOpen,
  sidebarActiveMember,
} = spirit;
const spiritStore = useSpiritTeamStore();
const runtimeStore = useChatRuntimeStore();
const llmRetryStore = useLlmRetryStore();
/** Active LLM retry state for the current session — drives the reconnect banner. */
const llmRetry = computed(() => {
  const sid = session.selectedSessionForUi?.id;
  return sid ? llmRetryStore.retryFor(sid) : null;
});
function dismissLlmAlert() {
  const sid = session.selectedSessionForUi?.id;
  if (sid) llmRetryStore.clear(sid);
}
/** Design 69 Phase 3: agent-visible skill catalog for the current session
 *  (pushed via the skill.catalog WS event on connection setup). */
const skillCatalog = computed(() => {
  const sid = session.selectedSessionForUi?.id;
  return sid ? runtimeStore.skillCatalogFor(sid) : [];
});

const uiConfig = useUiConfigStore();
const blockedStatus = useBlockedStatus(computed(() => session.v2Tasks));
/** Exposed to template — auto-unwraps the ComputedRef<BlockedResult> to its value. */
const blockedInfo = blockedStatus.blockedInfo;
const router = useRouter();
// T5.5: Mobile (<1024px) responsive logic removed — app targets desktop only.

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

function onNavigate(route: { name: string; params: Record<string, string> }) {
  router.push(route);
}

/** 跳转制品管理页：自动填充当前会话筛选并切到「会话产物」Tab。 */
function onOpenArtifactsPage() {
  const sid = session.selectedSessionForUi?.id;
  if (!sid) return;
  void router.push({ name: 'artifacts', query: { session: sid } });
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
