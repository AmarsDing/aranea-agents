<!-- web/src/components/chat/v2/SessionPanel.vue -->
<template>
  <div class="session-panel">
    <TaskList :session-id="sessionId" />
  </div>
</template>

<script setup lang="ts">
import TaskList from './TaskList.vue';

defineProps<{ sessionId: string }>();
</script>
