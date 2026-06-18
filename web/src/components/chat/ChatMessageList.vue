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
      T8.3 VirtualScroller: messages > VIRTUAL_SCROLL_THRESHOLD use DynamicScroller
      for dynamic-height virtualization. Falls back to normal v-for for short lists.
    -->
    <DynamicScroller
      v-else-if="useVirtualScroll"
      ref="virtualScrollRef"
      :items="conversationTurns"
      :min-item-size="80"
      key-field="id"
      class="col chat-messages__viewport chat-messages__viewport--virtual"
      @scroll.passive="$emit('scroll', $event)"
      @click="$emit('messages-click', $event)"
    >
      <template #default="{ item, itemIndex, active }">
        <DynamicScrollerItem
          :item="item"
          :active="active"
          :data-index="itemIndex"
          :data-turn-id="item.id"
        >
          <ConversationTurn
            :turn="item"
            @confirm="(id: string, approved: boolean) => $emit('confirm', id, approved)"
            @error-retry="(e: ErrorEvent) => $emit('error-retry', e)"
            @error-switch-model="(e: ErrorEvent) => $emit('error-switch-model', e)"
            @error-rephrase="(e: ErrorEvent) => $emit('error-rephrase', e)"
            @error-check-config="(e: ErrorEvent) => $emit('error-check-config', e)"
            @error-remove-attachment="(e: ErrorEvent) => $emit('error-remove-attachment', e)"
            @error-relogin="(e: ErrorEvent) => $emit('error-relogin', e)"
          />
        </DynamicScrollerItem>
      </template>
    </DynamicScroller>
    <div
      v-else
      ref="normalScrollEl"
      class="col relative-position chat-messages__viewport"
      @scroll.passive="$emit('scroll', $event)"
      @click="$emit('messages-click', $event)"
    >
      <ConversationTurn
        v-for="turn in conversationTurns"
        :key="turn.id"
        :turn="turn"
        @confirm="(id, approved) => $emit('confirm', id, approved)"
        @error-retry="(e) => $emit('error-retry', e)"
        @error-switch-model="(e) => $emit('error-switch-model', e)"
        @error-rephrase="(e) => $emit('error-rephrase', e)"
        @error-check-config="(e) => $emit('error-check-config', e)"
        @error-remove-attachment="(e) => $emit('error-remove-attachment', e)"
        @error-relogin="(e) => $emit('error-relogin', e)"
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
import { ref, computed, nextTick } from 'vue';
import { useI18n } from 'vue-i18n';
import { DynamicScroller, DynamicScrollerItem } from 'vue-virtual-scroller';
import 'vue-virtual-scroller/dist/vue-virtual-scroller.css';
import ChatPendingQueue from './ChatPendingQueue.vue';
import ConversationTurn from './ConversationTurn.vue';
import { useConversationTimeline } from '../../features/chat/composables/useConversationTimeline';
import type { Message, PendingMessage } from '../../features/chat/types';
import type { A2UIUserActionPayload } from '../../features/chat/a2uiUserAction';
import type { ArtifactMeta } from '../../features/artifact/types';
import type { Envelope } from '../../realtime/envelope';
import type { Activity as TimelineActivity } from '../../features/chat/activityTimelineTypes';
import type { ErrorEvent } from '../../features/chat/streamEventTypes';

/** T8.3: Virtual scrolling threshold — only enable for long conversations. */
const VIRTUAL_SCROLL_THRESHOLD = 100;

const props = defineProps<{
  sessionKey: string;
  messages: Message[];
  pendingMessages: PendingMessage[];
  isDark: boolean;
  isTeamSession?: boolean;
  plannerKind?: string;
  reasoningSidebarOpen?: boolean;
  showScrollBtn: boolean;
  progressEnvelopes?: readonly Envelope[];
  /** AF-FE-06: Activity-First timeline activities (from useActivityTimeline) */
  activityTimelineActivities?: readonly TimelineActivity[];
  /** AF-FE-06: Agent key from Activity data */
  activityAgentKey?: string;
  /** AF-FE-06: Root task content from Activity data */
  activityTaskContent?: string;
  /** AF-FE-06: Activity tree for building TeamPanel */
  activityTree?: readonly import('../../features/chat/activityTypes').ActivityTreeNode[];
  /** AF-FE-14: Raw Activity records (with turnId) for grouping by turn */
  activityRawRecords?: readonly import('../../features/chat/activityTypes').Activity[];
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
}>();

const { t } = useI18n();

const quickStartHints = [
  { icon: 'edit_note', text: t('chat.hintWrite', '帮我写一段代码') },
  { icon: 'psychology', text: t('chat.hintAnalyze', '分析这个问题') },
  { icon: 'translate', text: t('chat.hintTranslate', '翻译一段文字') },
];

const emptyScrollEl = ref<HTMLElement | null>(null);
const normalScrollEl = ref<HTMLElement | null>(null);
// T8.3: DynamicScroller ref for virtual scrolling mode
const virtualScrollRef = ref<InstanceType<typeof DynamicScroller> | null>(null);

const { conversationTurns } = useConversationTimeline({
  messages: computed(() => props.messages),
  isTeamSession: props.isTeamSession,
  plannerKind: computed(() => props.plannerKind ?? ''),
  progressEnvelopes: computed(() => props.progressEnvelopes ?? []),
  activityTimelineActivities: computed(() => props.activityTimelineActivities ?? []),
  activityAgentKey: computed(() => props.activityAgentKey ?? ''),
  activityTaskContent: computed(() => props.activityTaskContent ?? null),
  activityTree: computed(() => props.activityTree ?? []),
  activityRawRecords: computed(() => props.activityRawRecords ?? []),
});

/** T8.3: Enable virtual scrolling only for long conversations to keep short-list UX simple. */
const useVirtualScroll = computed(() => conversationTurns.value.length > VIRTUAL_SCROLL_THRESHOLD);

/**
 * T8.3: Scroll to a specific turn by id.
 * In virtual mode, uses DynamicScroller.scrollToItem; in normal mode, uses DOM query.
 * Returns true if the turn was found and scrolled to.
 */
async function scrollToTurnId(turnId: string): Promise<boolean> {
  const id = turnId.trim();
  if (!id) return false;
  if (useVirtualScroll.value && virtualScrollRef.value) {
    const index = conversationTurns.value.findIndex((t) => t.id === id);
    if (index < 0) return false;
    // DynamicScroller.scrollToItem scrolls the item into view, then we wait
    // for the DOM to update before the caller can highlight the element.
    virtualScrollRef.value.scrollToItem(index);
    await nextTick();
    return true;
  }
  // Normal mode: caller (useChatMessageScroll) handles DOM query + highlight.
  return false;
}

defineExpose({
  emptyScrollEl,
  normalScrollEl,
  virtualScrollRef,
  useVirtualScroll,
  scrollToTurnId,
  getScrollTarget: () => {
    if (!props.messages.length) return emptyScrollEl.value;
    if (useVirtualScroll.value) {
      // DynamicScroller root element (the scrollable container)
      const vsRef = virtualScrollRef.value as { $el?: HTMLElement } | null;
      return vsRef?.$el ?? null;
    }
    return normalScrollEl.value;
  },
});
</script>
