<template>
  <q-card flat bordered class="col column no-wrap chat-mid-card" style="min-height: 0">
    <template v-if="panelMode === 'team' && spiritTeam">
      <TaskExecutionPanel
        :team="spiritTeam"
        :messages="props.messages"
        @return-to-spirit="emit('return-to-spirit')"
      />
    </template>
    <template v-else-if="panelMode === 'member'">
      <div class="col column flex-center text-grey-6" style="min-height: 200px">
        <q-icon name="person" size="48px" class="q-mb-md" />
        <div class="text-body2">成员详情面板（P1 实现）</div>
      </div>
    </template>
    <template v-else>
    <q-banner v-if="wsReplaying" dense rounded class="q-mx-md q-mt-sm app-info-banner">
      <template #avatar>
        <q-spinner-dots color="primary" size="20px" />
      </template>
      {{ t("chat.wsReplaying", "正在同步历史事件…") }}
    </q-banner>
    <q-banner v-else-if="sessionLoading" dense rounded class="q-mx-md q-mt-sm app-info-banner">
      <template #avatar>
        <q-spinner-dots color="primary" size="20px" />
      </template>
      {{ t("chat.sessionLoading", "正在加载会话…") }}
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
              <q-tooltip>连接已断开</q-tooltip>
            </q-icon>
          </template>
          <template v-else-if="wsConnected === true">
            <span class="ws-connected-dot q-mr-xs">
              <q-tooltip v-if="sessionRevision">同步完成 · rev {{ sessionRevision }}</q-tooltip>
              <q-tooltip v-else>已连接</q-tooltip>
            </span>
          </template>
          <ChatRunnerStatus
            v-if="runStatus && runStatus !== 'idle' && runStatus !== 'completed' && runStatus !== 'cancelled' && runStatus !== 'failed'"
            class="chat-message-header__runner q-mr-xs"
            :status="runStatus"
            :agent-name="runAgentName"
            :started-at="runStartedAt"
            :event-count="runEventCount"
            @cancel="emit('stop')"
          />
          <q-btn flat round dense icon="bolt" aria-label="Session events" @click="emit('open-events')">
            <q-tooltip>会话事件</q-tooltip>
          </q-btn>
          <q-btn
            v-if="reasoningSidebarActive"
            flat
            round
            dense
            :icon="reasoningSidebarOpen ? 'psychology' : 'psychology_alt'"
            :color="reasoningSidebarOpen ? 'primary' : undefined"
            aria-label="思考面板"
            @click="emit('toggle-reasoning-sidebar')"
          >
            <q-tooltip>思考面板</q-tooltip>
          </q-btn>
        </div>
      </div>
    </q-card-section>
    <ChatTeamMemberStrip v-if="isTeamSession" :members="teamMemberLanes" />
    <div class="col row no-wrap chat-messages-area" style="min-height: 0">
    <div class="col column no-wrap chat-messages-main" style="min-height: 0">
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
    />

    <SynthesisResultCard
      v-if="synthesisResult && (!panelMode || panelMode === 'spirit')"
      :result="synthesisResult"
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
    </template>
  </q-card>
</template>

<script setup lang="ts">
// Container: approved — orchestrates virtual scroll, TurnBlock grouping, scroll anchoring, and composable wiring
import { computed, nextTick, onMounted, ref, toRef, watch } from "vue";
import { useI18n } from "vue-i18n";
import type { QVirtualScroll } from "quasar";
import ChatRunnerStatus from "./ChatRunnerStatus.vue";
import ChatTeamMemberStrip from "./ChatTeamMemberStrip.vue";
import ChatMessageList from "./ChatMessageList.vue";
import ChatComposer from "./ChatComposer.vue";
import ChatHeaderUsagePanel from "./ChatHeaderUsagePanel.vue";
import ChatHeaderPromptBar from "./ChatHeaderPromptBar.vue";
import ChatReasoningDrawer from "./ChatReasoningDrawer.vue";
import TaskExecutionPanel from "../spirit/TaskExecutionPanel.vue";
import SynthesisResultCard from "../spirit/SynthesisResultCard.vue";
import type { RunStatusValue } from "../../features/chat/types";
import { useChatTimeline, type TimelineItem } from "../../features/chat/composables/useChatTimeline";
import {
  CHAT_VIRTUAL_ROW_ESTIMATE,
  CHAT_VIRTUAL_SCROLL_THRESHOLD,
} from "../../features/chat/chatListVirtual";
import { useChatMessageScroll, useChatCodeCopy } from "../../features/chat/composables/useChatMessageScroll";
import { useChatScrollTitle } from "../../features/chat/useChatScrollTitle";
import type { A2UIUserActionPayload } from "../../features/chat/a2uiUserAction";
import type { Message, ReactToolLinkIndex } from "../../features/chat/types";
import type { ComposerUsageSnapshot } from "../../features/chat/composerUsageMetrics";
import type { PromptBreakdown } from "../../features/chat/contextBreakdown";
import type { ArtifactMeta } from "../../features/artifact/types";
import type { ChatAttachment } from "./types";
import type { SpiritTeam, SynthesisOutput } from "../../features/spirit/types";

type Option = { label: string; value: string; caption?: string };

const props = defineProps<{
  panelMode?: "spirit" | "team" | "member";
  spiritTeam?: SpiritTeam | null;
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
  fileSupported?: boolean;
  fileAccept?: string;
  showBackgroundJobs?: boolean;
  agentId?: string;
  jobsRefreshNonce?: number;
  reasoningSidebarOpen?: boolean;
  reasoningSidebarActive?: { messageId: string; reasoning: string; streaming: boolean } | null;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: string];
  "update:dialogMode": [value: string];
  "update:modelProvider": [value: string];
  "update:selectedKnowledgeBases": [value: string[]];
  "remove-attachment": [id: string];
  "pick-file": [];
  voice: [];
  send: [];
  stop: [];
  "enqueue-message": [content: string];
  "cancel-pending": [pendingId: string];
  "update-pending": [pendingId: string, content: string];
  "submit-await-reply": [];
  "submit-tool-confirm": [approved: boolean];
  "open-events": [];
  "open-artifact": [id: string];
  "paste-file": [file: File];
  "focus-turn": [turnId: string];
  navigate: [route: { name: string; params: Record<string, string> }];
  "focus-turn-cleared": [];
  "a2ui-user-action": [payload: A2UIUserActionPayload];
  feedback: [payload: { messageId: string; rating: "positive" | "negative" }];
  retry: [messageId: string];
  "dismiss-failed": [messageId: string];
  "attachment-deleted": [id: string];
  "download-artifact": [meta: import("../../features/artifact/types").ArtifactMeta];
  regenerate: [message: Message];
  "cancel-job": [job: { id: string; source: string }];
  "paste-unsupported": [];
  "new-session": [];
  "toggle-reasoning-sidebar": [];
  "pin-reasoning-message": [messageId: string];
  "close-reasoning-sidebar": [];
  "return-to-spirit": [];
  compact: [sessionId: string];
}>();

const { t } = useI18n();
const messagesRef = computed(() => props.messages);

const {
  messageRow,
  teamMemberLanes,
  useTurnBlockMode,
  turnBlocks,
  timelineItems,
} = useChatTimeline({
  messages: messagesRef,
  isTeamSession: props.isTeamSession,
});

const useVirtualMessageList = computed(() => timelineItems.value.length >= CHAT_VIRTUAL_SCROLL_THRESHOLD);
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
  emit("compact", sid);
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
    emit("focus-turn-cleared");
  }
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

<style scoped>
.ws-connected-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--color-success);
  opacity: 60%;
}
</style>
