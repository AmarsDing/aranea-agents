<template>
  <div :class="['workflow-kanban-board', { 'is-dark': isDark }]">
    <div v-if="$slots.header" class="workflow-kanban-board__header">
      <slot name="header" />
    </div>
    <div class="workflow-kanban-board__columns">
      <section
        v-for="column in columns"
        :key="column.key"
        class="workflow-kanban-board__column"
      >
        <header class="workflow-kanban-board__column-head">
          <span class="workflow-kanban-board__column-title">{{ column.label }}</span>
          <span class="workflow-kanban-board__column-count">{{ column.items.length }}</span>
        </header>
        <div class="workflow-kanban-board__column-body">
          <slot v-if="$slots['column-body']" name="column-body" :column="column" :items="column.items" />
          <template v-else>
            <div v-if="column.items.length === 0" class="workflow-kanban-board__empty">
              {{ emptyLabel }}
            </div>
            <draggable
              v-else
              :model-value="column.items"
              item-key="key"
              class="workflow-kanban-board__card-list"
              ghost-class="workflow-kanban-card--ghost"
              chosen-class="workflow-kanban-card--chosen"
              drag-class="workflow-kanban-card--dragging"
              :group="groupName"
              :animation="200"
              @update:model-value="(items: unknown[]) => onReorder(column.key, items)"
            >
              <template #item="{ element }">
                <slot name="card" :column="column" :item="element" />
              </template>
            </draggable>
          </template>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import draggable from "vuedraggable";

export type WorkflowKanbanColumn<T> = {
  key: string;
  label: string;
  items: T[];
};

withDefaults(
  defineProps<{
    columns: WorkflowKanbanColumn<unknown>[];
    isDark: boolean;
    emptyLabel?: string;
    groupName?: string;
  }>(),
  {
    emptyLabel: undefined,
    groupName: "workflow-kanban",
  },
);

const emit = defineEmits<{
  reorder: [payload: { columnKey: string; items: unknown[] }];
}>();

function onReorder(columnKey: string, items: unknown[]) {
  emit("reorder", { columnKey, items });
}
</script>
