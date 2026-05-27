<template>
  <q-card flat bordered class="col column no-wrap chat-mid-card" style="min-height: 0; border-radius: 18px">
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
          :is-dark="isDark"
        />
        <ChatHeaderPromptBar
          class="chat-message-header__prompt"
          :full-text="headerUserPrompt"
          :prompt-key="promptKey"
          :session-title="sessionTitle"
          :has-messages="props.messages.length > 0"
        />
        <div class="chat-message-header__actions row items-center justify-end no-wrap">
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
        </div>
      </div>
    </q-card-section>
    <ChatTeamMemberStrip v-if="isTeamSession" :members="teamMemberLanes" />
    <q-separator class="cream-sep" />
    <div :key="sessionKey" class="chat-messages col column no-wrap" style="min-height: 0">
      <div
        v-if="!props.messages.length"
        ref="messagesScrollEl"
        class="col relative-position chat-messages__viewport"
        @click="handleMessagesClick"
      >
        <div class="chat-empty-state column items-center justify-center">
          <div class="chat-empty-state__halo">
            <q-icon name="forum" size="38px" color="primary" />
          </div>
          <div class="chat-empty-state__title q-mt-md">{{ t("chat.emptyMessages") }}</div>
          <div class="chat-empty-state__hint text-caption q-mt-xs">{{ t("chat.inputLabel") }}</div>
        </div>
      </div>
      <q-virtual-scroll
        v-else-if="useVirtualMessageList"
        ref="virtualScrollRef"
        class="col chat-messages__viewport"
        style="min-height: 0"
        :items="timelineItems"
        :virtual-scroll-item-size="virtualRowSize"
        :virtual-scroll-slice-size="48"
        :virtual-scroll-slice-ratio-before="2"
        :virtual-scroll-slice-ratio-after="2"
        v-slot="{ item, index }"
        @scroll="onMessagesScrollWrapped"
        @click="handleMessagesClick"
      >
        <TurnBlock
          v-if="useTurnBlockMode && item.kind === 'block'"
          :block="item.block"
          :focused="turnIsFocused(item.block.turnId, item.block.user?.id)"
          :all-messages="props.messages"
          :is-dark="props.isDark"
          :is-team-session="props.isTeamSession"
          :planner-kind="props.plannerKind"
          :react-tool-link-index="props.reactToolLinkIndex"
          @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
          @feedback="(p) => emit('feedback', p)"
        />
        <ChatMessageRow
          v-else
          :message="item.message"
          :index="index"
          v-memo="[item.message.id, item.message.content_markdown, item.message.status, item.message.options_json, props.isDark, props.plannerKind]"
          :messages="props.messages"
          :is-dark="props.isDark"
          :is-team-session="props.isTeamSession"
          :planner-kind="props.plannerKind"
          :react-tool-link-index="props.reactToolLinkIndex"
          @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
          @feedback="(p) => emit('feedback', p)"
          @retry="(id) => emit('retry', id)"
          @dismiss-failed="(id) => emit('dismiss-failed', id)"
        />
      </q-virtual-scroll>
      <div
        v-else
        ref="messagesScrollEl"
        class="col relative-position chat-messages__viewport"
        @scroll.passive="onMessagesScrollWrapped"
        @click="handleMessagesClick"
      >
        <template v-if="useTurnBlockMode">
          <TurnBlock
            v-for="block in turnBlocks"
            :key="block.turnId"
            :block="block"
            :focused="turnIsFocused(block.turnId, block.user?.id)"
            :all-messages="props.messages"
            :is-dark="props.isDark"
            :is-team-session="props.isTeamSession"
            :planner-kind="props.plannerKind"
            :react-tool-link-index="props.reactToolLinkIndex"
            @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
            @feedback="(p) => emit('feedback', p)"
          />
        </template>
        <ChatMessageRow
          v-else
          v-for="(message, idx) in props.messages"
          :key="message.id"
          v-memo="[message.id, message.content_markdown, message.status, message.options_json, props.isDark, props.plannerKind]"
          :message="message"
          :index="idx"
          :messages="props.messages"
          :is-dark="props.isDark"
          :is-team-session="props.isTeamSession"
          :planner-kind="props.plannerKind"
          :react-tool-link-index="props.reactToolLinkIndex"
          @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
          @retry="(id) => emit('retry', id)"
          @dismiss-failed="(id) => emit('dismiss-failed', id)"
        />
      </div>
      <ChatPendingQueue
        :messages="props.pendingMessages ?? []"
        :is-dark="props.isDark"
        @cancel-pending="(id) => emit('cancel-pending', id)"
        @update-pending="(id, content) => emit('update-pending', id, content)"
      />
      <transition name="chat-scroll-fade">
        <q-btn
          v-if="showScrollBtn"
          round
          unelevated
          color="primary"
          icon="arrow_downward"
          class="chat-scroll-bottom"
          aria-label="滚动到最新消息"
          @click="scrollToBottom(true)"
        />
      </transition>
    </div>

    <q-separator class="cream-sep" />
    <ChatComposer
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
      :is-awaiting-user="isAwaitingUser"
      :await-kind="awaitKind"
      :await-tool-key="awaitToolKey"
      :show-enqueue="showEnqueue"
      :session-id="sessionId"
      :session-artifacts="sessionArtifacts"
      :session-artifacts-loading="sessionArtifactsLoading"
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
      @focus-turn="emit('focus-turn', $event)"
      @navigate="emit('navigate', $event)"
    />
  </q-card>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, toRef, watch } from "vue";
import { useI18n } from "vue-i18n";
import type { QVirtualScroll } from "quasar";
import ChatMessageRow from "./ChatMessageRow.vue";
import TurnBlock from "./TurnBlock.vue";
import ChatRunnerStatus from "../../features/chat/components/ChatRunnerStatus.vue";
import ChatTeamMemberStrip, { type TeamMemberLane } from "./ChatTeamMemberStrip.vue";
import ChatPendingQueue from "./ChatPendingQueue.vue";
import ChatComposer from "./ChatComposer.vue";
import ChatHeaderUsagePanel from "./ChatHeaderUsagePanel.vue";
import ChatHeaderPromptBar from "./ChatHeaderPromptBar.vue";
import type { RunStatusValue } from "../../features/chat/types";
import { useChatMessageRow } from "../../features/chat/useChatMessageRow";
import {
  CHAT_VIRTUAL_ROW_ESTIMATE,
  CHAT_VIRTUAL_SCROLL_THRESHOLD,
} from "../../features/chat/chatListVirtual";
import {
  groupMessagesByTurn,
  type TurnBlockGroup,
} from "../../features/chat/groupMessagesByTurn";
import { useTurnBlockEnabled } from "../../features/chat/useTurnBlock";
import { useChatMessageScroll, useChatCodeCopy } from "../../features/chat/composables/useChatMessageScroll";
import { useChatScrollTitle } from "../../features/chat/useChatScrollTitle";
import type { A2UIUserActionPayload } from "../../features/chat/a2uiUserAction";
import type { Message, ReactToolLinkIndex } from "../../features/chat/types";
import type { ComposerUsageSnapshot } from "../../features/chat/composerUsageMetrics";
import type { ArtifactMeta } from "../../features/artifact/types";
import type { ChatAttachment } from "./types";

type Option = { label: string; value: string; caption?: string };

const props = defineProps<{
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
  sessionTotalTokens?: number | null;
  knowledgeBaseOptions?: Option[];
  selectedKnowledgeBases?: string[];
  isDark: boolean;
  sending?: boolean;
  inputDisabled?: boolean;
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
  showBackgroundJobs?: boolean;
  agentId?: string;
  jobsRefreshNonce?: number;
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
  "focus-turn": [turnId: string];
  navigate: [route: { name: string; params: Record<string, string> }];
  "focus-turn-cleared": [];
  "a2ui-user-action": [payload: A2UIUserActionPayload];
  feedback: [payload: { messageId: string; rating: "positive" | "negative" }];
  retry: [messageId: string];
  "dismiss-failed": [messageId: string];
}>();

const { t } = useI18n();
const messagesRef = computed(() => props.messages);
const messageRow = useChatMessageRow(messagesRef);

const teamMemberLanes = computed((): TeamMemberLane[] => {
  if (!props.isTeamSession) return [];
  const lanes = new Map<string, TeamMemberLane>();
  for (const message of props.messages) {
    if (!messageRow.isTeamMember(message)) continue;
    const key = messageRow.messageIdentityKey(message);
    const meta = messageRow.teamMemberMeta(message);
    const label = meta?.name || meta?.agent_key || messageRow.displayMessageName(message);
    const streaming = message.status === "streaming" || message.status === "tool_running";
    const prev = lanes.get(key);
    lanes.set(key, {
      key,
      label,
      streaming: (prev?.streaming ?? false) || streaming,
    });
  }
  return [...lanes.values()];
});

const useTurnBlockMode = computed(() => useTurnBlockEnabled() && !props.isTeamSession);
const turnBlocks = computed((): TurnBlockGroup[] =>
  useTurnBlockMode.value ? groupMessagesByTurn(props.messages) : []
);

type TimelineItem =
  | { kind: "block"; block: TurnBlockGroup }
  | { kind: "message"; message: Message };

const timelineItems = computed((): TimelineItem[] => {
  if (useTurnBlockMode.value) {
    return turnBlocks.value.map((block) => ({ kind: "block" as const, block }));
  }
  return props.messages.map((message) => ({ kind: "message" as const, message }));
});

const useVirtualMessageList = computed(() => timelineItems.value.length >= CHAT_VIRTUAL_SCROLL_THRESHOLD);
const virtualRowSize = CHAT_VIRTUAL_ROW_ESTIMATE;
const virtualScrollRef = ref<QVirtualScroll | null>(null);
const messagesScrollEl = ref<HTMLElement | null>(null);

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
