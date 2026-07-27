<template>
  <div :key="sessionKey" class="chat-messages col column no-wrap" style="min-height: 0">
    <div
      v-if="!hasV2Activities"
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
    <div
      v-else
      ref="scrollViewportEl"
      class="col chat-messages__viewport chat-messages__viewport--virtual"
      @scroll.passive="$emit('scroll', $event)"
      @click="$emit('messages-click', $event)"
    >
      <SessionPanelV2
        :session-id="sessionId ?? ''"
        @regenerate="(t) => $emit('regenerate-v2', t)"
        @resume-task="(t) => $emit('resume-task', t)"
        @pause-agent="(sid) => $emit('pause-agent', sid)"
        @inject-agent="(p) => $emit('inject-agent', p)"
        @expand="(ids) => $emit('expand', ids)"
        @confirm-step="(p) => $emit('confirm-step', p)"
        @submit-clarification="(p) => $emit('submit-clarification', p)"
      />
    </div>
    <ChatPendingQueue
      :messages="pendingMessages"
      :is-dark="isDark"
      @cancel-pending="(id) => $emit('cancel-pending', id)"
      @interrupt-pending="(id) => $emit('interrupt-pending', id)"
      @update-pending="(id, content) => $emit('update-pending', id, content)"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, provide } from 'vue';
import { useI18n } from 'vue-i18n';
import ChatPendingQueue from './ChatPendingQueue.vue';
import SessionPanelV2 from './v2/SessionPanel.vue';
import { useScrollToActivity } from '../../features/chat/composables/useScrollToActivity';
import { useActivityQueries } from '../../features/chat/composables/useActivityQueries';
import { CHAT_SCROLL_EL_KEY } from '../../features/chat/composables/useLazyTaskHydration';
import type { Message, PendingMessage, ConfirmStepPayload, SubmitClarificationPayload } from '../../features/chat/types';
import type { A2UIUserActionPayload } from '../../features/chat/a2uiUserAction';
import type { ArtifactMeta } from '../../features/artifact/types';
import type { Step } from '../../features/chat/v2Types';

const AUTO_EXPAND_HOLD_MS = 3000;

const props = defineProps<{
  sessionKey: string;
  /** legacy unused; hydrate via v2 store */
  messages?: Message[];
  pendingMessages: PendingMessage[];
  isDark: boolean;
  isTeamSession?: boolean;
  plannerKind?: string;
  reasoningSidebarOpen?: boolean;
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
  'confirm-step': [payload: ConfirmStepPayload];
  'submit-clarification': [payload: SubmitClarificationPayload];
  'error-retry': [step: Step];
  'error-switch-model': [step: Step];
  'error-rephrase': [step: Step];
  'error-check-config': [step: Step];
  'error-remove-attachment': [step: Step];
  'error-relogin': [step: Step];
  'expand-member': [payload: { agentKey: string; agentName?: string; teamId?: string }];
  'enter-session': [sessionId: string];
  'cancel-team': [teamId: string];
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
  'regenerate-v2': [task: import('../../features/chat/v2Types').Task];
  'resume-task': [task: import('../../features/chat/v2Types').Task];
}>();

const { t } = useI18n();

const activityStore = useActivityQueries();
const hasV2Activities = computed(() =>
  props.sessionId ? activityStore.getSessionTasks(props.sessionId).length > 0 : false,
);

const quickStartHints = [
  { icon: 'edit_note', text: t('chat.hintWrite', '帮我写一段代码') },
  { icon: 'psychology', text: t('chat.hintAnalyze', '分析这个问题') },
  { icon: 'translate', text: t('chat.hintTranslate', '翻译一段文字') },
];

const emptyScrollEl = ref<HTMLElement | null>(null);
const scrollViewportEl = ref<HTMLElement | null>(null);

// 懒水合 composable 的 observer root（折叠卡视口感知）。
provide(CHAT_SCROLL_EL_KEY, scrollViewportEl);

// T8.6: 点击左侧 Agent 卡片 → 滚动并高亮中间面板对应的 AgentCard。
const { locateCommand } = useScrollToActivity();
const autoExpandFor = ref<{ agentKey: string; teamId: string } | null>(null);
provide('chat:autoExpandFor', autoExpandFor);
let autoExpandTimer: ReturnType<typeof setTimeout> | null = null;

watch(locateCommand, async (cmd) => {
  if (!cmd || !scrollViewportEl.value) return;
  autoExpandFor.value = { agentKey: cmd.agentKey, teamId: cmd.teamId || '' };
  if (autoExpandTimer) clearTimeout(autoExpandTimer);
  autoExpandTimer = setTimeout(() => {
    autoExpandFor.value = null;
  }, AUTO_EXPAND_HOLD_MS);
  await nextTick();
  let el: HTMLElement | null = null;
  if (cmd.teamId) {
    el = scrollViewportEl.value.querySelector(
      `[data-team-id="${cssEscape(cmd.teamId)}"] [data-agent-key="${cssEscape(cmd.agentKey)}"]`,
    ) as HTMLElement | null;
  }
  if (!el) {
    el = scrollViewportEl.value.querySelector(`[data-agent-key="${cssEscape(cmd.agentKey)}"]`) as HTMLElement | null;
  }
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
    if (!hasV2Activities.value) return emptyScrollEl.value;
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
