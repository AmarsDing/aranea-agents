<template>
  <WorkflowKanbanBoard :columns="columns" :is-dark="isDark" empty-label="暂无任务">
    <template #header>
      <div class="row items-center q-gutter-sm q-mb-md">
        <div class="text-subtitle2">任务看板</div>
        <q-badge v-if="liveConnected" rounded class="graph-kanban-live">实时</q-badge>
        <q-space />
        <q-btn flat dense round icon="refresh" :loading="loading" @click="$emit('refresh')">
          <q-tooltip>刷新</q-tooltip>
        </q-btn>
      </div>
      <div v-if="!loading && tasks.length === 0" class="graph-kanban-empty q-mb-md">
        <div class="text-body2">{{ emptyHint.title }}</div>
        <div class="text-caption text-grey-7 q-mt-xs">{{ emptyHint.detail }}</div>
      </div>
      <q-spinner v-if="loading" color="primary" size="28px" class="q-mb-md" />
    </template>
    <template #column-body="{ column }">
      <draggable
        :list="columnItems(column.key)"
        item-key="taskId"
        group="graph-tasks"
        :disabled="!adminDrag"
        class="graph-kanban-draggable"
        ghost-class="graph-kanban-ghost"
        @change="(evt: DragChangeEvent) => onDragChange(column.key, evt)"
      >
        <template #item="{ element }">
          <GraphTaskKanbanCard
            :ref="(el) => setCardRef(element.taskId, el)"
            :task="element"
            :is-dark="isDark"
            :selected="element.taskId === selectedTaskId"
            class="q-mb-sm"
            @select="$emit('selectTask', element.taskId)"
          />
        </template>
      </draggable>
      <div v-if="columnItems(column.key).length === 0" class="workflow-kanban-board__empty">暂无任务</div>
    </template>
  </WorkflowKanbanBoard>
</template>

<script setup lang="ts">
import { computed, nextTick, reactive, ref, watch, type ComponentPublicInstance } from 'vue';
import draggable from 'vuedraggable';
import type { Task } from '../../features/graph/types';
import {
  GRAPH_TASK_KANBAN_COLUMNS,
  GRAPH_TASK_KANBAN_EMPTY_HINT,
  kanbanAdminActionForDrop,
} from '../../features/graph/tasks/kanbanColumns';
import WorkflowKanbanBoard from '../workflow/WorkflowKanbanBoard.vue';
import GraphTaskKanbanCard from './GraphTaskKanbanCard.vue';

const props = defineProps<{
  tasks: Task[];
  loading: boolean;
  liveConnected: boolean;
  selectedTaskId?: string | null;
  isDark: boolean;
  adminDrag?: boolean;
}>();

const emit = defineEmits<{
  refresh: [];
  selectTask: [taskId: string];
  adminAction: [payload: { taskId: string; action: 'unblock' | 'approve' }];
}>();

const emptyHint = GRAPH_TASK_KANBAN_EMPTY_HINT;
const COLUMN_DEFS = GRAPH_TASK_KANBAN_COLUMNS;

const columnLists = reactive<Record<string, Task[]>>({
  pending: [],
  active: [],
  review: [],
  done: [],
  issue: [],
});

function syncColumns() {
  for (const column of COLUMN_DEFS) {
    columnLists[column.key] = props.tasks.filter((task) => column.statuses.includes(task.status));
  }
}

watch(() => props.tasks, syncColumns, { immediate: true, deep: true });

const columns = computed(() =>
  COLUMN_DEFS.map((column) => ({
    key: column.key,
    label: column.label,
    items: columnLists[column.key] ?? [],
  })),
);

function columnItems(key: string) {
  return columnLists[key] ?? [];
}

const cardRefs = ref(new Map<string, HTMLElement>());

function setCardRef(taskId: string, el: Element | ComponentPublicInstance | null) {
  const root = (el as ComponentPublicInstance | null)?.$el ?? el;
  if (root instanceof HTMLElement) {
    cardRefs.value.set(taskId, root);
  } else {
    cardRefs.value.delete(taskId);
  }
}

watch(
  () => props.selectedTaskId,
  (taskId) => {
    if (!taskId) return;
    nextTick(() => {
      cardRefs.value.get(taskId)?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    });
  },
);

type DragChangeEvent = {
  added?: { element: Task; newIndex: number };
  removed?: { element: Task; oldIndex: number };
};

function onDragChange(targetColumnKey: string, evt: DragChangeEvent) {
  if (!props.adminDrag || !evt.added?.element) return;
  const task = evt.added.element;
  syncColumns();
  const action = kanbanAdminActionForDrop(targetColumnKey, task.status);
  if (action) {
    emit('adminAction', { taskId: task.taskId, action });
  }
}
</script>
