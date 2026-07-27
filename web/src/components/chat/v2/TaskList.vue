<!-- web/src/components/chat/v2/TaskList.vue -->
<template>
  <div class="task-list">
    <TaskCard
      v-for="task in tasks"
      :key="task.ID"
      :task="task"
      :hydrated="queries.isTaskHydrated(task.ID)"
      :hydration-state="queries.taskHydrationState(task.ID)"
      :collapsed="lazy.isCollapsed(task.ID)"
      @regenerate="(t) => $emit('regenerate', t)"
      @resume-task="(t) => $emit('resume-task', t)"
      @pause-agent="(sid) => $emit('pause-agent', sid)"
      @inject-agent="(p) => $emit('inject-agent', p)"
      @expand="(ids) => $emit('expand', ids)"
      @confirm-step="(p) => $emit('confirm-step', p)"
      @submit-clarification="(p) => $emit('submit-clarification', p)"
      @hydrate="(t) => lazy.expandTask(t.ID)"
      @toggle-collapse="(t) => lazy.toggleCollapse(t.ID)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, inject, nextTick, watch, type Ref } from 'vue';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import {
  useLazyTaskHydration,
  CHAT_SCROLL_EL_KEY,
} from '../../../features/chat/composables/useLazyTaskHydration';
import type { Task } from '../../../features/chat/v2Types';
import type { ConfirmStepPayload, SubmitClarificationPayload } from '../../../features/chat/types';
import TaskCard from './TaskCard.vue';

const props = defineProps<{ sessionId: string }>();
defineEmits<{
  regenerate: [task: Task];
  'resume-task': [task: Task];
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  expand: [sessionIds: string[]];
  'confirm-step': [payload: ConfirmStepPayload];
  'submit-clarification': [payload: SubmitClarificationPayload];
}>();
const queries = useActivityQueries();
const tasks = computed(() => queries.getSessionTasks(props.sessionId));

// 懒水合编排：折叠卡滚入视口 500ms / 点击 → hydrateTask。
// components 禁止直访 store（layer 检查），needsHydration/hydrate 走 useActivityQueries 门面。
const scrollEl = inject<Ref<HTMLElement | null>>(CHAT_SCROLL_EL_KEY, computed(() => null) as never);
const lazy = useLazyTaskHydration({
  scrollEl,
  needsHydration: (taskId) =>
    !queries.isTaskHydrated(taskId) && queries.taskHydrationState(taskId) !== 'loading',
  hydrate: (taskId) => queries.hydrateTask(taskId),
});

// tasks 渲染 / 水合状态变化后同步观察集合（flush: 'post' 保证 DOM 已更新）。
watch(
  () => tasks.value.map((t) => `${t.ID}:${queries.isTaskHydrated(t.ID)}`).join('|'),
  async () => {
    await nextTick();
    lazy.syncCards();
  },
  { immediate: true, flush: 'post' },
);
</script>
