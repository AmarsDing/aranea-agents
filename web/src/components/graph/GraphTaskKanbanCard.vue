<template>
  <q-card
    flat
    bordered
    :class="['graph-task-card', { 'is-dark': isDark, 'graph-task-card--selected': selected }]"
    @click="$emit('select')"
  >
    <q-card-section class="row items-start justify-between no-wrap q-pb-sm">
      <q-icon name="drag_indicator" size="16px" class="graph-task-card__drag-handle q-mr-xs" />
      <div class="col min-width-0">
        <div class="text-weight-medium">{{ task.nodeId || task.taskId }}</div>
        <div class="text-caption app-text-secondary">{{ task.requiredRole || "worker" }}</div>
      </div>
      <q-badge rounded :color="statusColor">{{ statusLabel }}</q-badge>
    </q-card-section>
    <q-separator />
    <q-card-section class="q-gutter-xs">
      <div v-if="task.assignee" class="text-caption">执行者：{{ task.assignee }}</div>
      <div v-if="task.summary" class="text-body2">{{ task.summary }}</div>
      <div v-else-if="task.input" class="text-caption app-text-secondary">{{ truncate(task.input) }}</div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { Task } from "../../features/graph/types";
import { TASK_STATUS_COLORS, TASK_STATUS_LABELS } from "../../features/graph/types";

const props = defineProps<{
  task: Task;
  isDark: boolean;
  selected?: boolean;
}>();

defineEmits<{ select: [] }>();

const statusLabel = computed(() => TASK_STATUS_LABELS[props.task.status] ?? props.task.status);
const statusColor = computed(() => TASK_STATUS_COLORS[props.task.status] ?? "grey");

function truncate(text: string, max = 120) {
  return text.length > max ? `${text.slice(0, max)}…` : text;
}
</script>
