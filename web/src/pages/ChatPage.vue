<template>
  <div v-if="!coreReady" class="flex flex-center" style="height: 100vh">
    <q-spinner-dots size="40px" color="accent" />
  </div>
  <ChatWorkspaceShell v-else>
    <ChatEntitySidebar
      v-if="!isMobile"
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
      @update:search="layout.search = $event"
      @select-spirit="onSelectSpirit()"
      @select-agent="entity.selectAgent($event)"
      @agent-settings="(id) => entity.openSettings('agent', id)"
      @agent-delete="(id) => entity.openDelete('agent', id)"
      @agent-reorder="(payload) => entity.onGroupReorder(payload.groupKey, payload.ids)"
      @select-spirit-team="spiritStore.selectTeam($event)"
      @toggle-team-expand="spiritStore.toggleTeamExpand($event)"
      @spirit-settings="(id) => entity.openSettings('agent', id)"
    />

    <ChatSideToggle
      v-if="!isMobile"
      :open="layout.leftOpen"
      :icon="layout.leftOpen ? 'chevron_left' : 'chevron_right'"
      :aria-label="layout.t('chat.collapseList')"
      @toggle="layout.leftOpen = !layout.leftOpen"
    />

    <div class="chat-workspace-main col column no-wrap">
      <q-banner v-if="session.inboundHydrateError" dense rounded class="app-banner-warning q-mx-sm q-mt-sm">
        {{ session.inboundHydrateError }}
      </q-banner>
      <ChatMessagePanel
        v-model="composer.inputText"
        :dialog-mode="composer.dialogMode"
        :model-provider="composer.modelProvider"
        :panel-mode="spiritStore.activePanelMode"
        :spirit-team="spiritStore.activeTeam"
        :active-member="activeMember"
        :synthesis-result="spiritStore.synthesisResult"
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
        :is-runner-active="composer.isRunnerActive"
        :is-awaiting-user="composer.isAwaitingUser"
        :await-kind="composer.awaitKind"
        :await-tool-key="composer.awaitToolKey"
        :ws-replaying="session.wsReplaying"
        :execution-progress="session.executionProgress"
        :spirit-loading-message="session.spiritLoadingMessage"
        :spirit-status-bar="spiritStatusBar"
        :spirit-max-concurrent-teams="spiritStore.maxConcurrentTeams ?? undefined"
        :spirit-evolution-suggestion="spiritStore.lastEvolutionSuggestion"
        :spirit-completion-stats="spiritStore.completionStats"
        :compress-status="session.compressStatus"
        :show-tool-calls="uiConfig.showToolCalls"
        :activity-timeline-activities="session.activityTimeline.streamEvents"
        :activity-agent-key="session.activityTimeline.activities[0]?.agentKey"
        :activity-task-content="session.activityTimeline.activities.find((a: {kind: string}) => a.kind === 'task')?.content"
        :activity-tree="session.activityTimeline.activityTree"
        :activity-raw-records="session.activityTimeline.activities"
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
        @interrupt-pending="composer.onInterruptPending"
        @update-pending="composer.onUpdatePending"
        @submit-await-reply="composer.submitAwaitingReply"
        @submit-tool-confirm="composer.submitToolConfirm"
        @open-events="session.openSessionEvents"
        @open-artifact="session.openSessionArtifact"
        @open-sessions="mobileSessionOpen = true"
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
        @compact="session.onCompactSession"
        @toggle-tool-calls="uiConfig.setShowToolCalls(!uiConfig.showToolCalls)"
        @confirm-activity="onConfirmActivity"
        @cancel-team="spiritStore.cancelTeam"
        @resume-team="spiritStore.resumeTeam"
        @retry-team="spiritStore.retryTeam"
        @archive-team="spiritStore.archiveTeam"
        @select-member="spiritStore.selectMember"
        @return-to-team="spiritStore.activeTeamId ? spiritStore.selectTeam(spiritStore.activeTeamId) : undefined"
        @return-to-spirit="onSelectSpirit"
        @status-bar-click-running="onStatusBarClickRunning"
        @status-bar-click-interrupted="onStatusBarClickInterrupted"
        @status-bar-click-last-event="onStatusBarClickLastEvent"
      />
      <input ref="fileRef" type="file" hidden multiple :accept="session.fileAccept" @change="composer.onFileChange" />
    </div>

    <ChatSideToggle
      v-if="!isMobile"
      :open="layout.rightOpen"
      :icon="layout.rightOpen ? 'chevron_right' : 'chevron_left'"
      :aria-label="layout.t('chat.collapseSession')"
      @toggle="layout.rightOpen = !layout.rightOpen"
    />

    <ChatSessionSidebar
      v-if="!isMobile"
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
      <q-dialog
        v-if="isMobile"
        v-model="mobileSessionOpen"
        position="bottom"
        :full-width="true"
        class="mobile-session-dialog"
      >
        <q-card class="mobile-session-sheet column no-wrap">
          <div class="mobile-session-sheet__handle" />
          <div class="mobile-session-sheet__header row items-center justify-between q-px-md q-py-sm">
            <div class="text-subtitle2 text-weight-medium">{{ layout.t('chat.sessionListTitle') }}</div>
            <q-btn v-close-popup flat dense round icon="close" />
          </div>
          <q-separator />
          <div class="mobile-session-sheet__body col">
            <ChatSessionSidebar
              :open="true"
              :sessions="session.displaySessions"
              :inbox-sessions="session.inboxSessions"
              :selected-session-id="session.selectedSessionForUi?.id"
              :is-dark="layout.isDark"
              :favorite-ids="session.favoriteIds"
              @select="onMobileSessionSelect"
              @new-session="onMobileNewSession"
              @rename="session.onRenameSession"
              @toggle-pin="session.onTogglePinSession"
              @toggle-favorite="session.onToggleFavorite"
              @trace="session.openSessionTrace"
              @delete="entity.openDelete"
              @restore="session.onRestoreSession"
              @archive="session.onArchiveSession"
              @detail="session.onSessionDetail"
            />
          </div>
        </q-card>
      </q-dialog>

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
import SessionTimelineDialog from '../components/chat/SessionTimelineDialog.vue';
import { computed, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { useChatWorkspace } from '../features/chat/composables/useChatWorkspace';
import { confirmActivity } from '../features/chat/api';
import { useSpiritTeamStore } from '../stores/spirit';
import { useUiConfigStore } from '../stores/uiConfig';
import { DEFAULT_MAX_PARALLEL_TEAMS } from '../features/spirit/observabilityConstants';
import type { Agent } from '../features/agents/types';

const SPIRIT_AGENT_KEY = '__spirit__';

const { coreReady, fileRef, layout, entity, session, composer, dialogs } = useChatWorkspace();
const spiritStore = useSpiritTeamStore();
const uiConfig = useUiConfigStore();
const router = useRouter();
const $q = useQuasar();
const { t } = useI18n();

const isMobile = computed(() => $q.screen.lt.md);
const mobileSessionOpen = ref(false);

watch(
  isMobile,
  (mobile) => {
    if (mobile) {
      layout.leftOpen = false;
      layout.rightOpen = false;
      mobileSessionOpen.value = false;
    }
  },
  { immediate: true },
);

function onMobileSessionSelect(sessionId: string) {
  session.onSelectSession(sessionId);
  mobileSessionOpen.value = false;
}

function onMobileNewSession() {
  session.onNewSession();
  mobileSessionOpen.value = false;
}

const activeMember = computed(() => {
  const team = spiritStore.activeTeam;
  const memberId = spiritStore.activeMemberId;
  if (!team || !memberId) return null;
  return team.members.find((m) => m.agentId === memberId) ?? null;
});

const spiritStatusBar = computed(() => {
  const teams = spiritStore.teams;
  if (!teams.length) return null;
  const running = teams.filter((t) => t.status === 'running' || t.status === 'pending').length;
  const interrupted = teams.filter((t) => t.status === 'interrupted').length;
  const completedTeams = teams.filter((t) => t.status === 'completed');
  const failedTeams = teams.filter((t) => t.status === 'failed');
  // Aggregate token usage from all teams that have token data
  const totalTokenIn = teams.reduce((sum, t) => sum + (t.tokenIn ?? 0), 0);
  const totalTokenOut = teams.reduce((sum, t) => sum + (t.tokenOut ?? 0), 0);
  return {
    runningTeamCount: running,
    interruptedTeamCount: interrupted,
    quotaUsed: running,
    quotaMax: spiritStore.maxConcurrentTeams ?? DEFAULT_MAX_PARALLEL_TEAMS,
    tokenUsage: totalTokenIn > 0 || totalTokenOut > 0 ? { in: totalTokenIn, out: totalTokenOut } : null,
    lastEvent:
      completedTeams.length > 0 || failedTeams.length > 0
        ? {
            type: (completedTeams.length > 0 ? 'completed' : 'failed') as 'completed' | 'failed',
            teamName: (completedTeams[0] ?? failedTeams[0])?.teamName ?? '',
            teamId: (completedTeams[0] ?? failedTeams[0])?.id ?? '',
          }
        : null,
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

function onNavigate(route: { name: string; params: Record<string, string> }) {
  router.push(route);
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

function onStatusBarClickLastEvent() {
  // Use the lastEvent teamId if available, otherwise find by name
  const lastEvent = spiritStatusBar.value?.lastEvent;
  if (lastEvent?.teamId) {
    spiritStore.selectTeam(lastEvent.teamId);
    return;
  }
  if (lastEvent?.teamName) {
    const team = spiritStore.teams.find((t) => t.teamName === lastEvent.teamName);
    if (team) spiritStore.selectTeam(team.id);
  }
}

/** N-14: Handle confirm-activity event from ConfirmBlock → API call. */
async function onConfirmActivity(activityId: string, approved: boolean) {
  const sid = session.selectedSessionForUi?.id;
  if (!sid) return;
  try {
    const ok = await confirmActivity(sid, activityId, approved);
    if (!ok) {
      $q.notify({
        type: 'warning',
        message: approved ? t('chat.confirmActivity.approveRejected') : t('chat.confirmActivity.denyRejected'),
      });
    }
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('chat.confirmActivity.failed') });
  }
}
</script>
