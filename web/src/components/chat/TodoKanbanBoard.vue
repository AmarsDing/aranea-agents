<template>
  <div v-if="boardState && boardState.todos.length > 0" class="todo-kanban">
    <div class="todo-kanban__header" @click="expanded = !expanded">
      <template v-if="!expanded">
        <span class="todo-kanban__summary">
          <q-icon name="checklist" size="14px" class="todo-kanban__summary-icon" />
          {{ pendingCount }} {{ t('chat.todo.pending') }} · {{ inProgressCount }} {{ t('chat.todo.inProgress') }} ·
          {{ completedCount }} {{ t('chat.todo.completed') }}
        </span>
      </template>
      <template v-else>
        <span class="todo-kanban__title">
          <q-icon name="checklist" size="14px" class="todo-kanban__title-icon" />
          {{ t('chat.todo.summary') }}
        </span>
      </template>
      <q-icon
        name="expand_more"
        size="16px"
        class="todo-kanban__toggle"
        :class="{ 'todo-kanban__toggle--expanded': expanded }"
      />
    </div>

    <div v-if="expanded" class="todo-kanban__columns">
      <TodoColumn
        :title="t('chat.todo.pending')"
        :items="pendingItems"
        column-key="pending"
        color="var(--color-text-secondary)"
      />
      <TodoColumn
        :title="t('chat.todo.inProgress')"
        :items="inProgressItems"
        column-key="in_progress"
        color="var(--color-accent)"
      />
      <TodoColumn
        :title="t('chat.todo.completed')"
        :items="completedItems"
        column-key="completed"
        color="var(--color-success)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { TodoBoardState, TodoItem } from '../../features/chat/agentTreeTypes';
import TodoColumn from './TodoColumn.vue';

const props = defineProps<{
  boardState: TodoBoardState | null;
}>();

const { t } = useI18n();
const expanded = ref(false);

const pendingItems = computed<TodoItem[]>(() => props.boardState?.todos.filter((t) => t.status === 'pending') ?? []);

const inProgressItems = computed<TodoItem[]>(
  () => props.boardState?.todos.filter((t) => t.status === 'in_progress') ?? [],
);

const completedItems = computed<TodoItem[]>(
  () => props.boardState?.todos.filter((t) => t.status === 'completed') ?? [],
);

const pendingCount = computed(() => pendingItems.value.length);
const inProgressCount = computed(() => inProgressItems.value.length);
const completedCount = computed(() => completedItems.value.length);
</script>

<style scoped lang="sass">
.todo-kanban
  border: 1px solid var(--glass-border)
  border-radius: 12px
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent)
  backdrop-filter: blur(var(--glass-blur-default))
  margin-bottom: 12px
  overflow: hidden

.todo-kanban__header
  display: flex
  align-items: center
  padding: 10px 14px
  cursor: pointer
  user-select: none
  transition: background 0.15s ease

  &:hover
    background: color-mix(in srgb, var(--glass-surface-hover) 50%, transparent)

.todo-kanban__summary
  font-size: 12px
  color: var(--color-text-secondary)
  flex: 1
  display: flex
  align-items: center
  gap: 6px

.todo-kanban__summary-icon
  color: var(--color-text-secondary)

.todo-kanban__title
  font-size: 12px
  font-weight: 600
  color: var(--color-text-secondary)
  flex: 1
  display: flex
  align-items: center
  gap: 6px

.todo-kanban__title-icon
  color: var(--color-accent)

.todo-kanban__toggle
  color: var(--color-text-secondary)
  transition: transform 0.2s ease

.todo-kanban__toggle--expanded
  transform: rotate(180deg)

.todo-kanban__columns
  display: flex
  gap: 8px
  padding: 8px 10px 10px
</style>
