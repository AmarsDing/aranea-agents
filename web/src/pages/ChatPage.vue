<template>
  <ChatWorkspaceShell>
    <ChatEntitySidebar
      v-model:search="search"
      v-model:agents="displayAgents"
      v-model:teams="displayTeams"
      :open="leftOpen"
      :selected-kind="selectedEntityKind"
      :selected-agent-id="store.selectedAgent?.id"
      :selected-team-id="selectedTeamId"
      :category-tree="categoryTree"
      :is-dark="isDark"
      @agent-reorder-end="onEndAgent"
      @team-reorder-end="onEndTeam"
      @select-agent="selectAgent"
      @select-team="selectTeam"
      @settings="openSettings"
      @delete="openDelete"
    />

    <ChatSideToggle
      :open="leftOpen"
      :icon="leftOpen ? 'chevron_left' : 'chevron_right'"
      :ariaLabel="t('chat.collapseList')"
      @toggle="leftOpen = !leftOpen"
    />

    <div class="chat-workspace-main col column no-wrap">
      <ChatMessagePanel
        v-model="inputText"
        v-model:dialog-mode="dialogMode"
        v-model:model-provider="modelProvider"
        :messages="displayMessages"
        :attachments="attachments"
        :mode-options="modeOpts"
        :provider-options="provOpts"
        :session-title="selectedSessionForUi?.title || t('chat.untitledSession')"
        :context-ratio="selectedSessionForUi?.context_used_ratio ?? 0"
        :is-dark="isDark"
        :sending="sending"
        :input-disabled="inputDisabled"
        :is-awaiting-user="isAwaitingUser"
        :await-kind="awaitKind"
        :await-tool-key="awaitToolKey"
        :ws-replaying="wsReplaying"
        :session-revision="sessionRevision"
        :ws-connected="wsConnected"
        :is-team-session="selectedEntityKind === 'team'"
        :planner-kind="activePlannerKind"
        :react-tool-link-index="reactToolLinkIndex"
        :pending-messages="pendingMessages"
        :run-status="runStatus"
        :run-agent-name="runMeta?.agentName"
        :run-started-at="runMeta?.startedAt"
        :run-event-count="runMeta?.eventCount"
        :show-enqueue="isRunnerActive"
        @enqueue-message="onEnqueueWhileRunning"
        @update:dialog-mode="onModeChange"
        @update:model-provider="onProviderChange"
        @remove-attachment="removeAttachment"
        @pick-file="pickFile"
        @voice="onVoiceClick"
        @send="onSend"
        @stop="stopStreaming"
        @cancel-pending="onCancelPending"
        @update-pending="onUpdatePending"
        @submit-await-reply="submitAwaitingReply"
        @submit-tool-confirm="submitToolConfirm"
        @open-events="openSessionEvents"
        @a2ui-user-action="submitA2UIUserAction"
      />
      <chat-session-artifacts-panel
        :session-id="selectedSessionForUi?.id ?? ''"
        :items="sessionArtifacts"
        :loading="sessionArtifactsLoading"
        @open="openSessionArtifact"
      />
      <ChatBackgroundJobsPanel
        v-if="selectedEntityKind === 'agent'"
        :session-id="selectedSessionForUi?.id"
        :agent-id="store.selectedAgent?.id"
        :refresh-nonce="jobsRefreshNonce"
      />
      <input ref="fileRef" type="file" hidden multiple @change="onFileChange" />
    </div>

    <ChatSideToggle
      :open="rightOpen"
      :icon="rightOpen ? 'chevron_right' : 'chevron_left'"
      :ariaLabel="t('chat.collapseSession')"
      @toggle="rightOpen = !rightOpen"
    />

    <ChatSessionSidebar
      :open="rightOpen"
      :sessions="displaySessions"
      :selected-session-id="selectedSessionForUi?.id"
      :is-dark="isDark"
      @select="onSelectSession"
      @new-session="onNewSession"
      @rename="onRenameSession"
      @trace="openSessionTrace"
      @delete="openDelete"
      @restore="onRestoreSession"
      @archive="onArchiveSession"
      @detail="onSessionDetail"
    />

    <template #dialogs>
      <ChatSettingsDialog
        v-model="settingsOpen"
        v-model:name="editName"
        v-model:provider="editProvider"
        v-model:model="editModel"
        :title="settingsTitle"
        :mode="settingsMode"
        :agent-key="editKey"
        :saving="settingsSaving"
        @save="onSaveSettings"
      />

      <ChatDeleteDialog
        v-model="deleteOpen"
        v-model:name-input="deleteNameInput"
        :title="deleteTitleText"
        :kind="deleteKind"
        :expected-name="expectedDeleteName"
        :blocked-busy="deleteBlockBusy"
        :can-confirm="canConfirmDelete"
        :has-name-error="Boolean(deleteNameError && deleteNameInput)"
        :deleting="deleting"
        @confirm="onConfirmDelete"
      />

      <SessionTimelineDialog
        v-model="traceOpen"
        :session-id="traceSessionId"
        :session-title="traceSessionTitle"
        :initial-tab="traceInitialTab"
        :stream-deps="traceStreamDeps"
      />
    </template>
  </ChatWorkspaceShell>
</template>

<script setup lang="ts">
import ChatDeleteDialog from "../components/chat/ChatDeleteDialog.vue";
import ChatEntitySidebar from "../components/chat/ChatEntitySidebar.vue";
import ChatMessagePanel from "../components/chat/ChatMessagePanel.vue";
import ChatSessionSidebar from "../components/chat/ChatSessionSidebar.vue";
import ChatSideToggle from "../components/chat/ChatSideToggle.vue";
import ChatSettingsDialog from "../components/chat/ChatSettingsDialog.vue";
import ChatWorkspaceShell from "../components/chat/ChatWorkspaceShell.vue";
import ChatSessionArtifactsPanel from "../components/chat/ChatSessionArtifactsPanel.vue";
import ChatBackgroundJobsPanel from "../components/chat/ChatBackgroundJobsPanel.vue";
import SessionTimelineDialog from "../components/chat/SessionTimelineDialog.vue";
import { useChatWorkspace } from "../features/chat/composables/useChatWorkspace";

const {
  t,
  isDark,
  leftOpen,
  rightOpen,
  search,
  selectedEntityKind,
  selectedTeamId,
  fileRef,
  displayAgents,
  categoryTree,
  displayTeams,
  inputText,
  dialogMode,
  modelProvider,
  activePlannerKind,
  reactToolLinkIndex,
  sending,
  inputDisabled,
  modeOpts,
  provOpts,
  attachments,
  settingsOpen,
  settingsMode,
  editName,
  editKey,
  editProvider,
  editModel,
  settingsSaving,
  deleteOpen,
  deleteKind,
  deleteNameInput,
  deleteBlockBusy,
  deleting,
  traceOpen,
  traceSessionId,
  traceSessionTitle,
  traceInitialTab,
  traceStreamDeps,
  settingsTitle,
  displaySessions,
  selectedSessionForUi,
  displayMessages,
  expectedDeleteName,
  deleteNameError,
  canConfirmDelete,
  deleteTitleText,
  store,
  onEndAgent,
  onEndTeam,
  selectAgent,
  selectTeam,
  onSelectSession,
  onRenameSession,
  openSessionTrace,
  openSessionEvents,
  onRestoreSession,
  onArchiveSession,
  onSessionDetail,
  onNewSession,
  onSend,
  submitA2UIUserAction,
  onModeChange,
  onProviderChange,
  stopStreaming,
  openSettings,
  onSaveSettings,
  openDelete,
  onConfirmDelete,
  pickFile,
  onFileChange,
  removeAttachment,
  pendingMessages,
  runMeta,
  isRunnerActive,
  onEnqueueWhileRunning,
  runStatus,
  isAwaitingUser,
  awaitKind,
  awaitToolKey,
  wsReplaying,
  sessionRevision,
  wsConnected,
  jobsRefreshNonce,
  submitAwaitingReply,
  submitToolConfirm,
  sessionArtifacts,
  sessionArtifactsLoading,
  openSessionArtifact,
  onCancelPending,
  onUpdatePending,
  onVoiceClick
} = useChatWorkspace();
</script>
