<!-- web/src/components/chat/v2/TaskList.vue -->
<template>
  <div class="task-list">
    <TaskCard
      v-for="task in tasks"
      :key="task.ID"
      :task="task"
      @regenerate="(t) => $emit('regenerate', t)"
      @pause-agent="(sid) => $emit('pause-agent', sid)"
      @inject-agent="(p) => $emit('inject-agent', p)"
      @confirm-step="(p) => $emit('confirm-step', p)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import type { Task } from '../../../features/chat/v2Types';
import type { ConfirmStepPayload } from '../../../features/chat/types';
import TaskCard from './TaskCard.vue';

const props = defineProps<{ sessionId: string }>();
defineEmits<{
  regenerate: [task: Task];
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  'confirm-step': [payload: ConfirmStepPayload];
}>();
const store = useActivityQueries();
const tasks = computed(() => store.getSessionTasks(props.sessionId));
</script>
