<template>
  <div v-if="!coreReady" class="flex flex-center" style="height: 100vh">
    <q-spinner-dots size="40px" color="primary" />
  </div>
  <ChatWorkspaceShell v-else>
    <ChatEntitySidebar
      v-model:search="layout.search"
      :open="layout.leftOpen"
      :spirit-teams="spiritStore.teams"
      :expanded-team-ids="spiritStore.expandedTeamIds"
      :selected-kind="entity.selectedEntityKind"
      :selected-team-id="spiritStore.activeTeamId"
      :is-dark="layout.isDark"
      @select-spirit="spiritStore.returnToSpirit()"
      @select-spirit-team="spiritStore.selectTeam($event)"
      @toggle-team-expand="spiritStore.toggleTeamExpand($event)"
    />

    <ChatSideToggle
      :open="layout.leftOpen"
      :icon="layout.leftOpen ? 'chevron_left' : 'chevron_right'"
      :ariaLabel="layout.t('chat.collapseList')"
      @toggle="layout.leftOpen = !layout.leftOpen"
    />

    <div class="chat-workspace-main col column no-wrap">
      <q-banner
        v-if="session.inboundHydrateError"
        dense
        rounded
        class="app-banner-warning q-mx-sm q-mt-sm"
      >
        {{ session.inboundHydrateError }}
      </q-banner>
      <ChatMessagePanel
        v-model="composer.inputText"
        v-model:dialog-mode="composer.dialogMode"
        v-model:model-provider="composer.modelProvider"
        :panel-mode="spiritStore.activePanelMode"
        :spirit-team="spiritStore.activeTeam"
        :messages="session.displayMessages"
        :attachments="composer.attachments"
        :mode-options="composer.modeOpts"
        :provider-options="composer.provOpts"
        :session-title="session.selectedSessionForUi?.title || layout.t('chat.untitledSession')"
        :session-id="session.selectedSessionForUi?.id"
        :context-ratio="session.selectedSessionForUi?.context_used_ratio ?? 0"
        :context-status="session.selectedSessionForUi?.context_status"
        :usage-snapshot="session.composerUsageSnapshot"
        :context-breakdown="session.contextBreakdown"
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
        :session-loading="session.sessionLoading"
        :session-revision="session.sessionRevision"
        :ws-connected="session.wsConnected"
        :is-team-session="entity.selectedEntityKind === 'team'"
        :planner-kind="entity.activePlannerKind"
        :react-tool-link-index="session.reactToolLinkIndex"
        :focus-turn-id="session.focusTurnId"
        :session-artifacts="session.sessionArtifacts"
        :session-artifacts-loading="session.sessionArtifactsLoading"
        :file-supported="session.fileSupported"
        :file-accept="session.fileAccept"
        :show-background-jobs="entity.selectedEntityKind === 'agent'"
        :agent-id="entity.store.selectedAgent?.id"
        :jobs-refresh-nonce="session.jobsRefreshNonce"
        :reasoning-sidebar-open="session.reasoningSidebarOpen"
        :reasoning-sidebar-active="session.reasoningSidebarActive"
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
        @paste-file="composer.uploadFile"
        @voice="composer.onVoiceClick"
        @send="composer.onSend"
        @stop="composer.stopStreaming"
        @cancel-pending="composer.onCancelPending"
        @update-pending="composer.onUpdatePending"
        @submit-await-reply="composer.submitAwaitingReply"
        @submit-tool-confirm="composer.submitToolConfirm"
        @open-events="session.openSessionEvents"
        @open-artifact="session.openSessionArtifact"
        @attachment-deleted="session.onArtifactDeleted"
        @download-artifact="session.downloadArtifact"
        @focus-turn="session.focusSessionTurn"
        @navigate="onNavigate"
        @cancel-job="composer.cancelBackgroundJob"
        @paste-unsupported="onPasteUnsupported"
        @new-session="session.addAgentSession"
        @focus-turn-cleared="session.clearFocusTurn"
        @toggle-reasoning-sidebar="session.toggleReasoningSidebar"
        @pin-reasoning-message="session.pinReasoningMessage"
        @close-reasoning-sidebar="session.toggleReasoningSidebar"
        @a2ui-user-action="composer.submitA2UIUserAction"
        @feedback="composer.onMessageFeedback"
        @retry="composer.retryFailedMessage"
        @dismiss-failed="composer.dismissFailedMessage"
        @regenerate="composer.regenerateMessage"
        @compact="onCompactSession"
      />
      <input ref="fileRef" type="file" hidden multiple :accept="session.fileAccept" @change="composer.onFileChange" />
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
import SessionTimelineDialog from "../components/chat/SessionTimelineDialog.vue";
import { useRouter } from "vue-router";
import { useChatWorkspace } from "../features/chat/composables/useChatWorkspace";
import { useSpiritTeamStore } from "../stores/spirit";
import { useQuasar } from "quasar";
import { useI18n } from "vue-i18n";

const { coreReady, fileRef, layout, entity, session, composer, dialogs } = useChatWorkspace();
const spiritStore = useSpiritTeamStore();
const router = useRouter();
const $q = useQuasar();
const { t } = useI18n();

function onNavigate(route: { name: string; params: Record<string, string> }) {
  router.push(route);
}

function onPasteUnsupported() {
  $q.notify({ type: "warning", message: t("chat.clipboardFileUnsupported", "当前模型不支持此类型的文件粘贴") });
}

async function onCompactSession(sessionId: string) {
  try {
    const result = await session.compactSessionAction(sessionId);
    if (result.compacted) {
      const before = Math.round((result.estimated_tokens_before / 1000) * 10) / 10;
      const after = Math.round((result.estimated_tokens_after / 1000) * 10) / 10;
      $q.notify({
        type: "positive",
        message: t("chat.contextManuallyCompressed", `上下文已压缩 (${before}k → ${after}k tokens)`),
        timeout: 4000,
      });
    } else {
      $q.notify({
        type: "info",
        message: t("chat.contextNoCompactionNeeded", "当前上下文无需压缩"),
        timeout: 3000,
      });
    }
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e);
    $q.notify({ type: "negative", message: t("chat.contextCompactFailed", "压缩失败") + `: ${msg}`, timeout: 5000 });
  }
}
</script>
