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
      P2 #6: 统一使用 DynamicScroller，移除阈值切换。
      原实现在 conversationTurns.length > 100 时从 v-for 切换到 DynamicScroller，
      导致组件树完全重建（v-if/v-else 切换不同组件类型），在阈值临界点引起 UI 冻结。
      vue-virtual-scroller 对短列表性能足够好，无需双模式。
    -->
    <DynamicScroller
      v-else
      ref="virtualScrollRef"
      :items="conversationTurns"
      :min-item-size="80"
      key-field="id"
      class="col chat-messages__viewport chat-messages__viewport--virtual"
      @scroll.passive="$emit('scroll', $event)"
      @click="$emit('messages-click', $event)"
    >
      <template #default="{ item, itemIndex, active }">
        <DynamicScrollerItem :item="item" :active="active" :data-index="itemIndex" :data-turn-id="item.id">
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

// P2 #6: 原阈值 VIRTUAL_SCROLL_THRESHOLD=100 已移除，统一使用 DynamicScroller。
// 原实现在消息数 >100 时从 v-for 切换到 DynamicScroller，导致组件树完全重建。

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
// P2 #6: normalScrollEl 已移除（统一使用 DynamicScroller，不再有 v-for 模式）
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

/**
 * P2 #6: 统一使用 DynamicScroller。useVirtualScroll 在有消息时始终为 true，
 * 保留暴露是为了 ChatMessagePanel/useChatScrollTitle 的 API 兼容性。
 */
const useVirtualScroll = computed(() => conversationTurns.value.length > 0);

/**
 * T8.3: Scroll to a specific turn by id.
 * Uses DynamicScroller.scrollToItem. Returns true if the turn was found.
 */
async function scrollToTurnId(turnId: string): Promise<boolean> {
  const id = turnId.trim();
  if (!id) return false;
  if (!virtualScrollRef.value) return false;
  const index = conversationTurns.value.findIndex((t) => t.id === id);
  if (index < 0) return false;
  // DynamicScroller.scrollToItem scrolls the item into view, then we wait
  // for the DOM to update before the caller can highlight the element.
  virtualScrollRef.value.scrollToItem(index);
  await nextTick();
  return true;
}

defineExpose({
  emptyScrollEl,
  virtualScrollRef,
  useVirtualScroll,
  scrollToTurnId,
  getScrollTarget: () => {
    if (!props.messages.length) return emptyScrollEl.value;
    // P2 #6: 统一使用 DynamicScroller 的根元素作为滚动容器
    const vsRef = virtualScrollRef.value as { $el?: HTMLElement } | null;
    return vsRef?.$el ?? null;
  },
});
</script>
