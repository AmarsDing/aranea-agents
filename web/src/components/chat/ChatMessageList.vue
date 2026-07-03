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
      <SessionPanelV2 :session-id="sessionId ?? ''" />
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
import { ref, watch, nextTick } from 'vue';
import { useI18n } from 'vue-i18n';
import ChatPendingQueue from './ChatPendingQueue.vue';
import SessionPanelV2 from './v2/SessionPanel.vue';
import { useScrollToActivity } from '../../features/chat/composables/useScrollToActivity';
import type { Message, PendingMessage } from '../../features/chat/types';
import type { A2UIUserActionPayload } from '../../features/chat/a2uiUserAction';
import type { ArtifactMeta } from '../../features/artifact/types';
import type { ErrorEvent } from '../../features/chat/streamEventTypes';

const AUTO_EXPAND_HOLD_MS = 3000;

const props = defineProps<{
  sessionKey: string;
  messages: Message[];
  pendingMessages: PendingMessage[];
  isDark: boolean;
  isTeamSession?: boolean;
  plannerKind?: string;
  reasoningSidebarOpen?: boolean;
  showScrollBtn: boolean;
  /** v2: active session id for SessionPanelV2 (renders the v2 task tree). */
  sessionId?: string;
  /** P1#1/2: agent key → display name lookup for TeamCard/AgentCard. */
  agentMap?: Map<string, { displayName: string; agentKey: string }>;
  /** P1#3: parent run status to gate cancel button visibility. */
  runStatus?: import('../../features/chat/types').RunStatusValue;
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
  'cancel-team': [teamId: string];
  'retry-team': [teamId: string];
  'pause-team': [teamId: string];
  'unpause-team': [teamId: string];
  'inject-team': [payload: { teamId: string; message: string }];
  'cancel-agent': [sessionId: string];
  'retry-agent': [sessionId: string];
  'pause-agent': [sessionId: string];
  'resume-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  // T5.2/T5.3 / §B.7.2: Forward team-card / agent-card expand events upstream
  // so ChatPage can lazy-load member/child session activities.
  expand: [sessionIds: string[]];
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

// T8.6: 点击左侧 Agent 卡片 → 滚动并高亮中间面板对应的 AgentCard。
// useScrollToActivity 为模块级 ref 单例，ChatEntitySidebar 调用 locate() 触发此处 watch。
const { locateCommand } = useScrollToActivity();
const autoExpandFor = ref<string>('');
let autoExpandTimer: ReturnType<typeof window.setTimeout> | null = null;

watch(locateCommand, async (cmd) => {
  if (!cmd || !scrollViewportEl.value) return;
  // 触发父级 TeamCard / AgentCard 自动展开，确保目标节点可见
  autoExpandFor.value = cmd.agentKey || cmd.teamId || '';
  if (autoExpandTimer) window.clearTimeout(autoExpandTimer);
  autoExpandTimer = window.setTimeout(() => {
    autoExpandFor.value = '';
  }, AUTO_EXPAND_HOLD_MS);
  // 等待数据更新（如展开新会话）渲染到 DOM
  await nextTick();
  let el = scrollViewportEl.value.querySelector(`[data-agent-key="${cssEscape(cmd.agentKey)}"]`) as HTMLElement | null;
  // 多成员团队渲染为 TeamCard，按 teamId 二次定位
  if (!el && cmd.teamId) {
    el = scrollViewportEl.value.querySelector(`[data-team-id="${cssEscape(cmd.teamId)}"]`) as HTMLElement | null;
  }
  if (!el) return;
  el.scrollIntoView({ behavior: 'smooth', block: 'center' });
  el.classList.add('activity-locate-highlight');
  window.setTimeout(() => el.classList.remove('activity-locate-highlight'), 3000);
});

/** 转义 agentKey 中可能的 CSS 选择器特殊字符（如点、冒号），避免 querySelector 解析失败。 */
function cssEscape(value: string): string {
  // 简单转义：用 CSS.escape（现代浏览器支持），否则回退到双引号包裹
  if (typeof CSS !== 'undefined' && typeof CSS.escape === 'function') {
    return CSS.escape(value);
  }
  return value.replace(/["\\]/g, '\\$&');
}

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

<style lang="sass">
/* T8.6: 点击左侧 Agent 卡片定位高亮 — 全局样式（非 scoped），
   因为类通过 classList.add 加到子组件 AgentCard/TeamCard 根元素上，
   scoped 样式无法穿透。按 B.9.3 设计使用黄色闪烁 3 次后由 JS 移除。 */
@keyframes activity-locate-flash
  0%, 100%
    box-shadow: 0 0 0 0 rgba(233, 162, 59, 0)
  50%
    box-shadow: 0 0 0 3px rgba(233, 162, 59, 0.5)

.activity-locate-highlight
  animation: activity-locate-flash 1s ease-in-out 3
  border-radius: 6px
</style>
