<template>
  <div :key="sessionKey" class="chat-messages col column no-wrap" style="min-height: 0">
    <div
      v-if="!messages.length"
      ref="emptyScrollEl"
      class="col relative-position chat-messages__viewport"
      @click="$emit('messages-click')"
    >
      <div class="chat-empty-state column items-center justify-center">
        <div class="chat-empty-state__halo">
          <q-icon name="auto_awesome" size="36px" color="primary" />
        </div>
        <div class="chat-empty-state__title q-mt-md">{{ t("chat.emptyMessages") }}</div>
        <div class="chat-empty-state__hint text-caption q-mt-xs">{{ t("chat.inputLabel") }}</div>
        <div class="chat-empty-state__suggestions q-mt-lg row q-gutter-sm justify-center">
          <q-chip
            v-for="(s, i) in quickStartHints"
            :key="i"
            clickable
            outline
            color="primary"
            class="chat-empty-state__chip"
            :icon="s.icon"
            :label="s.text"
          />
        </div>
      </div>
    </div>
    <q-virtual-scroll
      v-else-if="useVirtual"
      ref="virtualScrollRef"
      class="col chat-messages__viewport"
      style="min-height: 0"
      :items="timelineItems"
      :virtual-scroll-item-size="virtualRowSize"
      :virtual-scroll-slice-size="48"
      :virtual-scroll-slice-ratio-before="2"
      :virtual-scroll-slice-ratio-after="2"
      v-slot="{ item, index }"
      @scroll="$emit('scroll', $event)"
      @click="$emit('messages-click')"
    >
      <TurnBlock
        v-if="useTurnBlockMode && item.kind === 'block'"
        :block="item.block"
        :focused="turnIsFocused(item.block.turnId, item.block.user?.id)"
        :all-messages="messages"
        :is-dark="isDark"
        :is-team-session="isTeamSession"
        :planner-kind="plannerKind"
        :react-tool-link-index="reactToolLinkIndex"
        :reasoning-sidebar-open="reasoningSidebarOpen"
        @a2ui-user-action="(p) => $emit('a2ui-user-action', p)"
        @feedback="(p) => $emit('feedback', p)"
        @regenerate="(msg) => $emit('regenerate', msg)"
        @retry="(id) => $emit('retry', id)"
        @dismiss-failed="(id) => $emit('dismiss-failed', id)"
        @pin-reasoning="(id) => $emit('pin-reasoning-message', id)"
      />
      <ChatMessageRow
        v-else
        :message="item.message"
        :index="index"
        v-memo="[item.message.id, item.message.content_markdown, item.message.status, item.message.options_json, isDark, plannerKind]"
        :messages="messages"
        :is-dark="isDark"
        :is-team-session="isTeamSession"
        :planner-kind="plannerKind"
        :react-tool-link-index="reactToolLinkIndex"
        :reasoning-sidebar-open="reasoningSidebarOpen"
        @a2ui-user-action="(p) => $emit('a2ui-user-action', p)"
        @feedback="(p) => $emit('feedback', p)"
        @retry="(id) => $emit('retry', id)"
        @dismiss-failed="(id) => $emit('dismiss-failed', id)"
        @attachment-deleted="(id) => $emit('attachment-deleted', id)"
        @download-artifact="(meta) => $emit('download-artifact', meta)"
        @regenerate="(msg) => $emit('regenerate', msg)"
        @pin-reasoning="(id) => $emit('pin-reasoning-message', id)"
      />
    </q-virtual-scroll>
    <div
      v-else
      ref="normalScrollEl"
      class="col relative-position chat-messages__viewport"
      @scroll.passive="$emit('scroll', $event)"
      @click="$emit('messages-click')"
    >
      <template v-if="useTurnBlockMode">
        <TurnBlock
          v-for="block in turnBlocks"
          :key="block.turnId"
          :block="block"
          :focused="turnIsFocused(block.turnId, block.user?.id)"
          :all-messages="messages"
          :is-dark="isDark"
          :is-team-session="isTeamSession"
          :planner-kind="plannerKind"
          :react-tool-link-index="reactToolLinkIndex"
          :reasoning-sidebar-open="reasoningSidebarOpen"
          @a2ui-user-action="(p) => $emit('a2ui-user-action', p)"
          @feedback="(p) => $emit('feedback', p)"
          @regenerate="(msg) => $emit('regenerate', msg)"
          @retry="(id) => $emit('retry', id)"
          @dismiss-failed="(id) => $emit('dismiss-failed', id)"
          @pin-reasoning="(id) => $emit('pin-reasoning-message', id)"
        />
      </template>
      <ChatMessageRow
        v-else
        v-for="(message, idx) in messages"
        :key="message.id"
        v-memo="[message.id, message.content_markdown, message.status, message.options_json, isDark, plannerKind]"
        :message="message"
        :index="idx"
        :messages="messages"
        :is-dark="isDark"
        :is-team-session="isTeamSession"
        :planner-kind="plannerKind"
        :react-tool-link-index="reactToolLinkIndex"
        :reasoning-sidebar-open="reasoningSidebarOpen"
        @a2ui-user-action="(p) => $emit('a2ui-user-action', p)"
        @retry="(id) => $emit('retry', id)"
        @dismiss-failed="(id) => $emit('dismiss-failed', id)"
        @attachment-deleted="(id) => $emit('attachment-deleted', id)"
        @download-artifact="(meta) => $emit('download-artifact', meta)"
        @regenerate="(msg) => $emit('regenerate', msg)"
        @pin-reasoning="(id) => $emit('pin-reasoning-message', id)"
      />
    </div>
    <ChatPendingQueue
      :messages="pendingMessages"
      :is-dark="isDark"
      @cancel-pending="(id) => $emit('cancel-pending', id)"
      @update-pending="(id, content) => $emit('update-pending', id, content)"
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
        @click="$emit('scroll-to-bottom', true)"
      />
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { useI18n } from "vue-i18n";
import type { QVirtualScroll } from "quasar";
import TurnBlock from "./TurnBlock.vue";
import ChatMessageRow from "./ChatMessageRow.vue";
import ChatPendingQueue from "./ChatPendingQueue.vue";
import type { Message, ReactToolLinkIndex, PendingMessage } from "../../features/chat/types";
import type { TurnBlockGroup } from "../../features/chat/groupMessagesByTurn";
import type { TimelineItem } from "../../features/chat/composables/useChatTimeline";

const props = defineProps<{
  sessionKey: string;
  messages: Message[];
  pendingMessages: PendingMessage[];
  isDark: boolean;
  isTeamSession?: boolean;
  plannerKind?: string;
  reactToolLinkIndex: ReactToolLinkIndex;
  reasoningSidebarOpen?: boolean;
  useVirtual: boolean;
  useTurnBlockMode: boolean;
  timelineItems: TimelineItem[];
  turnBlocks: TurnBlockGroup[];
  virtualRowSize: number;
  showScrollBtn: boolean;
  turnIsFocused: (turnId: string, userId?: string) => boolean;
}>();

defineEmits<{
  "messages-click": [];
  scroll: [event: Event];
  "scroll-to-bottom": [smooth: boolean];
  "a2ui-user-action": [payload: any];
  feedback: [payload: { messageId: string; rating: "positive" | "negative" }];
  regenerate: [message: Message];
  retry: [messageId: string];
  "dismiss-failed": [messageId: string];
  "attachment-deleted": [id: string];
  "download-artifact": [meta: any];
  "pin-reasoning-message": [messageId: string];
  "cancel-pending": [pendingId: string];
  "update-pending": [pendingId: string, content: string];
}>();

const { t } = useI18n();

const quickStartHints = [
  { icon: "edit_note", text: t("chat.hintWrite", "帮我写一段代码") },
  { icon: "psychology", text: t("chat.hintAnalyze", "分析这个问题") },
  { icon: "translate", text: t("chat.hintTranslate", "翻译一段文字") },
];

const emptyScrollEl = ref<HTMLElement | null>(null);
const virtualScrollRef = ref<QVirtualScroll | null>(null);
const normalScrollEl = ref<HTMLElement | null>(null);

defineExpose({
  emptyScrollEl,
  virtualScrollRef,
  normalScrollEl,
  getScrollTarget: () => {
    if (props.useVirtual) return virtualScrollRef.value?.$el ?? null;
    if (!props.messages.length) return emptyScrollEl.value;
    return normalScrollEl.value;
  },
});
</script>
