<!-- web/src/components/chat/v2/TaskList.vue -->
<template>
  <div
    class="task-list"
    data-testid="task-list-window"
    :data-task-total="tasks.length"
    :data-window-start="windowStart"
    :data-window-end="windowEnd"
    :aria-busy="paging"
  >
    <button
      v-if="hasEarlier"
      type="button"
      class="task-list__page"
      data-testid="task-load-earlier"
      :disabled="paging"
      @click="loadEarlier"
    >
      {{ t('taskWorkspace.loadMore') }}
    </button>
    <TaskCard
      v-for="task in visibleTasks"
      :key="task.ID"
      :task="task"
      :hydrated="queries.isTaskHydrated(task.ID)"
      :hydration-state="queries.taskHydrationState(task.ID)"
      :collapsed="lazy.isCollapsed(task.ID)"
      @regenerate="(t) => $emit('regenerate', t)"
      @add-to-eval="(t) => $emit('add-to-eval', t)"
      @feedback="(p) => $emit('feedback', p)"
      @resume-task="(t) => $emit('resume-task', t)"
      @pause-agent="(sid) => $emit('pause-agent', sid)"
      @inject-agent="(p) => $emit('inject-agent', p)"
      @expand="(ids) => $emit('expand', ids)"
      @confirm-step="(p) => $emit('confirm-step', p)"
      @fork-turn="(t) => $emit('fork-turn', t)"
      @error-retry="(s) => $emit('error-retry', s)"
      @error-switch-model="(s) => $emit('error-switch-model', s)"
      @submit-clarification="(p) => $emit('submit-clarification', p)"
      @hydrate="(t) => lazy.expandTask(t.ID)"
      @toggle-collapse="(t) => lazy.toggleCollapse(t.ID)"
    />
    <button
      v-if="hasLater"
      type="button"
      class="task-list__page"
      data-testid="task-load-later"
      :disabled="paging"
      @click="loadLater"
    >
      {{ t('taskWorkspace.backToLatest') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, ref, watch, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import { useLazyTaskHydration, CHAT_SCROLL_EL_KEY } from '../../../features/chat/composables/useLazyTaskHydration';
import type { Step, Task, Turn } from '../../../features/chat/v2Types';
import type { ConfirmStepPayload, SubmitClarificationPayload } from '../../../features/chat/types';
import TaskCard from './TaskCard.vue';
import { useScrollToActivity } from '../../../features/chat/composables/useScrollToActivity';

const props = defineProps<{ sessionId: string }>();
defineEmits<{
  regenerate: [task: Task];
  'add-to-eval': [task: Task];
  feedback: [payload: { task: Task; rating: 'positive' | 'negative' }];
  'resume-task': [task: Task];
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  expand: [sessionIds: string[]];
  'confirm-step': [payload: ConfirmStepPayload];
  'fork-turn': [turn: Turn];
  'error-retry': [step: Step];
  'error-switch-model': [step: Step];
  'submit-clarification': [payload: SubmitClarificationPayload];
}>();
const queries = useActivityQueries();
let t: (key: string) => string;
try {
  t = useI18n().t;
} catch {
  t = (key) => key;
}
const tasks = computed(() => queries.getSessionTasks(props.sessionId));

// 懒水合编排：折叠卡滚入视口 500ms / 点击 → hydrateTask。
// components 禁止直访 store（layer 检查），needsHydration/hydrate 走 useActivityQueries 门面。
const scrollEl = inject<Ref<HTMLElement | null>>(CHAT_SCROLL_EL_KEY, computed(() => null) as never);
const lazy = useLazyTaskHydration({
  scrollEl,
  needsHydration: (taskId) => !queries.isTaskHydrated(taskId) && queries.taskHydrationState(taskId) !== 'loading',
  hydrate: (taskId) => queries.hydrateTask(taskId),
});

const TASK_WINDOW_SIZE = 60;
const TASK_WINDOW_STEP = 30;
const windowStart = ref(0);
const windowEnd = ref(0);
const paging = ref(false);
const visibleTasks = computed(() => tasks.value.slice(windowStart.value, windowEnd.value));
const hasEarlier = computed(() => windowStart.value > 0);
const hasLater = computed(() => windowEnd.value < tasks.value.length);
let lastScrollTop = 0;

function showLatest() {
  windowEnd.value = tasks.value.length;
  windowStart.value = Math.max(0, windowEnd.value - TASK_WINDOW_SIZE);
}

function captureAnchor(fromEnd: boolean) {
  const viewport = scrollEl.value;
  if (!viewport) return null;
  const viewportRect = viewport.getBoundingClientRect();
  const cards = Array.from(viewport.querySelectorAll<HTMLElement>('.task-card[data-task-id]')).filter((card) => {
    const rect = card.getBoundingClientRect();
    return rect.bottom > viewportRect.top && rect.top < viewportRect.bottom;
  });
  const card = fromEnd ? cards.at(-1) : cards[0];
  return card ? { id: card.dataset.taskId!, top: card.getBoundingClientRect().top } : null;
}

async function moveWindow(direction: 'earlier' | 'later') {
  if (paging.value || (direction === 'earlier' ? !hasEarlier.value : !hasLater.value)) return;
  paging.value = true;
  const anchor = captureAnchor(direction === 'later');
  if (direction === 'earlier') {
    windowStart.value = Math.max(0, windowStart.value - TASK_WINDOW_STEP);
    windowEnd.value = Math.min(tasks.value.length, windowStart.value + TASK_WINDOW_SIZE);
  } else {
    windowEnd.value = Math.min(tasks.value.length, windowEnd.value + TASK_WINDOW_STEP);
    windowStart.value = Math.max(0, windowEnd.value - TASK_WINDOW_SIZE);
  }
  await nextTick();
  if (anchor && scrollEl.value) {
    const card = scrollEl.value.querySelector<HTMLElement>(`[data-task-id="${CSS.escape(anchor.id)}"]`);
    if (card) scrollEl.value.scrollTop += card.getBoundingClientRect().top - anchor.top;
    lastScrollTop = scrollEl.value.scrollTop;
  }
  paging.value = false;
}

async function loadEarlier() {
  await moveWindow('earlier');
}
async function loadLater() {
  await moveWindow('later');
}

function onWindowScroll() {
  const viewport = scrollEl.value;
  if (!viewport || paging.value) return;
  const top = viewport.scrollTop;
  const delta = top - lastScrollTop;
  lastScrollTop = top;
  if (delta < 0 && top <= 24 && hasEarlier.value) void loadEarlier();
  else if (delta > 0 && top + viewport.clientHeight >= viewport.scrollHeight - 24 && hasLater.value) void loadLater();
}

watch(
  scrollEl,
  (viewport, previous) => {
    previous?.removeEventListener('scroll', onWindowScroll);
    viewport?.addEventListener('scroll', onWindowScroll, { passive: true });
    lastScrollTop = viewport?.scrollTop ?? 0;
  },
  { immediate: true },
);
onBeforeUnmount(() => scrollEl.value?.removeEventListener('scroll', onWindowScroll));
watch(() => props.sessionId, showLatest);
watch(
  () => tasks.value.length,
  (length, previousLength) => {
    if (!length) {
      windowStart.value = 0;
      windowEnd.value = 0;
    } else if (!previousLength || windowEnd.value >= previousLength) showLatest();
    else {
      windowEnd.value = Math.min(windowEnd.value, length);
      windowStart.value = Math.min(windowStart.value, Math.max(0, windowEnd.value - TASK_WINDOW_SIZE));
    }
  },
  { immediate: true },
);

function revealTask(taskId: string) {
  const index = tasks.value.findIndex((task) => task.ID === taskId);
  if (index < 0 || (index >= windowStart.value && index < windowEnd.value)) return;
  windowStart.value = Math.max(0, Math.min(index - Math.floor(TASK_WINDOW_SIZE / 2), tasks.value.length - TASK_WINDOW_SIZE));
  windowEnd.value = Math.min(tasks.value.length, windowStart.value + TASK_WINDOW_SIZE);
}

const { taskLocateCommand } = useScrollToActivity();
watch(taskLocateCommand, async (command) => {
  if (!command || command.sessionId !== props.sessionId) return;
  revealTask(command.taskId);
  await lazy.expandTask(command.taskId);
  await nextTick();
  if (command !== taskLocateCommand.value || command.sessionId !== props.sessionId) return;
  const card = scrollEl.value?.querySelector<HTMLElement>(`[data-task-id="${CSS.escape(command.taskId)}"]`);
  const target = command.stepId
    ? (card?.querySelector<HTMLElement>(`[data-step-id="${CSS.escape(command.stepId)}"]`) ?? card)
    : command.turnId
      ? card?.querySelector<HTMLElement>(`[data-turn-id="${CSS.escape(command.turnId)}"]`)
      : card;
  if (target && !target.hasAttribute('tabindex')) target.tabIndex = -1;
  target?.focus({ preventScroll: true });
  target?.scrollIntoView({ block: 'start', behavior: 'auto' });
});

// tasks 渲染 / 水合状态变化后同步观察集合（flush: 'post' 保证 DOM 已更新）。
watch(
  () => visibleTasks.value.map((t) => `${t.ID}:${queries.isTaskHydrated(t.ID)}`).join('|'),
  async () => {
    await nextTick();
    lazy.syncCards();
  },
  { immediate: true, flush: 'post' },
);
</script>

<style scoped>
.task-list {
  min-width: 0;
}
.task-list__page {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  min-height: 32px;
  padding: 6px 12px;
  border: 0;
  color: var(--color-accent);
  background: transparent;
  font-size: 13px;
  line-height: 20px;
  cursor: pointer;
}
.task-list__page:disabled {
  cursor: wait;
  opacity: 0.6;
}
</style>
