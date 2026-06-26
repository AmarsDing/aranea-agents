<template>
  <div :key="sessionKey" class="chat-messages col column no-wrap" style="min-height: 0">
    <div
      v-if="!messages.length"
      ref="emptyScrollEl"
      class="col relative-position chat-messages__viewport"
      @click="$emit('messages-click', $event)"
    >
      <div class="chat-empty-state column items-center justify-center">
        <div class="chat-empty-state__halo">
          <q-icon name="auto_awesome" size="36px" color="accent" />
        </div>
        <div class="chat-empty-state__title q-mt-md">{{ t('chat.emptyMessages') }}</div>
        <div class="chat-empty-state__hint text-caption q-mt-xs">{{ t('chat.inputLabel') }}</div>
        <div class="chat-empty-state__suggestions q-mt-lg row q-gutter-sm justify-center">
          <q-chip
            v-for="(s, i) in quickStartHints"
            :key="i"
            clickable
            outline
            color="accent"
            class="chat-empty-state__chip"
            :icon="s.icon"
            :label="s.text"
          />
        </div>
      </div>
    </div>
    <!--
      Phase A refactor: ActivityStream renders the full sorted Activity list.
      DynamicScroller + ConversationTurn + useConversationTimeline removed.
      Virtual scrolling is a future optimization (YAGNI); for typical chat
      lengths the native DOM is sufficient. If long-conversation perf returns,
      wrap ActivityStream items in DynamicScroller with per-activity items.
    -->
    <div
      v-else
      ref="scrollViewportEl"
      class="col chat-messages__viewport chat-messages__viewport--virtual"
      @scroll.passive="$emit('scroll', $event)"
      @click="$emit('messages-click', $event)"
    >
      <ActivityStream
        :activities="props.activities"
        @confirm="(id: string, approved: boolean) => $emit('confirm', id, approved)"
        @error-retry="(e: ErrorEvent) => $emit('error-retry', e)"
        @error-switch-model="(e: ErrorEvent) => $emit('error-switch-model', e)"
        @error-rephrase="(e: ErrorEvent) => $emit('error-rephrase', e)"
        @error-check-config="(e: ErrorEvent) => $emit('error-check-config', e)"
        @error-remove-attachment="(e: ErrorEvent) => $emit('error-remove-attachment', e)"
        @error-relogin="(e: ErrorEvent) => $emit('error-relogin', e)"
        @expand-member="(p) => $emit('expand-member', p)"
        @enter-session="(sid) => $emit('enter-session', sid)"
      />
    </div>
    <ChatPendingQueue
      :messages="pendingMessages"
      :is-dark="isDark"
      @cancel-pending="(id) => $emit('cancel-pending', id)"
      @interrupt-pending="(id) => $emit('interrupt-pending', id)"
      @update-pending="(id, content) => $emit('update-pending', id, content)"
    />
    <transition name="chat-scroll-fade">
      <q-btn
        v-if="showScrollBtn"
        round
        unelevated
        color="accent"
        icon="arrow_downward"
        class="chat-scroll-bottom"
        :aria-label="t('chat.scrollToLatest')"
        @click="$emit('scroll-to-bottom', true)"
      />
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import ChatPendingQueue from './ChatPendingQueue.vue';
import ActivityStream from './ActivityStream.vue';
import type { Message, PendingMessage } from '../../features/chat/types';
import type { A2UIUserActionPayload } from '../../features/chat/a2uiUserAction';
import type { ArtifactMeta } from '../../features/artifact/types';
import type { Activity } from '../../features/chat/activityTypes';
import type { ErrorEvent } from '../../features/chat/streamEventTypes';

const props = defineProps<{
  sessionKey: string;
  messages: Message[];
  pendingMessages: PendingMessage[];
  isDark: boolean;
  isTeamSession?: boolean;
  plannerKind?: string;
  reasoningSidebarOpen?: boolean;
  showScrollBtn: boolean;
  /** Phase A: flat sorted Activity list (current session, includes task). */
  activities: Activity[];
}>();

defineEmits<{
  'messages-click': [event: MouseEvent];
  scroll: [event: Event];
  'scroll-to-bottom': [smooth: boolean];
  'a2ui-user-action': [payload: A2UIUserActionPayload];
  feedback: [payload: { messageId: string; rating: 'positive' | 'negative' }];
  regenerate: [message: Message];
  retry: [messageId: string];
  'dismiss-failed': [messageId: string];
  'attachment-deleted': [id: string];
  'download-artifact': [meta: ArtifactMeta];
  'pin-reasoning-message': [messageId: string];
  'cancel-pending': [pendingId: string];
  'interrupt-pending': [pendingId: string];
  'update-pending': [pendingId: string, content: string];
  confirm: [activityId: string, approved: boolean];
  'error-retry': [event: ErrorEvent];
  'error-switch-model': [event: ErrorEvent];
  'error-rephrase': [event: ErrorEvent];
  'error-check-config': [event: ErrorEvent];
  'error-remove-attachment': [event: ErrorEvent];
  'error-relogin': [event: ErrorEvent];
  'expand-member': [payload: { agentKey: string; agentName?: string; teamId?: string }];
  'enter-session': [sessionId: string];
}>();

const { t } = useI18n();

const quickStartHints = [
  { icon: 'edit_note', text: t('chat.hintWrite', '帮我写一段代码') },
  { icon: 'psychology', text: t('chat.hintAnalyze', '分析这个问题') },
  { icon: 'translate', text: t('chat.hintTranslate', '翻译一段文字') },
];

const emptyScrollEl = ref<HTMLElement | null>(null);
// Phase A: native scroll container replacing DynamicScroller.
const scrollViewportEl = ref<HTMLElement | null>(null);

/**
 * Phase A: Virtual scrolling removed (DynamicScroller gone).
 * `useVirtualScroll` is kept as `false` for ChatMessagePanel /
 * useChatScrollTitle API compatibility — they fall back to
 * `messagesScrollEl` (getScrollTarget) which now returns the native
 * scroll viewport.
 */
const useVirtualScroll = false;

/**
 * Phase A: `scrollToTurnId` previously used DynamicScroller.scrollToItem.
 * Without virtual scrolling, focus-turn relies on useChatMessageScroll's
 * DOM query (`[data-turn-id="..."]`). This stub keeps the exposed API
 * contract but is a no-op; ChatMessagePanel only calls it when
 * `useVirtualScroll` is true (now always false).
 */
async function scrollToTurnId(_turnId: string): Promise<boolean> {
  return false;
}

defineExpose({
  emptyScrollEl,
  scrollViewportEl,
  virtualScrollRef: null,
  useVirtualScroll,
  scrollToTurnId,
  getScrollTarget: () => {
    if (!props.messages.length) return emptyScrollEl.value;
    return scrollViewportEl.value;
  },
});
</script>
