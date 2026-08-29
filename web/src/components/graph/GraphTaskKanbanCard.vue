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
        <div class="text-caption app-text-secondary">{{ task.requiredRole || t('graphs.taskWorkerFallback') }}</div>
      </div>
      <AppStatusChip :status="task.status" />
    </q-card-section>
    <q-separator />
    <q-card-section class="q-gutter-xs">
      <div v-if="task.assignee" class="text-caption">{{ t('graphs.taskAssigneeLabel', { name: task.assignee }) }}</div>
      <div v-if="task.summary" class="text-body2">{{ task.summary }}</div>
      <div v-else-if="task.input" class="text-caption app-text-secondary">{{ truncate(task.input) }}</div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppStatusChip from '../common/AppStatusChip.vue';
import type { Task } from '../../features/graph/types';
import { truncate } from '../../features/graph/utils';

const { t } = useI18n();

defineProps<{
  task: Task;
  isDark: boolean;
  selected?: boolean;
}>();

defineEmits<{ select: [] }>();
</script>
