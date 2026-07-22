<!-- web/src/components/chat/v2/SessionPanel.vue -->
<template>
  <div class="session-panel">
    <TaskList
      :session-id="sessionId"
      @regenerate="(t) => $emit('regenerate', t)"
      @resume-task="(t) => $emit('resume-task', t)"
      @pause-agent="(sid) => $emit('pause-agent', sid)"
      @inject-agent="(p) => $emit('inject-agent', p)"
      @retry-team="(teamId) => $emit('retry-team', teamId)"
      @expand="(ids) => $emit('expand', ids)"
      @confirm-step="(p) => $emit('confirm-step', p)"
    />
  </div>
</template>

<script setup lang="ts">
import TaskList from './TaskList.vue';
import type { Task } from '../../../features/chat/v2Types';
import type { ConfirmStepPayload } from '../../../features/chat/types';

defineProps<{ sessionId: string }>();
defineEmits<{
  regenerate: [task: Task];
  'resume-task': [task: Task];
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  'retry-team': [teamId: string];
  expand: [sessionIds: string[]];
  'confirm-step': [payload: ConfirmStepPayload];
}>();
</script>
