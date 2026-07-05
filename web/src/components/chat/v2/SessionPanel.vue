<!-- web/src/components/chat/v2/SessionPanel.vue -->
<template>
  <div class="session-panel">
    <TaskList
      :session-id="sessionId"
      @regenerate="(t) => $emit('regenerate', t)"
      @pause-agent="(sid) => $emit('pause-agent', sid)"
      @inject-agent="(p) => $emit('inject-agent', p)"
    />
  </div>
</template>

<script setup lang="ts">
import TaskList from './TaskList.vue';
import type { Task } from '../../../features/chat/v2Types';

defineProps<{ sessionId: string }>();
defineEmits<{
  regenerate: [task: Task];
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
}>();
</script>
