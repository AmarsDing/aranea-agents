<template>
  <q-page class="mobile-chat-page column no-wrap">
    <div v-if="!workspace.coreReady" class="col flex flex-center">
      <q-spinner-dots size="40px" color="accent" />
    </div>

    <template v-else>
      <div class="mobile-chat-page__topbar row items-center no-wrap q-pl-xs q-pr-md">
        <q-btn flat round dense icon="arrow_back" :aria-label="t('mobile.backToSessions')" @click="goBack" />
        <div class="col text-subtitle2 ellipsis">
          {{ workspace.session.selectedSessionForUi?.title || t('chat.untitledSession') }}
        </div>
      </div>

      <q-banner v-if="workspace.session.inboundHydrateError" dense rounded class="app-banner-warning q-mx-sm q-mt-sm">
        {{ workspace.session.inboundHydrateError }}
      </q-banner>
      <LlmRetryBanner
        v-if="llmRetry"
        :attempt="llmRetry.attempt"
        :max-retries="llmRetry.maxRetries"
        :delay-ms="llmRetry.delayMs"
        :error="llmRetry.error"
      />

      <ChatMessagePanel
        v-model="workspace.composer.inputText"
        class="col"
        :dialog-mode="workspace.composer.dialogMode"
        :model-provider="workspace.composer.modelProvider"
        :panel-mode="spiritStore.activePanelMode"
        :spirit-team="spiritStore.activeTeam"
        :active-member="activeMember"
        :messages="workspace.session.displayMessages"
        :attachments="workspace.composer.attachments"
        :mode-options="workspace.composer.modeOpts"
        :provider-options="workspace.composer.provOpts"
        :session-title="workspace.session.selectedSessionForUi?.title || t('chat.untitledSession')"
        :session-id="workspace.session.selectedSessionForUi?.id"
        :context-ratio="workspace.session.selectedSessionForUi?.context_used_ratio ?? 0"
        :context-status="workspace.session.selectedSessionForUi?.context_status"
        :context-breakdown="workspace.session.contextBreakdown"
        :is-dark="workspace.layout.isDark"
        :sending="workspace.composer.sending"
        :input-disabled="workspace.composer.inputDisabled"
        :is-runner-active="workspace.composer.isRunnerActive"
        :ws-replaying="workspace.session.wsReplaying"
        :spirit-loading-message="workspace.session.spiritLoadingMessage"
        :spirit-status-bar="spiritStatusBar"
        :compress-status="workspace.session.compressStatus"
        :show-tool-calls="uiConfig.showToolCalls"
        :session-loading="workspace.session.sessionLoading"
        :session-revision="workspace.session.sessionRevision"
        :ws-connected="workspace.session.wsConnected"
        :is-team-session="workspace.entity.selectedEntityKind === 'team'"
        :planner-kind="workspace.entity.activePlannerKind"
        :focus-turn-id="workspace.session.focusTurnId"
        :session-artifacts="workspace.session.sessionArtifacts"
        :session-artifacts-loading="workspace.session.sessionArtifactsLoading"
        :file-supported="workspace.session.fileSupported"
        :file-accept="workspace.session.fileAccept"
        :show-background-jobs="workspace.entity.selectedEntityKind === 'agent'"
        :agent-id="workspace.entity.store.selectedAgent?.id"
        :jobs-refresh-nonce="workspace.session.jobsRefreshNonce"
        :view-mode="spiritStore.viewMode"
        :composer-visible="spiritStore.composerVisible"
        :reasoning-sidebar-open="workspace.session.reasoningSidebarOpen"
        :reasoning-sidebar-active="workspace.session.reasoningSidebarActive"
        :pending-messages="workspace.composer.pendingMessages"
        :agent-map="agentMap"
        :run-status="workspace.composer.runStatus"
        :run-agent-name="workspace.composer.runMeta?.agentName"
        :run-started-at="workspace.composer.runMeta?.startedAt"
        :run-event-count="workspace.composer.runMeta?.eventCount"
        :show-enqueue="workspace.composer.isRunnerActive"
        @enqueue-message="workspace.composer.onEnqueueWhileRunning"
        @update:dialog-mode="workspace.composer.onModeChange"
        @update:model-provider="workspace.composer.onProviderChange"
        @remove-attachment="workspace.composer.removeAttachment"
        @pick-file="workspace.composer.pickFile"
        @paste-file="workspace.composer.uploadFile"
        @voice="workspace.composer.onVoiceClick"
        @send="workspace.composer.onSend"
        @stop="workspace.composer.stopStreaming"
        @cancel-pending="workspace.composer.onCancelPending"
        @interrupt-pending="workspace.composer.onInterruptPending"
        @update-pending="workspace.composer.onUpdatePending"
        @open-events="workspace.session.openSessionEvents"
        @open-artifacts-page="onDesktopOnly"
        @open-artifact="workspace.session.openSessionArtifact"
        @attachment-deleted="workspace.session.onArtifactDeleted"
        @download-artifact="workspace.session.downloadArtifact"
        @focus-turn="workspace.session.focusSessionTurn"
        @navigate="handlers.onNavigate"
        @cancel-job="workspace.composer.cancelBackgroundJob"
        @paste-unsupported="workspace.composer.onPasteUnsupported"
        @new-session="workspace.session.onNewSession"
        @focus-turn-cleared="workspace.session.clearFocusTurn"
        @toggle-reasoning-sidebar="workspace.session.toggleReasoningSidebar"
        @pin-reasoning-message="workspace.session.pinReasoningMessage"
        @close-reasoning-sidebar="workspace.session.toggleReasoningSidebar"
        @a2ui-user-action="workspace.composer.submitA2UIUserAction"
        @feedback="workspace.composer.onMessageFeedback"
        @retry="workspace.composer.retryFailedMessage"
        @dismiss-failed="workspace.composer.dismissFailedMessage"
        @regenerate="workspace.composer.regenerateMessage"
        @regenerate-v2="workspace.composer.regenerateV2Task"
        @resume-task="workspace.session.resumeTask"
        @compact="workspace.session.onCompactSession"
        @toggle-tool-calls="uiConfig.setShowToolCalls(!uiConfig.showToolCalls)"
        @confirm-activity="workspace.session.onConfirmActivity"
        @confirm-activity-grant="workspace.session.onConfirmActivityGrant"
        @submit-clarification="workspace.session.onSubmitClarification"
        @error-retry="workspace.errorBlock.onErrorRetry"
        @error-switch-model="workspace.errorBlock.onErrorSwitchModel"
        @error-rephrase="workspace.errorBlock.onErrorRephrase"
        @error-check-config="workspace.errorBlock.onErrorCheckConfig"
        @error-remove-attachment="workspace.errorBlock.onErrorRemoveAttachment"
        @error-relogin="handlers.onErrorRelogin"
        @cancel-team="spiritStore.cancelTeam"
        @pause-team="spiritStore.pauseTeam"
        @unpause-team="spiritStore.unpauseTeam"
        @inject-team="(p: { teamId: string; message: string }) => spiritStore.injectTeam(p.teamId, p.message)"
        @archive-team="spiritStore.archiveTeam"
        @select-member="spiritStore.selectMember"
        @select-spirit-team="spiritStore.selectTeam($event)"
        @return-to-team="spiritStore.activeTeamId ? spiritStore.selectTeam(spiritStore.activeTeamId) : undefined"
        @return-to-spirit="handlers.onSelectSpirit"
        @status-bar-click-running="handlers.onStatusBarClickRunning"
        @status-bar-click-interrupted="handlers.onStatusBarClickInterrupted"
        @toggle-view="spiritStore.toggleViewMode()"
        @toggle-composer="spiritStore.toggleComposer()"
        @expand-member="handlers.onExpandMember"
        @enter-session="handlers.onEnterSession"
        @cancel-agent="handlers.onCancelAgent"
        @retry-agent="handlers.onRetryAgent"
        @pause-agent="handlers.onPauseAgent"
        @resume-agent="handlers.onResumeAgent"
        @inject-agent="handlers.onInjectAgent"
        @expand="handlers.onExpandChildren"
      />
      <input
        ref="fileInputRef"
        type="file"
        hidden
        multiple
        :accept="workspace.session.fileAccept"
        @change="workspace.composer.onFileChange"
      />

      <SessionTimelineDialog
        v-model="workspace.dialogs.traceOpen"
        :session-id="workspace.dialogs.traceSessionId"
        :session-title="workspace.dialogs.traceSessionTitle"
        :initial-tab="workspace.dialogs.traceInitialTab"
        :stream-deps="workspace.dialogs.traceStreamDeps"
        :timeline="workspace.dialogs.timeline"
        :timeline-loading="workspace.dialogs.timelineLoading"
        :timeline-error="workspace.dialogs.timelineError"
        @refresh-trace="workspace.dialogs.reloadTimeline()"
      />
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import ChatMessagePanel from '../../components/chat/ChatMessagePanel.vue';
import LlmRetryBanner from '../../components/chat/LlmRetryBanner.vue';
import SessionTimelineDialog from '../../components/chat/SessionTimelineDialog.vue';
import { injectChatWorkspace } from '../../features/chat/composables/chatWorkspaceInjection';
import { useChatMessagePanelBindings } from '../../features/chat/composables/useChatMessagePanelBindings';
import { useSpiritTeamStore } from '../../stores/spirit';
import { useLlmRetryStore } from '../../stores/chat/llmRetryStore';

const { t } = useI18n();
const $q = useQuasar();
const router = useRouter();
const workspace = injectChatWorkspace();
const bindings = useChatMessagePanelBindings(workspace);
// Top-level refs auto-unwrap in template (nested `bindings.x.value` would not).
const { activeMember, agentMap, spiritStatusBar, uiConfig, handlers } = bindings;
const spiritStore = useSpiritTeamStore();
const llmRetryStore = useLlmRetryStore();

/** Active LLM retry state for the current session — drives the reconnect banner. */
const llmRetry = computed(() => {
  const sid = workspace.session.selectedSessionForUi?.id;
  return sid ? llmRetryStore.retryFor(sid) : null;
});

// The workspace's fileRef is bound by ChatPage on desktop; on mobile the
// hidden input lives here. Both write the same ref object — only one page is
// mounted at a time per breakpoint guard, so there is no conflict.
const fileInputRef = workspace.fileRef;

function goBack() {
  void router.push({ name: 'mobile-sessions' });
}

/** Artifacts management page is desktop-only; guard bounces desktop routes. */
function onDesktopOnly() {
  $q.notify({ type: 'info', message: t('mobile.desktopOnlyFeature') });
}
</script>

<style scoped>
.mobile-chat-page__topbar {
  min-height: 44px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.08);
}

.mobile-chat-page :deep(.chat-mid-card) {
  border: none;
  border-radius: 0;
  box-shadow: none;
}
</style>
