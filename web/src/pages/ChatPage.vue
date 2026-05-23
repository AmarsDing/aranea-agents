<template>
  <ChatWorkspaceShell>
    <ChatEntitySidebar
      v-model:search="layout.search"
      v-model:agents="entity.displayAgents"
      v-model:teams="entity.displayTeams"
      :open="layout.leftOpen"
      :selected-kind="entity.selectedEntityKind"
      :selected-agent-id="entity.store.selectedAgent?.id"
      :selected-team-id="entity.selectedTeamId"
      :category-tree="entity.categoryTree"
      :is-dark="layout.isDark"
      @agent-reorder-end="entity.onEndAgent"
      @team-reorder-end="entity.onEndTeam"
      @select-agent="entity.selectAgent"
      @select-team="entity.selectTeam"
      @settings="entity.openSettings"
      @delete="entity.openDelete"
    />

    <ChatSideToggle
      :open="layout.leftOpen"
      :icon="layout.leftOpen ? 'chevron_left' : 'chevron_right'"
      :ariaLabel="layout.t('chat.collapseList')"
      @toggle="layout.leftOpen = !layout.leftOpen"
    />

    <div class="chat-workspace-main col column no-wrap">
      <ChatMessagePanel
        v-model="composer.inputText"
        v-model:dialog-mode="composer.dialogMode"
        v-model:model-provider="composer.modelProvider"
        :messages="session.displayMessages"
        :attachments="composer.attachments"
        :mode-options="composer.modeOpts"
        :provider-options="composer.provOpts"
        :session-title="session.selectedSessionForUi?.title || layout.t('chat.untitledSession')"
        :context-ratio="session.selectedSessionForUi?.context_used_ratio ?? 0"
        :session-total-tokens="session.selectedSessionForUi?.total_tokens ?? null"
        :knowledge-base-options="composer.knowledgeBaseOptions"
        :selected-knowledge-bases="composer.selectedKnowledgeBases"
        :is-dark="layout.isDark"
        :sending="composer.sending"
        :input-disabled="composer.inputDisabled"
        :is-awaiting-user="composer.isAwaitingUser"
        :await-kind="composer.awaitKind"
        :await-tool-key="composer.awaitToolKey"
        :ws-replaying="session.wsReplaying"
        :session-revision="session.sessionRevision"
        :ws-connected="session.wsConnected"
        :is-team-session="entity.selectedEntityKind === 'team'"
        :planner-kind="entity.activePlannerKind"
        :react-tool-link-index="session.reactToolLinkIndex"
        :focus-turn-id="session.focusTurnId"
        :pending-messages="composer.pendingMessages"
        :run-status="composer.runStatus"
        :run-agent-name="composer.runMeta?.agentName"
        :run-started-at="composer.runMeta?.startedAt"
        :run-event-count="composer.runMeta?.eventCount"
        :show-enqueue="composer.isRunnerActive"
        @enqueue-message="composer.onEnqueueWhileRunning"
        @update:dialog-mode="composer.onModeChange"
        @update:model-provider="composer.onProviderChange"
        @update:selected-knowledge-bases="(v) => (composer.selectedKnowledgeBases = v)"
        @remove-attachment="composer.removeAttachment"
        @pick-file="composer.pickFile"
        @voice="composer.onVoiceClick"
        @send="composer.onSend"
        @stop="composer.stopStreaming"
        @cancel-pending="composer.onCancelPending"
        @update-pending="composer.onUpdatePending"
        @submit-await-reply="composer.submitAwaitingReply"
        @submit-tool-confirm="composer.submitToolConfirm"
        @open-events="session.openSessionEvents"
        @focus-turn-cleared="session.clearFocusTurn"
        @a2ui-user-action="composer.submitA2UIUserAction"
        @feedback="composer.onMessageFeedback"
      />
      <chat-session-artifacts-panel
        :session-id="session.selectedSessionForUi?.id ?? ''"
        :items="session.sessionArtifacts"
        :loading="session.sessionArtifactsLoading"
        @open="session.openSessionArtifact"
      />
      <ChatBackgroundJobsPanel
        v-if="entity.selectedEntityKind === 'agent'"
        :session-id="session.selectedSessionForUi?.id"
        :agent-id="entity.store.selectedAgent?.id"
        :refresh-nonce="session.jobsRefreshNonce"
        @focus-turn="session.focusSessionTurn"
      />
      <input ref="fileRef" type="file" hidden multiple @change="composer.onFileChange" />
    </div>

    <ChatSideToggle
      :open="layout.rightOpen"
      :icon="layout.rightOpen ? 'chevron_right' : 'chevron_left'"
      :ariaLabel="layout.t('chat.collapseSession')"
      @toggle="layout.rightOpen = !layout.rightOpen"
    />

    <ChatSessionSidebar
      :open="layout.rightOpen"
      :sessions="session.displaySessions"
      :selected-session-id="session.selectedSessionForUi?.id"
      :is-dark="layout.isDark"
      @select="session.onSelectSession"
      @new-session="session.onNewSession"
      @rename="session.onRenameSession"
      @trace="session.openSessionTrace"
      @delete="entity.openDelete"
      @restore="session.onRestoreSession"
      @archive="session.onArchiveSession"
      @detail="session.onSessionDetail"
    />

    <template #dialogs>
      <ChatSettingsDialog
        v-model="dialogs.settingsOpen"
        v-model:name="dialogs.editName"
        v-model:provider="dialogs.editProvider"
        v-model:model="dialogs.editModel"
        :title="dialogs.settingsTitle"
        :mode="dialogs.settingsMode"
        :agent-key="dialogs.editKey"
        :saving="dialogs.settingsSaving"
        @save="dialogs.onSaveSettings"
      />

      <ChatDeleteDialog
        v-model="dialogs.deleteOpen"
        v-model:name-input="dialogs.deleteNameInput"
        :title="dialogs.deleteTitleText"
        :kind="dialogs.deleteKind"
        :expected-name="dialogs.expectedDeleteName"
        :blocked-busy="dialogs.deleteBlockBusy"
        :can-confirm="dialogs.canConfirmDelete"
        :has-name-error="Boolean(dialogs.deleteNameError && dialogs.deleteNameInput)"
        :deleting="dialogs.deleting"
        @confirm="dialogs.onConfirmDelete"
      />

      <SessionTimelineDialog
        v-model="dialogs.traceOpen"
        :session-id="dialogs.traceSessionId"
        :session-title="dialogs.traceSessionTitle"
        :initial-tab="dialogs.traceInitialTab"
        :stream-deps="dialogs.traceStreamDeps"
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

const { fileRef, layout, entity, session, composer, dialogs } = useChatWorkspace();
</script>
