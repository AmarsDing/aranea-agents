<!-- Container: approved because both chat pages share session, sender and confirmation orchestration. -->
<template>
  <q-page v-if="!coreReady" class="app-standard-page flex flex-center" data-workspace-bootstrap>
    <q-banner v-if="coreLoadError" class="bg-negative" role="alert">
      <template #avatar><q-icon name="cloud_off" /></template>
      {{ coreLoadError }}
      <template #action
        ><q-btn
          flat
          :label="layout.t('uiSystem.operations.retry')"
          :loading="coreLoading"
          @click="workspace.initializeWorkspace"
      /></template>
    </q-banner>
    <q-spinner-dots v-else size="40px" color="primary" :aria-label="layout.t('uiSystem.loadingWorkspace')" />
  </q-page>
  <component
    :is="taskMode ? TaskWorkspaceShell : ChatWorkspaceShell"
    v-else
    :pane="taskPane"
    :session-title="session.selectedSessionForUi?.title"
    :reading-expanded="readingExpanded && contextTab === 'artifacts' && taskContextOpen"
    :mobile-mode="route.path.startsWith('/mobile/')"
    :context-open="taskContextOpen"
    :context-tab="contextTab"
    @show-chat="taskPane = 'chat'"
    @show-navigation="showTaskNavigation"
    @show-artifacts="showTaskArtifacts"
    @show-activity="showTaskActivity"
    @context-visible="taskContextVisible = $event"
    @close-context="closeTaskArtifacts"
    @return-chat="returnToMobileChat"
  >
    <ChatEntitySidebar
      v-if="!taskMode || taskNavigation === 'members'"
      :class="{ 'task-navigation': taskMode }"
      :search="layout.search"
      :open="taskMode || layout.leftOpen"
      :agents="entity.displayAgents"
      :spirit-teams="spiritStore.sortedTeams"
      :expanded-team-ids="spiritStore.expandedTeamIds"
      :selected-kind="spiritStore.activePanelMode === 'spirit' ? 'spirit' : entity.selectedEntityKind"
      :selected-agent-id="entity.store.selectedAgent?.id"
      :selected-team-id="spiritStore.activeTeamId"
      :default-agent-id="
        (entity.store.agents.find((a: Agent) => a.agent_key === SPIRIT_AGENT_KEY) || entity.store.agents[0])?.id
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
      v-if="!taskMode"
      :open="layout.leftOpen"
      :icon="layout.leftOpen ? 'chevron_left' : 'chevron_right'"
      :aria-label="layout.t('chat.collapseList')"
      @toggle="layout.leftOpen = !layout.leftOpen"
    />

    <div class="chat-workspace-main col column no-wrap" :class="{ 'task-conversation': taskMode }">
      <q-banner v-if="taskMode && locationError" dense rounded class="app-banner-warning q-mx-sm q-mt-sm" role="alert">
        {{ locationError }}
        <template #action
          ><q-btn flat dense no-caps :label="layout.t('taskWorkspace.retry')" @click="retryLocation" /><q-btn
            flat
            dense
            no-caps
            :label="layout.t('taskWorkspace.returnToCurrent')"
            @click="clearLocation"
        /></template>
      </q-banner>
      <q-banner v-else-if="taskMode && locationBusy" dense class="app-info-banner q-mx-sm" role="status">{{
        layout.t('taskWorkspace.locating')
      }}</q-banner>
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
        :reconnect-error="session.reconnectError"
        :draft-save-failed="composer.draftSaveFailed"
        :initial-scroll-top="taskMode ? chatScrollTop : undefined"
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
        :submit-pending="composer.submitting"
        :input-disabled="composer.inputDisabled || (taskMode && Boolean(locationError || locationBusy))"
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
        :workspace-mode="taskMode"
        @retry-sync="session.retryReconnect"
        @scroll-position="chatScrollTop = $event"
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
        @attachment-deleted="session.onArtifactDeleted"
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
        @fork-turn="session.onForkSessionTurn"
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
      v-if="!taskMode"
      :open="layout.rightOpen"
      :icon="layout.rightOpen ? 'chevron_right' : 'chevron_left'"
      :aria-label="layout.t('chat.collapseSession')"
      @toggle="layout.rightOpen = !layout.rightOpen"
    />

    <ChatSessionSidebar
      v-if="taskMode ? taskNavigation === 'sessions' : !showSessionTree"
      :class="{ 'task-navigation': taskMode }"
      :open="taskMode || layout.rightOpen"
      :sessions="session.displaySessions"
      :inbox-sessions="session.inboxSessions"
      :selected-session-id="session.selectedSessionForUi?.id"
      :is-dark="layout.isDark"
      :favorite-ids="session.favoriteIds"
      :total-count="session.sessionListTotal"
      :has-more="session.sessionListHasMore"
      :loading="session.sessionListLoading"
      :loading-more="session.sessionListLoadingMore"
      :search-keyword="session.sessionListKeyword"
      :searchable="entity.selectedEntityKind === 'agent'"
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
      @load-more="session.onSessionListLoadMore"
      @search="session.onSessionListSearch"
    />
    <SessionTreeSidebar
      v-else-if="!taskMode || taskNavigation === 'tree'"
      :class="{ 'task-navigation': taskMode }"
      :tree-nodes="session.sessionTree.spiritTreeNodes"
      :active-session-id="session.selectedSessionForUi?.id ?? ''"
      :default-expanded="true"
      @select="spirit.onSelectSessionTreeNode"
    />

    <ChatArtifactPanel
      v-if="taskMode && taskContextOpen && contextTab === 'artifacts'"
      id="task-workspace-context"
      class="task-context"
      :session-id="session.selectedSessionForUi?.id"
      :artifacts="session.sessionArtifacts"
      :total="session.sessionArtifactsTotal"
      :loading="session.sessionArtifactsLoading"
      :loading-more="session.sessionArtifactsLoadingMore"
      :has-more="session.sessionArtifactsHasMore"
      :error="session.sessionArtifactsError"
      :target="taskPanelTarget"
      :preview-active="taskContextVisible"
      :reading-expanded="readingExpanded"
      :scroll-top="contextScrollTop"
      :can-manage="!route.path.startsWith('/mobile/')"
      :versions="previewVersions"
      :versions-loading="versionsLoading"
      :versions-error="versionsError"
      @toggle-reading="readingExpanded = !readingExpanded"
      @scroll-position="saveContextScroll"
      @refresh="session.reloadSessionArtifacts"
      @load-more="session.loadMoreSessionArtifacts"
      @close="closeTaskArtifacts"
      @back="artifactStore.closeArtifactPreview"
      @preview="(meta) => artifactStore.openArtifactPreview(meta.id, meta.version)"
      @download="session.downloadArtifact"
      @manage="session.goArtifactsPage"
      @quote="onQuoteSelection"
      @reload-versions="reloadVersions"
    />
    <TaskWorkspaceActivityPanel
      v-else-if="taskMode && taskContextOpen"
      id="task-workspace-context"
      class="task-context"
      :tasks="workspaceTasks"
      :pending-steps="pendingSteps"
      :incomplete="activityIncomplete"
      :process="processView"
      :executor-names="agentMap"
      @select-task="selectProcessTask"
      @select-turn="selectProcessTurn"
      @follow-current="onFollowCurrentProcess"
      @retry-process="retryProcess"
      @locate-turn="locateWorkspaceTurn"
      @open-member="spirit.openTaskMember"
      @locate="locateWorkspaceTask"
      @copy-link="copyTaskLink"
      @close="closeTaskArtifacts"
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

      <!-- P0 会话产物抽屉：会话内列出产物，点击经全局预览弹窗查看 -->
      <SessionArtifactsDrawer
        v-if="!taskMode"
        :open="session.artifactsDrawerOpen"
        :artifacts="session.sessionArtifacts"
        :loading="session.sessionArtifactsLoading"
        @update:open="session.artifactsDrawerOpen = $event"
        @preview="(meta: ArtifactMeta) => artifactStore.openArtifactPreview(meta.id, meta.version)"
        @download="session.downloadArtifact"
        @manage="session.goArtifactsPage"
      />

      <AddEvalCaseDialog
        v-if="!injectedWorkspace"
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
  </component>
</template>

<script setup lang="ts">
import ChatDeleteDialog from '../../components/chat/ChatDeleteDialog.vue';
import ChatArtifactPanel from '../../components/chat/ChatArtifactPanel.vue';
import AddEvalCaseDialog from '../../components/evaluation/AddEvalCaseDialog.vue';
import SessionArtifactsDrawer from '../../components/artifact/SessionArtifactsDrawer.vue';
import ChatEntitySidebar from '../../components/chat/ChatEntitySidebar.vue';
import ChatMessagePanel from '../../components/chat/ChatMessagePanel.vue';
import ChatSessionSidebar from '../../components/chat/ChatSessionSidebar.vue';
import ChatSideToggle from '../../components/chat/ChatSideToggle.vue';
import ChatSettingsDialog from '../../components/chat/ChatSettingsDialog.vue';
import ChatWorkspaceShell from '../../components/chat/ChatWorkspaceShell.vue';
import TaskWorkspaceShell from '../../components/chat/TaskWorkspaceShell.vue';
import LlmRetryBanner from '../../components/chat/LlmRetryBanner.vue';
import MemberSessionDialog from '../../components/chat/v2/MemberSessionDialog.vue';
import SessionTimelineDialog from '../../components/chat/SessionTimelineDialog.vue';
import SessionTreeSidebar from '../../components/chat/SessionTreeSidebar.vue';
import { computed, inject, nextTick, ref } from 'vue';
import TaskWorkspaceActivityPanel from '../../components/chat/TaskWorkspaceActivityPanel.vue';
import { useTaskWorkspaceActivity } from './composables/useTaskWorkspaceActivity';
import { useTaskWorkspaceProcess } from './composables/useTaskWorkspaceProcess';
import { useScrollToActivity } from './composables/useScrollToActivity';
import { useRoute, useRouter } from 'vue-router';
import { CHAT_WORKSPACE_KEY } from './composables/chatWorkspaceInjection';
import { useTaskWorkspaceLocation } from './composables/useTaskWorkspaceLocation';
import { SPIRIT_AGENT_KEY } from '../spirit/types';
import { useChatWorkspace } from './composables/useChatWorkspace';
import { useChatSpiritPanel } from './composables/useChatSpiritPanel';
import { useBlockedStatus } from './composables/useBlockedStatus';
import { useTaskWorkspaceView } from './composables/useTaskWorkspaceView';
import { useSpiritTeamStore } from '../../stores/spirit';
import { useUiConfigStore } from '../../stores/uiConfig';
import { useChatRuntimeStore } from '../../stores/chat/runtimeStore';
import { useLlmRetryStore } from '../../stores/chat/llmRetryStore';
import { useArtifactStore } from '../../stores/artifact';
import type { Agent } from '../agents/types';
import type { ArtifactMeta, ArtifactTextQuote } from '../artifact/types';

const props = withDefaults(defineProps<{ taskMode?: boolean }>(), { taskMode: false });
const route = useRoute();
const router = useRouter();
const taskContextVisible = ref(false);
const injectedWorkspace = inject(CHAT_WORKSPACE_KEY, null);
const workspace = injectedWorkspace ?? useChatWorkspace();
const {
  coreReady,
  coreLoading,
  coreLoadError,
  fileRef,
  layout,
  entity,
  session,
  composer,
  dialogs,
  errorBlock,
  evalCase,
} = workspace;
const artifactStore = useArtifactStore();
const {
  tasks: workspaceTasks,
  pendingSteps,
  incomplete: activityIncomplete,
} = useTaskWorkspaceActivity(() => session.selectedSessionForUi?.id);
const { locateTask, locateTurn } = useScrollToActivity();
const {
  pane: taskPane,
  navigation: taskNavigation,
  contextOpen: taskContextOpen,
  contextTab,
  readingExpanded,
  chatScrollTop,
  processTaskId,
  processTurnId,
  contextScrollTop,
  saveContextScroll,
  panelTarget: taskPanelTarget,
  openArtifacts: openTaskArtifacts,
  closeArtifacts: closeTaskArtifacts,
  showNavigation: showTaskNavigation,
  previewVersions,
  versionsLoading,
  versionsError,
  reloadVersions,
} = useTaskWorkspaceView(
  () => props.taskMode && coreReady.value && !session.navigationLoading,
  () => session.selectedSessionForUi?.id,
);
const {
  view: processView,
  selectTask: selectProcessTask,
  selectTurn: selectProcessTurn,
  selectLocation: selectProcessLocation,
  followCurrent: followCurrentProcess,
  retry: retryProcess,
} = useTaskWorkspaceProcess({
  enabled: () =>
    props.taskMode &&
    coreReady.value &&
    !session.navigationLoading &&
    taskContextVisible.value &&
    contextTab.value === 'activity',
  sessionId: () => session.selectedSessionForUi?.id,
  taskId: processTaskId,
  turnId: processTurnId,
});
function showTaskArtifacts() {
  contextTab.value = 'artifacts';
  openTaskArtifacts();
}
function showTaskActivity() {
  contextTab.value = 'activity';
  openTaskArtifacts();
}
async function locateWorkspaceTask(taskId: string, stepId?: string, isCurrent = () => true) {
  const sid = session.selectedSessionForUi?.id;
  selectProcessLocation(taskId, stepId);
  taskPane.value = 'chat';
  await nextTick();
  if (isCurrent() && sid && sid === session.selectedSessionForUi?.id) locateTask(sid, taskId, stepId);
}
async function locateWorkspaceTurn(taskId: string, turnId: string) {
  const sid = session.selectedSessionForUi?.id;
  if (
    !processView.value.canLocateTurn ||
    processView.value.task?.ID !== taskId ||
    processView.value.turn?.ID !== turnId
  )
    return;
  taskPane.value = 'chat';
  await nextTick();
  if (sid && sid === session.selectedSessionForUi?.id) locateTurn(sid, taskId, turnId);
}
async function onFollowCurrentProcess() {
  const sid = session.selectedSessionForUi?.id;
  followCurrentProcess();
  taskPane.value = 'chat';
  await nextTick();
  const task = processView.value.task;
  if (sid && sid === session.selectedSessionForUi?.id && task && !processView.value.manual) locateTask(sid, task.ID);
}
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
const {
  error: locationError,
  busy: locationBusy,
  retry: retryLocation,
  copyTaskLink,
} = useTaskWorkspaceLocation({
  enabled: () => props.taskMode,
  ready: () => coreReady.value && !session.sessionLoading,
  sessionId: () => session.selectedSessionForUi?.id,
  bindingLoading: () => session.navigationLoading,
  bindingError: () => session.navigationError,
  showPanel: (panel) => {
    if (panel === 'activity') showTaskActivity();
    else if (panel === 'artifacts') showTaskArtifacts();
    else taskPane.value = 'chat';
  },
  locate: locateWorkspaceTask,
});
function clearLocation() {
  void router.replace({
    name: route.name!,
    query: currentSessionQuery(),
  });
}
function currentSessionQuery() {
  const selected = session.selectedSessionForUi;
  return selected
    ? {
        session: selected.id,
        ...(entity.selectedEntityKind === 'team' && entity.selectedTeamId
          ? { team: entity.selectedTeamId }
          : { agent: selected.agent_id }),
      }
    : {};
}
function returnToMobileChat() {
  const selected = session.selectedSessionForUi;
  void router.push(selected ? { name: 'mobile-chat', query: currentSessionQuery() } : { name: 'mobile-sessions' });
}

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

/**
 * P0 会话产物点击查看（2026-09-01）：header 产物按钮默认打开会话内
 * 产物抽屉（点击条目即预览）；「管理全部产物」由抽屉底部入口跳页。
 */
function onOpenArtifactsPage() {
  if (props.taskMode) {
    showTaskArtifacts();
    void session.reloadSessionArtifacts();
  } else session.openArtifactsDrawer();
}

function onQuoteSelection(quote: ArtifactTextQuote) {
  if (quote.meta.session_id !== session.selectedSessionForUi?.id) return;
  const label = layout.t('taskWorkspace.quoteSource', { name: quote.meta.name, version: quote.meta.version });
  const text = `${label}\n${quote.text
    .split('\n')
    .map((line) => `> ${line}`)
    .join('\n')}`;
  composer.inputText = composer.inputText.trimEnd() ? `${composer.inputText.trimEnd()}\n\n${text}` : text;
  taskPane.value = 'chat';
  void nextTick(() => {
    const input = document.querySelector<HTMLTextAreaElement>('.task-workspace .composer-input textarea');
    input?.focus();
    input?.setSelectionRange(input.value.length, input.value.length);
  });
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
