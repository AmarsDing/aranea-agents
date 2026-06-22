<template>
  <div v-if="boards.length > 0" class="todo-kanban-tabs">
    <!-- Tab strip (only when more than one agent has a board) -->
    <div v-if="boards.length > 1" class="todo-kanban-tabs__tabs" role="tablist">
      <button
        v-for="entry in boards"
        :key="entry.agentKey"
        type="button"
        role="tab"
        :aria-selected="entry.agentKey === activeKey"
        class="todo-kanban-tabs__tab"
        :class="{ 'todo-kanban-tabs__tab--active': entry.agentKey === activeKey }"
        @click="activeKey = entry.agentKey"
      >
        <q-icon
          :name="entry.agentKey === ROOT_AGENT_KEY ? 'auto_awesome' : 'person'"
          size="14px"
          class="todo-kanban-tabs__tab-icon"
        />
        <span class="todo-kanban-tabs__tab-label ellipsis">{{ entry.agentName }}</span>
        <span class="todo-kanban-tabs__tab-count">{{ entry.board.todos.length }}</span>
      </button>
    </div>
    <!-- Active board -->
    <TodoKanbanBoard v-if="activeBoard" :board-state="activeBoard" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { ROOT_AGENT_KEY, type TodoBoardState } from '../../features/chat/agentTreeTypes';
import type { TodoBoardEntry } from '../../features/chat/composables/useTodoBoard';
import TodoKanbanBoard from './TodoKanbanBoard.vue';

/**
 * TD-TK-5: Multi-agent Todo Board tabs.
 *
 * Renders a tab strip when more than one agent has emitted a
 * `todo_write` event, and a single (legacy) kanban board when there is
 * only one. The active tab defaults to the most-recent writer and
 * follows changes to the `boards` prop automatically.
 */
const props = defineProps<{
  boards: readonly TodoBoardEntry[];
}>();

const activeKey = ref<string | null>(props.boards[0]?.agentKey ?? null);

watch(
  () => props.boards,
  (next) => {
    // Re-anchor on prop change: keep the current tab if still present,
    // otherwise fall back to the most-recent writer.
    if (next.length === 0) {
      activeKey.value = null;
      return;
    }
    const stillThere = next.some((b) => b.agentKey === activeKey.value);
    if (!stillThere) {
      activeKey.value = next[0]!.agentKey;
    }
  },
);

const activeBoard = computed<TodoBoardState | null>(() => {
  if (!activeKey.value) return null;
  const entry = props.boards.find((b) => b.agentKey === activeKey.value);
  return entry?.board ?? null;
});
</script>

<style scoped lang="sass">
.todo-kanban-tabs
  margin-bottom: 12px

.todo-kanban-tabs__tabs
  display: flex
  align-items: center
  gap: 4px
  padding: 4px 6px
  margin-bottom: 6px
  border-radius: 10px
  background: color-mix(in srgb, var(--glass-surface) 40%, transparent)
  border: 1px solid color-mix(in srgb, var(--glass-border) 60%, transparent)
  overflow-x: auto

.todo-kanban-tabs__tab
  display: inline-flex
  align-items: center
  gap: 5px
  padding: 5px 12px
  border: 1px solid transparent
  border-radius: 7px
  background: transparent
  font-size: 12px
  color: var(--color-text-secondary)
  cursor: pointer
  user-select: none
  transition: background 0.15s ease, color 0.15s ease, border-color 0.15s ease
  white-space: nowrap
  flex-shrink: 0

  &:hover
    background: color-mix(in srgb, var(--glass-surface-hover) 50%, transparent)
    color: var(--color-text-primary)

.todo-kanban-tabs__tab--active
  background: color-mix(in srgb, var(--color-accent) 10%, transparent)
  border-color: color-mix(in srgb, var(--color-accent) 25%, transparent)
  color: var(--color-text-primary)

.todo-kanban-tabs__tab-icon
  flex-shrink: 0
  color: var(--color-text-secondary)

.todo-kanban-tabs__tab--active .todo-kanban-tabs__tab-icon
  color: var(--color-accent)

.todo-kanban-tabs__tab-label
  max-width: 120px
  font-weight: 500

.todo-kanban-tabs__tab-count
  font-size: 10px
  font-weight: 600
  color: var(--color-text-secondary)
  background: color-mix(in srgb, var(--color-text-secondary) 10%, transparent)
  border-radius: 8px
  padding: 1px 5px
  line-height: 15px
</style>
