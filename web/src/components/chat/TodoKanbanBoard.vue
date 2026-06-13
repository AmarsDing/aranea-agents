<template>
  <div v-if="boardState && boardState.todos.length > 0" class="todo-kanban">
    <div class="todo-kanban__header" @click="expanded = !expanded">
      <template v-if="!expanded">
        <span class="todo-kanban__summary">
          📋 {{ pendingCount }} {{ t('chat.todo.pending') }} · {{ inProgressCount }} {{ t('chat.todo.inProgress') }} · {{ completedCount }} {{ t('chat.todo.completed') }}
        </span>
      </template>
      <template v-else>
        <span class="todo-kanban__title">📋 {{ t('chat.todo.summary') }}</span>
      </template>
      <span class="todo-kanban__toggle">{{ expanded ? '▼' : '▶' }}</span>
    </div>

    <div v-if="expanded" class="todo-kanban__columns">
      <TodoColumn
        :title="t('chat.todo.pending')"
        :items="pendingItems"
        column-key="pending"
        color="var(--color-text-tertiary)"
      />
      <TodoColumn
        :title="t('chat.todo.inProgress')"
        :items="inProgressItems"
        column-key="in_progress"
        color="var(--color-primary)"
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

const pendingItems = computed<TodoItem[]>(() =>
  props.boardState?.todos.filter((t) => t.status === 'pending') ?? [],
);

const inProgressItems = computed<TodoItem[]>(() =>
  props.boardState?.todos.filter((t) => t.status === 'in_progress') ?? [],
);

const completedItems = computed<TodoItem[]>(() =>
  props.boardState?.todos.filter((t) => t.status === 'completed') ?? [],
);

const pendingCount = computed(() => pendingItems.value.length);
const inProgressCount = computed(() => inProgressItems.value.length);
const completedCount = computed(() => completedItems.value.length);
</script>

<style scoped lang="sass">
.todo-kanban
  border: 1px solid var(--glass-border)
  border-radius: 8px
  background: color-mix(in srgb, var(--color-primary) 3%, var(--glass-surface))
  margin-bottom: 12px
  overflow: hidden

.todo-kanban__header
  display: flex
  align-items: center
  padding: 8px 12px
  cursor: pointer
  user-select: none
  transition: background 0.12s

  &:hover
    background: color-mix(in srgb, var(--color-text-primary) 4%, transparent)

.todo-kanban__summary
  font-size: 12px
  color: var(--color-text-secondary)
  flex: 1

.todo-kanban__title
  font-size: 12px
  font-weight: 600
  color: var(--color-text-secondary)
  flex: 1

.todo-kanban__toggle
  color: var(--color-text-tertiary)
  font-size: 10px
  margin-left: 8px

.todo-kanban__columns
  display: flex
  gap: 8px
  padding: 6px 8px 8px
</style>
