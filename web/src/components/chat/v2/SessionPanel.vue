<!-- web/src/components/chat/v2/SessionPanel.vue -->
<template>
  <div class="session-panel">
    <TaskList
      :session-id="sessionId"
      @regenerate="(t) => $emit('regenerate', t)"
      @add-to-eval="(t) => $emit('add-to-eval', t)"
      @feedback="(p) => $emit('feedback', p)"
      @resume-task="(t) => $emit('resume-task', t)"
      @pause-agent="(sid) => $emit('pause-agent', sid)"
      @inject-agent="(p) => $emit('inject-agent', p)"
      @expand="(ids) => $emit('expand', ids)"
      @confirm-step="(p) => $emit('confirm-step', p)"
      @fork-turn="(t) => $emit('fork-turn', t)"
      @submit-clarification="(p) => $emit('submit-clarification', p)"
    />
  </div>
</template>

<script setup lang="ts">
import TaskList from './TaskList.vue';
import type { Task, Turn } from '../../../features/chat/v2Types';
import type { ConfirmStepPayload, SubmitClarificationPayload } from '../../../features/chat/types';

defineProps<{ sessionId: string }>();
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
  'submit-clarification': [payload: SubmitClarificationPayload];
}>();
</script>
