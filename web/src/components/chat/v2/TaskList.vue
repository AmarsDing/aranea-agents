<!-- web/src/components/chat/v2/TaskList.vue -->
<template>
  <div class="task-list">
    <TaskCard
      v-for="task in tasks"
      :key="task.ID"
      :task="task"
      @regenerate="(t) => $emit('regenerate', t)"
      @resume-task="(t) => $emit('resume-task', t)"
      @pause-agent="(sid) => $emit('pause-agent', sid)"
      @inject-agent="(p) => $emit('inject-agent', p)"
      @retry-team="(teamId) => $emit('retry-team', teamId)"
      @expand="(ids) => $emit('expand', ids)"
      @confirm-step="(p) => $emit('confirm-step', p)"
      @submit-clarification="(p) => $emit('submit-clarification', p)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import type { Task } from '../../../features/chat/v2Types';
import type { ConfirmStepPayload, SubmitClarificationPayload } from '../../../features/chat/types';
import TaskCard from './TaskCard.vue';

const props = defineProps<{ sessionId: string }>();
defineEmits<{
  regenerate: [task: Task];
  'resume-task': [task: Task];
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  'retry-team': [teamId: string];
  expand: [sessionIds: string[]];
  'confirm-step': [payload: ConfirmStepPayload];
  'submit-clarification': [payload: SubmitClarificationPayload];
}>();
const store = useActivityQueries();
const tasks = computed(() => store.getSessionTasks(props.sessionId));
</script>
