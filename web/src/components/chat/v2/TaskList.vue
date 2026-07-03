<!-- web/src/components/chat/v2/TaskList.vue -->
<template>
  <div class="task-list">
    <TaskCard v-for="task in tasks" :key="task.ID" :task="task" @regenerate="(t) => $emit('regenerate', t)" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Task } from '../../../features/chat/v2Types';
import TaskCard from './TaskCard.vue';

const props = defineProps<{ sessionId: string }>();
defineEmits<{
  regenerate: [task: Task];
}>();
const store = useChatActivityStore();
const tasks = computed(() => store.getSessionTasks(props.sessionId));
</script>
