<template>
  <q-card flat bordered class="col column no-wrap chat-mid-card" style="min-height: 0">
    <!-- Breadcrumb navigation for team/member modes -->
    <div v-if="panelMode === 'team' || panelMode === 'member'" class="row items-center q-px-md q-py-xs spirit-breadcrumb">
      <q-btn flat dense no-caps icon="auto_awesome" :label="t('chat.spiritLabel')" color="accent" class="spirit-breadcrumb__item" @click="emit('return-to-spirit')" />
      <template v-if="spiritTeam">
        <q-icon name="chevron_right" size="16px" class="spirit-breadcrumb__sep" />
        <q-btn
          v-if="panelMode === 'member'"
          flat dense no-caps
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

    <template v-if="panelMode === 'team' && spiritTeam">
      <TaskExecutionPanel :team="spiritTeam" :messages="props.messages" :max-parallel="spiritMaxConcurrentTeams" :completion-stats="spiritCompletionStats" :render-markdown="renderChatMarkdown" @return-to-spirit="emit('return-to-spirit')" @cancel-team="(teamId) => emit('cancel-team', teamId)" @resume-team="(teamId) => emit('resume-team', teamId)" @retry-team="(teamId) => emit('retry-team', teamId)" @select-member="(memberId) => emit('select-member', memberId)" @archive-team="(teamId) => emit('archive-team', teamId)" />
    </template>
    <template v-else-if="panelMode === 'member' && spiritTeam && activeMember">
      <MemberReadOnlyPanel :member="activeMember" :team="spiritTeam" :messages="props.messages" :render-markdown="renderChatMarkdown" @return-to-team="emit('return-to-team')" @return-to-spirit="emit('return-to-spirit')" />
    </template>
    <template v-else>
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
          <ChatHeaderUsagePanel
            class="chat-message-header__usage"
            :context-ratio="contextRatio"
            :context-status="contextStatus"
            :usage-snapshot="usageSnapshot"
            :breakdown="contextBreakdown"
            :is-dark="isDark"
            :session-id="sessionId"
            @compact="onCompactSession"
          />
          <ChatHeaderPromptBar
            class="chat-message-header__prompt"
            :full-text="headerUserPrompt"
            :prompt-key="promptKey"
            :session-title="sessionTitle"
            :has-messages="props.messages.length > 0"
          />
          <div class="chat-message-header__actions row items-center justify-end no-wrap">
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
      <div v-if="!panelMode || panelMode === 'spirit'" class="row items-center justify-end q-px-md q-py-xs">
        <UiConfigToggle class="q-mr-sm" :show-tool-calls="showToolCalls ?? true" @toggle="emit('toggle-tool-calls')" />
        <q-btn
          flat
          dense
          no-caps
          :icon="expandAllActive ? 'unfold_less' : 'unfold_more'"
          :label="expandAllActive ? t('chat.collapseAll', '折叠全部') : t('chat.expandAll', '展开全部')"
          class="text-caption"
          :style="{ color: 'var(--color-text-tertiary)' }"
          @click="expandAllActive ? handleCollapseAll() : handleExpandAll()"
        />
      </div>
      <div class="col row no-wrap chat-messages-area" style="min-height: 0">
        <div class="col column no-wrap chat-messages-main" style="min-height: 0">
          <TodoKanbanBoard v-if="(showToolCalls ?? true) && (!panelMode || panelMode === 'spirit')" :board-state="todoBoardState" />
          <ChatMessageList
            ref="messageListRef"
            :session-key="sessionKey"
            :messages="props.messages"
            :pending-messages="props.pendingMessages ?? []"
            :is-dark="props.isDark"
            :is-team-session="props.isTeamSession"
            :planner-kind="props.plannerKind"
            :react-tool-link-index="props.reactToolLinkIndex"
            :reasoning-sidebar-open="props.reasoningSidebarOpen"
            :use-virtual="useVirtualMessageList"
            :use-turn-block-mode="useTurnBlockMode"
            :timeline-items="timelineItems"
            :turn-blocks="turnBlocks"
            :virtual-row-size="virtualRowSize"
            :show-scroll-btn="showScrollBtn"
            :turn-is-focused="turnIsFocused"
            :is-block-collapsed="isBlockCollapsed"
            :activity-timeline-activities="props.activityTimelineActivities"
            :activity-agent-key="props.activityAgentKey"
            :activity-task-content="props.activityTaskContent"
            :activity-tree="props.activityTree"
            :activity-raw-records="props.activityRawRecords"
            @messages-click="handleMessagesClick"
            @scroll="onMessagesScrollWrapped"
            @scroll-to-bottom="scrollToBottom"
            @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
            @feedback="(p) => emit('feedback', p)"
            @regenerate="(msg) => emit('regenerate', msg)"
            @retry="(id) => emit('retry', id)"
            @dismiss-failed="(id) => emit('dismiss-failed', id)"
            @attachment-deleted="(id) => emit('attachment-deleted', id)"
            @download-artifact="(meta) => emit('download-artifact', meta)"
            @pin-reasoning-message="(id) => emit('pin-reasoning-message', id)"
            @cancel-pending="(id) => emit('cancel-pending', id)"
            @update-pending="(id, content) => emit('update-pending', id, content)"
            @toggle-block-collapse="toggleBlock"
          />

          <SynthesisResultCard
            v-if="synthesisResult && (!panelMode || panelMode === 'spirit')"
            :result="synthesisResult"
            :rendered-content="renderChatMarkdown(synthesisResult.content)"
            :evolution-suggestion="spiritEvolutionSuggestion"
            class="q-mx-md q-mb-sm"
          />

          <ContextIndicator
            v-if="compressStatus && compressStatus !== 'normal' && (!panelMode || panelMode === 'spirit')"
            :status="compressStatus"
            class="q-mx-md q-mb-sm"
          />

          <ChatComposer
            v-if="!panelMode || panelMode === 'spirit'"
            :model-value="modelValue"
            :attachments="attachments"
            :dialog-mode="dialogMode"
            :model-provider="modelProvider"
            :mode-options="modeOptions"
            :provider-options="providerOptions"
            :context-ratio="contextRatio"
            :context-status="contextStatus"
            :usage-snapshot="usageSnapshot"
            :session-total-tokens="sessionTotalTokens"
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
            :session-artifacts="sessionArtifacts"
            :session-artifacts-loading="sessionArtifactsLoading"
            :file-supported="fileSupported"
            :file-accept="fileAccept"
            :show-background-jobs="showBackgroundJobs"
            :agent-id="agentId"
            :jobs-refresh-nonce="jobsRefreshNonce"
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
            @open-artifact="emit('open-artifact', $event)"
            @attachment-deleted="emit('attachment-deleted', $event)"
            @download-artifact="emit('download-artifact', $event)"
            @paste-file="emit('paste-file', $event)"
            @focus-turn="emit('focus-turn', $event)"
            @navigate="emit('navigate', $event)"
            @cancel-job="emit('cancel-job', $event)"
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
        :quota-used="spiritStatusBar.quotaUsed"
        :quota-max="spiritStatusBar.quotaMax"
        :token-usage="spiritStatusBar.tokenUsage"
        :last-event="spiritStatusBar.lastEvent"
        :complexity-level="spiritStatusBar.complexityLevel"
        :complexity-reason="spiritStatusBar.complexityReason"
        :checkpoint-step="spiritStatusBar.checkpointStep"
        :dq-score="spiritStatusBar.dqScore"
        @click-running="emit('status-bar-click-running')"
        @click-interrupted="emit('status-bar-click-interrupted')"
        @click-last-event="emit('status-bar-click-last-event')"
      />
    </template>
  </q-card>
</template>

<script setup lang="ts">
// Container: approved — orchestrates virtual scroll, TurnBlock grouping, scroll anchoring, and composable wiring
import { computed, nextTick, onMounted, provide, readonly, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { QVirtualScroll } from 'quasar';
import type { Envelope } from '../../realtime/envelope';
import ChatRunnerStatus from './ChatRunnerStatus.vue';
import ChatTeamMemberStrip from './ChatTeamMemberStrip.vue';
import ChatMessageList from './ChatMessageList.vue';
import ChatComposer from './ChatComposer.vue';
import ChatHeaderUsagePanel from './ChatHeaderUsagePanel.vue';
import ChatHeaderPromptBar from './ChatHeaderPromptBar.vue';
import ChatReasoningDrawer from './ChatReasoningDrawer.vue';
import UiConfigToggle from './UiConfigToggle.vue';
import TodoKanbanBoard from './TodoKanbanBoard.vue';
import ContextIndicator from '../sessions/ContextIndicator.vue';
import TaskExecutionPanel from '../spirit/TaskExecutionPanel.vue';
import MemberReadOnlyPanel from '../spirit/MemberReadOnlyPanel.vue';
import SynthesisResultCard from '../spirit/SynthesisResultCard.vue';
import SpiritStatusBar from '../spirit/SpiritStatusBar.vue';
import type { RunStatusValue } from '../../features/chat/types';
import { TOOL_DISPLAY_KEY } from '../../features/chat/types';
import type { CompressStatus } from '../../features/session/types';
import type { EvolutionSuggestion, SpiritStatusBarData } from '../../features/spirit/types';

import { useTodoBoard } from '../../features/chat/composables/useTodoBoard';
import { useChatTimeline, type TimelineItem } from '../../features/chat/composables/useChatTimeline';
import { useActivityFirstEnabled } from '../../features/chat/useActivityFirstFlag';
import { CHAT_VIRTUAL_ROW_ESTIMATE, CHAT_VIRTUAL_SCROLL_THRESHOLD } from '../../features/chat/chatListVirtual';
import { useChatMessageScroll, useChatCodeCopy } from '../../features/chat/composables/useChatMessageScroll';
import { useChatScrollTitle } from '../../features/chat/useChatScrollTitle';
import type { A2UIUserActionPayload } from '../../features/chat/a2uiUserAction';
import type { Message, ReactToolLinkIndex } from '../../features/chat/types';
import type { ComposerUsageSnapshot } from '../../features/chat/composerUsageMetrics';
import type { PromptBreakdown } from '../../features/chat/contextBreakdown';
import type { ArtifactMeta } from '../../features/artifact/types';
import type { ChatAttachment } from './types';
import type { SpiritTeam, SpiritMember, SynthesisOutput, CompletionStats } from '../../features/spirit/types';
import { renderChatMarkdown } from '../../features/chat/chatMessageMarkdown';
import type { ContextualMessage } from '../../features/chat/composables/useContextualLoadingMessage';
import { useAutoCollapse } from '../../features/chat/composables/useAutoCollapse';
import { EXECUTION_COLLAPSE_CONTROL_KEY } from '../../features/chat/executionCardHelpers';

type Option = { label: string; value: string; caption?: string };

const props = defineProps<{
  panelMode?: 'spirit' | 'team' | 'member';
  spiritTeam?: SpiritTeam | null;
  activeMember?: SpiritMember | null;
  synthesisResult?: SynthesisOutput | null;
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
  usageSnapshot?: ComposerUsageSnapshot | null;
  contextBreakdown?: PromptBreakdown | null;
  sessionTotalTokens?: number | null;
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
  reactToolLinkIndex: ReactToolLinkIndex;
  pendingMessages?: { id: string; content: string; status: string; created_at: string }[];
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
  /**
   * Ordered execution_progress envelopes for the active stream. Surfaced by
   * useChatStreamManager and consumed by useAgentBlocks to render inline
   * orchestration / team / tool / thinking step cards in the timeline.
   *
   * See docs/reports/2026-06-10-proposal-execution-progress-inline.md
   */
  executionProgress?: readonly Envelope[];
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
  /** Max concurrent teams from store (for TaskExecutionPanel). */
  spiritMaxConcurrentTeams?: number;
  /** Evolution suggestion from DQ analysis (for SynthesisResultCard). */
  spiritEvolutionSuggestion?: EvolutionSuggestion | null;
  /** Team completion breakdown from spirit_teams_all_completed event. */
  spiritCompletionStats?: CompletionStats | null;
  compressStatus?: CompressStatus;
  showToolCalls?: boolean;
  /** AF-FE-06: Activity-First timeline activities */
  activityTimelineActivities?: readonly import('../../features/chat/activityTimelineTypes').Activity[];
  /** AF-FE-06: Agent key from Activity data */
  activityAgentKey?: string;
  /** AF-FE-06: Root task content from Activity data */
  activityTaskContent?: string;
  /** AF-FE-06: Activity tree for building TeamPanel */
  activityTree?: readonly import('../../features/chat/activityTypes').ActivityTreeNode[];
  /** AF-FE-14: Raw Activity records (with turnId) for grouping by turn */
  activityRawRecords?: readonly import('../../features/chat/activityTypes').Activity[];
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
  'cancel-job': [job: { id: string; source: string }];
  'paste-unsupported': [];
  'new-session': [];
  'toggle-reasoning-sidebar': [];
  'pin-reasoning-message': [messageId: string];
  'close-reasoning-sidebar': [];
  'return-to-spirit': [];
  'return-to-team': [];
  'cancel-team': [teamId: string];
  'resume-team': [teamId: string];
  'retry-team': [teamId: string];
  'select-member': [memberId: string];
  'archive-team': [teamId: string];
  compact: [sessionId: string];
  'status-bar-click-running': [];
  'status-bar-click-interrupted': [];
  'status-bar-click-last-event': [];
  'toggle-tool-calls': [];
}>();

const { t } = useI18n();
const messagesRef = computed(() => props.messages);

const { messageRow, teamMemberLanes, useTurnBlockMode, turnBlocks, timelineItems } = useChatTimeline({
  messages: messagesRef,
  isTeamSession: props.isTeamSession,
});

const executionProgressRef = computed(() => props.executionProgress ?? []);

// AF-Phase3: useAgentBlocks is fully removed. The AF path renders via
// ConversationTurn (zero inference). The legacy TurnBlock path renders
// via useChatTimeline.turnBlocks (groupMessagesByTurn, no 13-layer inference).

const {
  collapsedBlockKeys,
  expandAllActive,
  isCollapsed: isBlockCollapsed,
  toggleBlock,
  expandAll,
  collapseAll,
  reset: resetAutoCollapse,
} = useAutoCollapse(turnBlocks);

// ── SP-FE-30: Provide/Inject global collapse control ──
const expandAllSignal = ref(0);
const collapseAllSignal = ref(0);
provide(EXECUTION_COLLAPSE_CONTROL_KEY, {
  expandAllSignal: readonly(expandAllSignal),
  collapseAllSignal: readonly(collapseAllSignal),
});

// ── TK: Provide tool display config for child components ──
provide(TOOL_DISPLAY_KEY, computed(() => ({
  showToolCalls: props.showToolCalls ?? true,
})));

// ── TK: Todo board composable ──
const { todoBoardState } = useTodoBoard(messagesRef);

function handleExpandAll() {
  expandAll();
  expandAllSignal.value++;
}

function handleCollapseAll() {
  collapseAll();
  collapseAllSignal.value++;
}


const useVirtualMessageList = computed(() => {
  // AF-FE-05: When Activity-First data is available, ConversationTurn renders
  // which already provides compact timeline views — no need for virtual scroll.
  // Virtual scroll is only useful for the TurnBlock/ChatMessageRow path where
  // individual messages can be numerous.
  // Use activityRawRecords (unfiltered) instead of activityTimelineActivities
  // (filtered) to detect AF data availability — timelineActivities may be empty
  // when all activities are kind=task (root nodes only), but AF path should
  // still be activated via conversationTurns built from rawRecords.
  if (useActivityFirstEnabled() && props.activityRawRecords?.length) return false;
  return timelineItems.value.length >= CHAT_VIRTUAL_SCROLL_THRESHOLD;
});
const virtualRowSize = CHAT_VIRTUAL_ROW_ESTIMATE;
const messageListRef = ref<InstanceType<typeof ChatMessageList> | null>(null);

const virtualScrollRef = computed(() => messageListRef.value?.virtualScrollRef ?? null);
const messagesScrollEl = computed(() => messageListRef.value?.getScrollTarget() ?? null);

const sessionKey = computed(() => props.sessionId?.trim() || props.sessionTitle);
const sessionTitleRef = computed(() => props.sessionTitle);

const { showScrollBtn, highlightedTurnId, onMessagesScroll, scrollToBottom, scrollToTurnId } = useChatMessageScroll({
  sessionKey,
  messages: messagesRef,
  useTurnBlockMode,
  turnBlocks,
  useVirtualMessageList,
  timelineItemsLength: computed(() => timelineItems.value.length),
  virtualScrollRef,
  messagesScrollEl,
});

const { headerUserPrompt, promptKey, refreshActivePrompt, resetToLatestOrSession } = useChatScrollTitle({
  sessionTitle: sessionTitleRef,
  messages: messagesRef,
  messagesScrollEl,
  virtualScrollRef,
  useVirtualMessageList,
});

function onMessagesScrollWrapped(event?: Event) {
  onMessagesScroll(event);
  refreshActivePrompt();
}

function onCompactSession(sid: string) {
  emit('compact', sid);
}

const { handleMessagesClick } = useChatCodeCopy();

function turnIsFocused(turnId: string, userMsgId?: string) {
  const h = highlightedTurnId.value;
  if (!h) return false;
  return h === turnId || (!!userMsgId && h === userMsgId);
}

watch(
  () => props.focusTurnId,
  async (turnId) => {
    if (!turnId?.trim()) return;
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
