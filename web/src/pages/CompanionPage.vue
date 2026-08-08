<template>
  <div class="companion-page">
    <div v-if="!coreReady" class="flex flex-center full-height">
      <q-spinner-dots size="40px" color="accent" />
    </div>

    <template v-else>
      <HudCanvas
        ref="hudRef"
        :voice-state="companion.voiceState"
        :voice-mode-on="companion.voiceModeOn"
        :subtitle="companion.subtitlePartial"
        :error="companion.lastError"
        :spectrum="spectrum"
        :amplitude="amplitude"
        @toggle-chat="companion.toggleChat()"
        @toggle-voice="toggleVoiceMode()"
        @dismiss-error="companion.clearVoiceError()"
      />

      <!-- V2-T5：全息确认卡浮层（确认通过时粒子流发射 + HUD 能量爆发） -->
      <div ref="confirmLayerRef" class="companion-page__confirm-layer">
        <HoloConfirmCard
          v-if="activeConfirm"
          :key="activeConfirm.id"
          :card="activeConfirm"
          :queue-size="confirmQueue.length"
          :voice-mode-on="companion.voiceModeOn"
          @decide="onConfirmDecide(activeConfirm!, $event)"
        />
      </div>

      <CompanionChatPanel :open="companion.chatOpen" @close="companion.toggleChat()">
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
          @open-artifacts-page="handlers.onOpenArtifactsPage"
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
      </CompanionChatPanel>

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
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';

import HudCanvas from '../components/companion/HudCanvas.vue';
import CompanionChatPanel from '../components/companion/CompanionChatPanel.vue';
import HoloConfirmCard from '../components/companion/HoloConfirmCard.vue';
import ChatMessagePanel from '../components/chat/ChatMessagePanel.vue';
import LlmRetryBanner from '../components/chat/LlmRetryBanner.vue';
import SessionTimelineDialog from '../components/chat/SessionTimelineDialog.vue';
import { useChatWorkspace } from '../features/chat/composables/useChatWorkspace';
import { useChatMessagePanelBindings } from '../features/chat/composables/useChatMessagePanelBindings';
import { TOOL_CONFIRM_REPLY, type ToolConfirmReply } from '../features/chat/types';
import { useVoiceSession } from '../features/companion/voice/useVoiceSession';
import { useCompanionConfirms } from '../features/companion/useCompanionConfirms';
import { spawnLaunchBurst } from '../features/companion/launchParticles';
import type { ConfirmCardModel, ConfirmDecision } from '../features/companion/types';
import { useCompanionStore } from '../stores/companion';
import { useSpiritTeamStore } from '../stores/spirit';
import { useLlmRetryStore } from '../stores/chat/llmRetryStore';

const { t } = useI18n();

// 与 ChatPage 同级的独立 workspace 实例（不同路由不会同时挂载，见 ChatPage）。
const workspace = useChatWorkspace();
const { coreReady } = workspace;
const bindings = useChatMessagePanelBindings(workspace);
// Top-level refs auto-unwrap in template (nested `bindings.x.value` would not).
const { activeMember, agentMap, spiritStatusBar, uiConfig, handlers } = bindings;

const spiritStore = useSpiritTeamStore();
const llmRetryStore = useLlmRetryStore();
const companion = useCompanionStore();

// 语音会话绑定当前选中的聊天会话（/v1/voice?session_id=...，设计 §2.1）。
const { spectrum, amplitude, toggleVoiceMode } = useVoiceSession({
  sessionId: () => workspace.session.selectedSessionForUi?.id ?? null,
});

// V2-T5：确认卡队列（activityV2Store 派生，WS 状态推进自动出队）。
const { queue: confirmQueue, active: activeConfirm } = useCompanionConfirms(
  () => workspace.session.selectedSessionForUi?.id ?? null,
);
const confirmLayerRef = ref<HTMLDivElement | null>(null);
const hudRef = ref<InstanceType<typeof HudCanvas> | null>(null);

const DECISION_REPLY: Record<ConfirmDecision, ToolConfirmReply> = {
  approve: TOOL_CONFIRM_REPLY.approve,
  deny: TOOL_CONFIRM_REPLY.deny,
  always: TOOL_CONFIRM_REPLY.approveAlways,
};

/** 确认卡决议：批准时粒子流发射 + HUD 能量爆发（乐观视觉），随后走 grant API。 */
function onConfirmDecide(card: ConfirmCardModel, decision: ConfirmDecision) {
  if (decision !== 'deny') {
    if (confirmLayerRef.value) spawnLaunchBurst(confirmLayerRef.value);
    hudRef.value?.triggerBurst();
  }
  void workspace.session.onConfirmActivityGrant({
    sessionId: card.sessionId,
    activityId: card.id,
    reply: DECISION_REPLY[decision],
  });
}

/** Active LLM retry state for the current session — drives the reconnect banner. */
const llmRetry = computed(() => {
  const sid = workspace.session.selectedSessionForUi?.id;
  return sid ? llmRetryStore.retryFor(sid) : null;
});

// The workspace's fileRef is bound by ChatPage on desktop; the companion page
// hosts its own hidden input. Only one route page is mounted at a time.
const fileInputRef = workspace.fileRef;
</script>

<style scoped lang="sass">
.companion-page
  position: relative
  width: 100%
  height: 100vh
  overflow: hidden
  // HUD 画布为固有深空底色（产品形态，非主题变量）；浮层元素仍走主题 token
  background: radial-gradient(ellipse at 50% 40%, #101826 0%, #090d14 70%)

  &__confirm-layer
    position: absolute
    left: 50%
    bottom: 132px
    transform: translateX(-50%)
    z-index: 20
    display: flex
    flex-direction: column
    align-items: center
    pointer-events: none

    // 卡片本身恢复交互（浮层容器仅定位 + 粒子宿主）
    :deep(.holo-confirm)
      pointer-events: auto
</style>

<!-- 粒子发射层样式：spawnLaunchBurst 动态创建的元素无 scoped 属性，需全局类 -->
<style lang="sass">
.launch-burst
  position: absolute
  inset: 0
  overflow: visible
  pointer-events: none

  &__p
    position: absolute
    border-radius: 50%
    background: radial-gradient(circle, #a5f3fc 0%, #22d3ee 55%, transparent 75%)
    box-shadow: 0 0 8px rgba(0, 229, 255, 0.8), 0 0 2px #fff
    will-change: transform, opacity
</style>
