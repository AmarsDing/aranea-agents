<template>
  <q-card flat bordered class="col column no-wrap chat-mid-card" style="min-height: 0">
    <!-- Breadcrumb navigation for team/member modes -->
    <div
      v-if="panelMode === 'team' || panelMode === 'member'"
      class="row items-center q-px-md q-py-xs spirit-breadcrumb"
    >
      <q-btn
        flat
        dense
        no-caps
        icon="auto_awesome"
        :label="t('chat.spiritLabel')"
        color="accent"
        class="spirit-breadcrumb__item"
        @click="emit('return-to-spirit')"
      />
      <template v-if="spiritTeam">
        <q-icon name="chevron_right" size="16px" class="spirit-breadcrumb__sep" />
        <q-btn
          v-if="panelMode === 'member'"
          flat
          dense
          no-caps
          :label="spiritTeam.teamName"
          color="accent"
          class="spirit-breadcrumb__item"
          @click="emit('return-to-team')"
        />
        <span v-else class="text-body2 ellipsis spirit-breadcrumb__current">{{ spiritTeam.teamName }}</span>
      </template>
      <template v-if="panelMode === 'member' && activeMember">
        <q-icon name="chevron_right" size="16px" class="spirit-breadcrumb__sep" />
        <span class="text-body2 ellipsis spirit-breadcrumb__current">{{ activeMember.displayName }}</span>
      </template>
    </div>

    <q-banner v-if="wsReplaying" dense rounded class="q-mx-md q-mt-sm app-info-banner">
      <template #avatar>
        <q-spinner-dots color="accent" size="20px" />
      </template>
      {{ t('chat.wsReplaying', '正在同步历史事件…') }}
    </q-banner>
    <q-banner v-else-if="sessionLoading" dense rounded class="q-mx-md q-mt-sm app-info-banner">
      <template #avatar>
        <q-spinner-dots color="accent" size="20px" />
      </template>
      {{ t('chat.sessionLoading', '正在加载会话…') }}
    </q-banner>
    <q-card-section class="chat-message-header q-px-md q-py-sm">
      <div class="chat-message-header__grid">
        <div class="chat-message-header__actions chat-message-header__actions--left row items-center no-wrap">
          <template v-if="wsConnected === false">
            <q-icon name="wifi_off" size="18px" color="warning" class="q-mr-xs">
              <q-tooltip>{{ t('chat.connectionDisconnected') }}</q-tooltip>
            </q-icon>
          </template>
          <template v-else-if="wsConnected === true">
            <span class="ws-connected-dot q-mr-xs">
              <q-tooltip v-if="sessionRevision">{{ t('chat.syncComplete') }} · rev {{ sessionRevision }}</q-tooltip>
              <q-tooltip v-else>{{ t('chat.connected') }}</q-tooltip>
            </span>
          </template>
          <ChatRunnerStatus
            v-if="
              runStatus &&
              runStatus !== 'idle' &&
              runStatus !== 'completed' &&
              runStatus !== 'cancelled' &&
              runStatus !== 'failed'
            "
            class="chat-message-header__runner q-mr-xs"
            :status="runStatus"
            :agent-name="runAgentName"
            :started-at="runStartedAt"
            :event-count="runEventCount"
            @cancel="emit('stop')"
          />
        </div>
        <ChatHeaderPromptBar
          class="chat-message-header__prompt"
          :full-text="headerUserPrompt"
          :prompt-key="promptKey"
          :session-title="sessionTitle"
          :has-messages="props.messages.length > 0"
        />
        <div
          class="chat-message-header__actions chat-message-header__actions--right row items-center justify-end no-wrap"
        >
          <q-btn flat round dense icon="bolt" :aria-label="t('chat.sessionEvents')" @click="emit('open-events')">
            <q-tooltip>{{ t('chat.sessionEvents') }}</q-tooltip>
          </q-btn>
          <q-btn
            v-if="reasoningSidebarActive"
            flat
            round
            dense
            :icon="reasoningSidebarOpen ? 'psychology' : 'psychology_alt'"
            :color="reasoningSidebarOpen ? 'accent' : undefined"
            :aria-label="t('chat.thinkingPanel')"
            @click="emit('toggle-reasoning-sidebar')"
          >
            <q-tooltip>{{ t('chat.thinkingPanel') }}</q-tooltip>
          </q-btn>
        </div>
      </div>
    </q-card-section>
    <ChatTeamMemberStrip v-if="isTeamSession" :members="teamMemberLanes" />
    <div
      v-if="spiritLoadingMessage && (!panelMode || panelMode === 'spirit')"
      class="contextual-loading-bar q-mx-md q-mt-sm"
      :style="{ borderLeftColor: spiritLoadingMessage.color }"
    >
      <div class="row items-center no-wrap q-gutter-xs">
        <q-icon :name="spiritLoadingMessage.icon" :color="spiritLoadingMessage.color" size="16px" />
        <span class="text-caption">{{ spiritLoadingMessage.text }}</span>
      </div>
    </div>
    <div class="col row no-wrap chat-messages-area" style="min-height: 0">
      <div class="col column no-wrap chat-messages-main" style="min-height: 0">
        <template v-if="isChatView">
          <TodoKanbanBoard
            v-if="(showToolCalls ?? true) && (!panelMode || panelMode === 'spirit')"
            :board-state="todoBoardState"
          />
          <ChatMessageList
            ref="messageListRef"
          :session-key="sessionKey"
          :messages="props.messages"
          :pending-messages="props.pendingMessages ?? []"
          :is-dark="props.isDark"
          :is-team-session="props.isTeamSession"
          :planner-kind="props.plannerKind"
          :reasoning-sidebar-open="props.reasoningSidebarOpen"
          :show-scroll-btn="showScrollBtn"
          :session-id="props.sessionId"
          :agent-map="props.agentMap"
          :run-status="props.runStatus"
          @messages-click="handleMessagesClick"
          @scroll="onMessagesScrollWrapped"
          @scroll-to-bottom="scrollToBottom"
          @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
          @feedback="(p) => emit('feedback', p)"
          @regenerate="(msg) => emit('regenerate', msg)"
          @regenerate-v2="(task) => emit('regenerate-v2', task)"
          @retry="(id) => emit('retry', id)"
          @dismiss-failed="(id) => emit('dismiss-failed', id)"
          @attachment-deleted="(id) => emit('attachment-deleted', id)"
          @download-artifact="(meta) => emit('download-artifact', meta)"
          @pin-reasoning-message="(id) => emit('pin-reasoning-message', id)"
          @cancel-pending="(id) => emit('cancel-pending', id)"
          @interrupt-pending="(id) => emit('interrupt-pending', id)"
          @update-pending="(id, content) => emit('update-pending', id, content)"
          @confirm="(id, approved) => emit('confirm-activity', id, approved)"
          @error-retry="(e) => emit('error-retry', e)"
          @error-switch-model="(e) => emit('error-switch-model', e)"
          @error-rephrase="(e) => emit('error-rephrase', e)"
          @error-check-config="(e) => emit('error-check-config', e)"
          @error-remove-attachment="(e) => emit('error-remove-attachment', e)"
          @error-relogin="(e) => emit('error-relogin', e)"
          @expand-member="(p) => emit('expand-member', p)"
          @enter-session="(sid) => emit('enter-session', sid)"
          @cancel-team="(teamId) => emit('cancel-team', teamId)"
          @retry-team="(teamId) => emit('retry-team', teamId)"
          @pause-team="(teamId) => emit('pause-team', teamId)"
          @unpause-team="(teamId) => emit('unpause-team', teamId)"
          @inject-team="(p: { teamId: string; message: string }) => emit('inject-team', p)"
          @cancel-agent="(sessionId) => emit('cancel-agent', sessionId)"
          @retry-agent="(sessionId) => emit('retry-agent', sessionId)"
          @pause-agent="(sessionId) => emit('pause-agent', sessionId)"
          @resume-agent="(sessionId) => emit('resume-agent', sessionId)"
          @inject-agent="(p: { sessionId: string; message: string }) => emit('inject-agent', p)"
          @expand="(ids: string[]) => emit('expand', ids)"
        />

        <ContextIndicator
          v-if="compressStatus && compressStatus !== 'normal' && (!panelMode || panelMode === 'spirit')"
          :status="compressStatus"
          class="q-mx-md q-mb-sm"
        />
        </template>

        <!-- Observe mode: ComfyUI-style observation canvas replaces message list -->
        <ObservationPanel
          v-else
          class="col"
          :session-id="sessionId ?? ''"
          :spirit-session-id="sessionId ?? ''"
          :is-dark="isDark"
          :ws-connected="wsConnected"
        />

        <ChatComposer
          v-if="(!panelMode || panelMode === 'spirit') && (composerVisible ?? true)"
          :model-value="modelValue"
          :attachments="attachments"
          :dialog-mode="dialogMode"
          :model-provider="modelProvider"
          :mode-options="modeOptions"
          :provider-options="providerOptions"
          :context-ratio="contextRatio"
          :context-status="contextStatus"
          :knowledge-base-options="knowledgeBaseOptions"
          :selected-knowledge-bases="selectedKnowledgeBases"
          :is-dark="isDark"
          :sending="sending"
          :input-disabled="inputDisabled"
          :is-runner-active="isRunnerActive"
          :is-awaiting-user="isAwaitingUser"
          :await-kind="awaitKind"
          :await-tool-key="awaitToolKey"
          :show-enqueue="showEnqueue"
          :session-id="sessionId"
          :file-supported="fileSupported"
          :file-accept="fileAccept"
          @update:model-value="emit('update:modelValue', $event)"
          @update:dialog-mode="emit('update:dialogMode', $event)"
          @update:model-provider="emit('update:modelProvider', $event)"
          @update:selected-knowledge-bases="emit('update:selectedKnowledgeBases', $event)"
          @remove-attachment="emit('remove-attachment', $event)"
          @pick-file="emit('pick-file')"
          @voice="emit('voice')"
          @send="emit('send')"
          @stop="emit('stop')"
          @enqueue-message="emit('enqueue-message', $event)"
          @submit-await-reply="emit('submit-await-reply')"
          @submit-tool-confirm="emit('submit-tool-confirm', $event)"
          @paste-file="emit('paste-file', $event)"
          @paste-unsupported="emit('paste-unsupported')"
          @new-session="emit('new-session')"
        />
      </div>
      <ChatReasoningDrawer
        :open="Boolean(reasoningSidebarOpen)"
        :active-reasoning="reasoningSidebarActive ?? null"
        :is-dark="isDark"
        @close="emit('close-reasoning-sidebar')"
      />
    </div>
    <SpiritStatusBar
      v-if="spiritStatusBar && (!panelMode || panelMode === 'spirit')"
      :running-team-count="spiritStatusBar.runningTeamCount"
      :interrupted-team-count="spiritStatusBar.interruptedTeamCount"
      :completed-team-count="spiritStatusBar.completedTeamCount"
      :total-team-count="spiritStatusBar.totalTeamCount"
      :token-usage="spiritStatusBar.tokenUsage"
      :context-ratio="spiritStatusBar.contextRatio"
      :context-used-tokens="spiritStatusBar.contextUsedTokens"
      :context-window="spiritStatusBar.contextWindow"
      :session-id="sessionId"
      :complexity-level="spiritStatusBar.complexityLevel"
      :complexity-reason="spiritStatusBar.complexityReason"
      :checkpoint-step="spiritStatusBar.checkpointStep"
      :dq-score="spiritStatusBar.dqScore"
      :view-mode="viewMode ?? 'chat'"
      :composer-visible="composerVisible ?? true"
      @toggle-view="emit('toggle-view')"
      @toggle-composer="emit('toggle-composer')"
      @click-running="emit('status-bar-click-running')"
      @click-interrupted="emit('status-bar-click-interrupted')"
    />
  </q-card>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, provide, readonly, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import ChatRunnerStatus from './ChatRunnerStatus.vue';
import ChatTeamMemberStrip from './ChatTeamMemberStrip.vue';
import type { TeamMemberLane } from './ChatTeamMemberStrip.vue';
import ChatMessageList from './ChatMessageList.vue';
import ChatComposer from './ChatComposer.vue';
import ChatHeaderPromptBar from './ChatHeaderPromptBar.vue';
import ChatReasoningDrawer from './ChatReasoningDrawer.vue';
import TodoKanbanBoard from './TodoKanbanBoard.vue';
import ContextIndicator from '../sessions/ContextIndicator.vue';
import SpiritStatusBar from '../spirit/SpiritStatusBar.vue';
import ObservationPanel from './observe/ObservationPanel.vue';
import type { RunStatusValue } from '../../features/chat/types';
import { TOOL_DISPLAY_KEY } from '../../features/chat/types';
import type { CompressStatus } from '../../features/session/types';
import type { SpiritStatusBarData } from '../../features/spirit/types';

import { useTodoBoard } from '../../features/chat/composables/useTodoBoard';
import { useChatMessageScroll, useChatCodeCopy } from '../../features/chat/composables/useChatMessageScroll';
import { useChatScrollTitle } from '../../features/chat/useChatScrollTitle';
import type { A2UIUserActionPayload } from '../../features/chat/a2uiUserAction';
import type { Message } from '../../features/chat/types';
import type { PromptBreakdown } from '../../features/chat/contextBreakdown';
import type { ArtifactMeta } from '../../features/artifact/types';
import type { ChatAttachment } from './types';
import type { SpiritTeam, SpiritMember } from '../../features/spirit/types';
import type { ContextualMessage } from '../../features/chat/composables/useContextualLoadingMessage';
import { EXECUTION_COLLAPSE_CONTROL_KEY } from '../../features/chat/executionCardHelpers';
import type { Step } from '../../features/chat/v2Types';

type Option = { label: string; value: string; caption?: string };

const props = defineProps<{
  panelMode?: 'spirit' | 'team' | 'member';
  spiritTeam?: SpiritTeam | null;
  activeMember?: SpiritMember | null;
  modelValue: string;
  messages: Message[];
  attachments: ChatAttachment[];
  dialogMode: string;
  modelProvider: string;
  modeOptions: Option[];
  providerOptions: Option[];
  sessionTitle: string;
  sessionId?: string;
  contextRatio: number;
  contextStatus?: string;
  contextBreakdown?: PromptBreakdown | null;
  knowledgeBaseOptions?: Option[];
  selectedKnowledgeBases?: string[];
  isDark: boolean;
  sending?: boolean;
  inputDisabled?: boolean;
  isRunnerActive?: boolean;
  isAwaitingUser?: boolean;
  awaitKind?: string;
  awaitToolKey?: string;
  wsReplaying?: boolean;
  sessionLoading?: boolean;
  isTeamSession?: boolean;
  plannerKind?: string;
  pendingMessages?: { id: string; content: string; status: string; created_at: string }[];
  /** P1#1/2: agent key → display name lookup for TeamCard/AgentCard. */
  agentMap?: Map<string, { displayName: string; agentKey: string }>;
  runStatus?: RunStatusValue;
  runAgentName?: string;
  runStartedAt?: string;
  runEventCount?: number;
  showEnqueue?: boolean;
  sessionRevision?: number | null;
  wsConnected?: boolean;
  focusTurnId?: string;
  sessionArtifacts?: ArtifactMeta[];
  sessionArtifactsLoading?: boolean;
  fileSupported?: boolean;
  fileAccept?: string;
  showBackgroundJobs?: boolean;
  agentId?: string;
  jobsRefreshNonce?: number;
  reasoningSidebarOpen?: boolean;
  reasoningSidebarActive?: { messageId: string; reasoning: string; streaming: boolean } | null;
  spiritLoadingMessage?: ContextualMessage | null;
  /** Spirit status bar data. */
  spiritStatusBar?: SpiritStatusBarData | null;
  compressStatus?: CompressStatus;
  showToolCalls?: boolean;
  /** View mode: 'chat' (default) shows message list; 'observe' shows observation canvas. */
  viewMode?: 'chat' | 'observe';
  /** Whether the composer is visible (used in observe mode to toggle input bar). */
  composerVisible?: boolean;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: string];
  'update:dialogMode': [value: string];
  'update:modelProvider': [value: string];
  'update:selectedKnowledgeBases': [value: string[]];
  'remove-attachment': [id: string];
  'pick-file': [];
  voice: [];
  send: [];
  stop: [];
  'enqueue-message': [content: string];
  'cancel-pending': [pendingId: string];
  'interrupt-pending': [pendingId: string];
  'update-pending': [pendingId: string, content: string];
  'submit-await-reply': [];
  'submit-tool-confirm': [approved: boolean];
  'open-events': [];
  'open-artifact': [id: string];
  'paste-file': [file: File];
  'focus-turn': [turnId: string];
  navigate: [route: { name: string; params: Record<string, string> }];
  'focus-turn-cleared': [];
  'a2ui-user-action': [payload: A2UIUserActionPayload];
  feedback: [payload: { messageId: string; rating: 'positive' | 'negative' }];
  retry: [messageId: string];
  'dismiss-failed': [messageId: string];
  'attachment-deleted': [id: string];
  'download-artifact': [meta: import('../../features/artifact/types').ArtifactMeta];
  regenerate: [message: Message];
  'regenerate-v2': [task: import('../../features/chat/v2Types').Task];
  'cancel-job': [job: { id: string; source: string }];
  'paste-unsupported': [];
  'new-session': [];
  'toggle-reasoning-sidebar': [];
  'pin-reasoning-message': [messageId: string];
  'close-reasoning-sidebar': [];
  'return-to-spirit': [];
  'return-to-team': [];
  'cancel-team': [teamId: string];
  'retry-team': [teamId: string];
  'toggle-view': [];
  'toggle-composer': [];
  'pause-team': [teamId: string];
  'unpause-team': [teamId: string];
  'inject-team': [payload: { teamId: string; message: string }];
  'cancel-agent': [sessionId: string];
  'retry-agent': [sessionId: string];
  'pause-agent': [sessionId: string];
  'resume-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  'select-member': [memberId: string];
  'archive-team': [teamId: string];
  'select-spirit-team': [teamId: string];
  compact: [sessionId: string];
  'status-bar-click-running': [];
  'status-bar-click-interrupted': [];
  'toggle-tool-calls': [];
  'confirm-activity': [activityId: string, approved: boolean];
  'error-retry': [step: Step];
  'error-switch-model': [step: Step];
  'error-rephrase': [step: Step];
  'error-check-config': [step: Step];
  'error-remove-attachment': [step: Step];
  'error-relogin': [step: Step];
  'expand-member': [payload: { agentKey: string; agentName?: string; teamId?: string }];
  'enter-session': [sessionId: string];
  // T5.2/T5.3 / §B.7.2: Forward team-card / agent-card expand events upstream
  // so ChatPage can lazy-load member/child session activities.
  expand: [sessionIds: string[]];
}>();

const { t } = useI18n();
// T5.5: Mobile (<1024px) responsive logic removed — app targets desktop only.
// useQuasar/$q were only used for $q.screen.lt.md (isMobile).
// UnifiedExecutionPanel removed: team progress / DAG / task breakdown now render
// inline within ActivityStream via TeamCard / GraphStageBlock / PlanBlock,
// ordered by activity Timestamp (historical order). This eliminates the dual-data-source
// (spiritStore vs activityTree) refresh-desync issue and aligns with the m59
// design prototype where unified-panel content is generated in conversation order.

const messagesRef = computed(() => props.messages);

/** True when the view is in chat mode (default); false in observe mode. */
const isChatView = computed(() => (props.viewMode ?? 'chat') === 'chat');

// ── Team member lanes ──
const teamMemberLanes = computed((): TeamMemberLane[] => {
  if (!props.isTeamSession) return [];
  const lanes = new Map<string, TeamMemberLane>();
  for (const message of props.messages) {
    if (!message.team_member) continue;
    const key = message.team_member.agent_id || message.id;
    const label = message.agent_ref?.name || message.team_member.name || key;
    const streaming = message.status === 'streaming' || message.status === 'tool_running';
    const prev = lanes.get(key);
    lanes.set(key, {
      key,
      label,
      streaming: (prev?.streaming ?? false) || streaming,
    });
  }
  return [...lanes.values()];
});

// ── SP-FE-30: Provide/Inject global collapse control ──
// T8.5: The "Expand All" / "Collapse All" toolbar buttons above the chat were
// removed per user request (2026-07-04). The provide signals remain at 0 so
// child components injecting EXECUTION_COLLAPSE_CONTROL_KEY still get a valid
// object; if a future UI control wants to trigger expand/collapse, it just
// needs to increment the respective signal.
const expandAllSignal = ref(0);
const collapseAllSignal = ref(0);
provide(EXECUTION_COLLAPSE_CONTROL_KEY, {
  expandAllSignal: readonly(expandAllSignal),
  collapseAllSignal: readonly(collapseAllSignal),
});

// ── TK: Provide tool display config for child components ──
provide(
  TOOL_DISPLAY_KEY,
  computed(() => ({
    showToolCalls: props.showToolCalls ?? true,
  })),
);

// ── TK: Todo board composable ──
const { todoBoardState } = useTodoBoard(messagesRef);

const messageListRef = ref<InstanceType<typeof ChatMessageList> | null>(null);

const messagesScrollEl = computed(() => messageListRef.value?.getScrollTarget() ?? null);

const sessionKey = computed(() => props.sessionId?.trim() || props.sessionTitle);
const sessionTitleRef = computed(() => props.sessionTitle);

const { showScrollBtn, onMessagesScroll, scrollToBottom, scrollToTurnId } = useChatMessageScroll({
  sessionKey,
  messages: messagesRef,
  messagesScrollEl,
});

const { headerUserPrompt, promptKey, refreshActivePrompt, resetToLatestOrSession } = useChatScrollTitle({
  sessionTitle: sessionTitleRef,
  messages: messagesRef,
  messagesScrollEl,
  // T8.3: Enable virtual scroll path for long conversations.
  virtualScrollRef: computed(() => messageListRef.value?.virtualScrollRef ?? null),
  useVirtualMessageList: computed(() => messageListRef.value?.useVirtualScroll ?? false),
});

function onMessagesScrollWrapped(event?: Event) {
  onMessagesScroll(event);
  refreshActivePrompt();
}

const { handleMessagesClick } = useChatCodeCopy();

watch(
  () => props.focusTurnId,
  async (turnId) => {
    if (!turnId?.trim()) return;
    // T8.3: In virtual-scroll mode, DynamicScroller only renders visible items,
    // so useChatMessageScroll.scrollToTurnId (which uses querySelector) would fail
    // for off-screen turns. Use ChatMessageList.scrollToTurnId first to bring the
    // item into view via DynamicScroller.scrollToItem, then let useChatMessageScroll
    // handle the highlight (its scrollIntoView becomes a no-op or minor adjustment).
    if (messageListRef.value?.useVirtualScroll) {
      await messageListRef.value.scrollToTurnId(turnId);
    }
    await scrollToTurnId(turnId);
    emit('focus-turn-cleared');
  },
);

watch(sessionKey, () => {
  resetToLatestOrSession();
  void nextTick(() => refreshActivePrompt());
});

watch(
  () => props.messages.length,
  () => {
    void nextTick(() => refreshActivePrompt());
  },
);

onMounted(() => {
  void nextTick(() => refreshActivePrompt());
});
</script>

<style scoped lang="sass">
.ws-connected-dot
  display: inline-block
  width: 8px
  height: 8px
  border-radius: 50%
  background: var(--color-success)
  opacity: 60%

.contextual-loading-bar
  padding: 6px 12px
  border-radius: 8px
  background: color-mix(in srgb, var(--glass-surface) 50%, transparent)
  border-left: 3px solid var(--color-accent)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

.spirit-breadcrumb
  border-bottom: 1px solid var(--glass-border)
  background: var(--glass-surface)
  min-height: 32px

  &__item
    font-size: 12px
    padding: 2px 6px

  &__current
    color: var(--color-text-secondary)
    max-width: 200px

  &__sep
    color: var(--color-text-tertiary)
    margin: 0 4px
</style>
