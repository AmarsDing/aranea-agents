<template>
  <div class="task-board" :class="{ 'task-board--nested': depth > 0 }">
    <!-- Empty state -->
    <div v-if="entries.length === 0" class="task-board__empty">
      {{ t('chat.taskBoard.noEntries') }}
    </div>

    <!-- Entry list -->
    <div v-else class="task-board__list">
      <div
        v-for="(entry, idx) in entries"
        :key="entryKey(entry, idx)"
        class="task-board__item"
        :class="{ 'task-board__item--last': idx === entries.length - 1 }"
      >
        <!-- Vertical connecting line -->
        <div class="task-board__connector">
          <div class="task-board__connector-dot" />
          <div v-if="idx < entries.length - 1" class="task-board__connector-line" />
        </div>

        <!-- Node content -->
        <div class="task-board__node-wrapper">
          <!-- Max depth reached: render flat summary for sub_task_board -->
          <div
            v-if="entry.kind === 'sub_task_board' && depth >= MAX_DEPTH"
            class="task-board__depth-cap"
          >
            <q-icon name="account_tree" size="14px" style="color: var(--color-accent)" />
            <span>{{ t('chat.taskBoard.maxDepthReached') }}</span>
            <span class="task-board__depth-cap-count">
              {{ (entry.children ?? []).length }} {{ t('chat.taskBoard.subTasks') }}
            </span>
          </div>

          <!-- Normal rendering -->
          <TaskBoardNode v-else :node="entry" :depth="depth" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { TaskBoardNodeData } from '../../features/chat/agentTreeTypes';
import TaskBoardNode from './TaskBoardNode.vue';

const { t } = useI18n();

const MAX_DEPTH = 2;

withDefaults(
  defineProps<{
    entries: TaskBoardNodeData[];
    depth?: number;
  }>(),
  { depth: 0 },
);

function entryKey(entry: TaskBoardNodeData, idx: number): string {
  switch (entry.kind) {
    case 'task':
      return `task-${idx}`;
    case 'thinking':
      return `thinking-${idx}`;
    case 'action':
      return `action-${idx}`;
    case 'reply':
      return `reply-${idx}`;
    case 'sub_task_board':
      return `sub_task_board-${idx}`;
    case 'end':
      return `end-${idx}`;
    case 'error':
      return `error-${idx}`;
  }
}
</script>

<style scoped lang="sass">
.task-board
  position: relative

.task-board--nested
  margin-left: 4px

.task-board__empty
  color: var(--color-text-tertiary)
  font-size: 12px
  font-style: italic
  padding: 12px 8px
  text-align: center

.task-board__list
  display: flex
  flex-direction: column

.task-board__item
  display: flex
  gap: 0
  position: relative

.task-board__connector
  display: flex
  flex-direction: column
  align-items: center
  width: 16px
  flex-shrink: 0
  position: relative

.task-board__connector-dot
  width: 8px
  height: 8px
  border-radius: 50%
  background: var(--glass-border)
  margin-top: 10px
  flex-shrink: 0
  z-index: 1

.task-board__connector-line
  width: 2px
  flex: 1
  background: var(--glass-border)
  min-height: 4px

.task-board__item--last
  .task-board__connector-line
    display: none

.task-board__node-wrapper
  flex: 1
  min-width: 0
  padding-bottom: 4px

.task-board__depth-cap
  display: flex
  align-items: center
  gap: 6px
  padding: 6px 10px
  border-left: 3px solid var(--color-accent)
  border-radius: 0 6px 6px 0
  background: color-mix(in srgb, var(--color-accent) 6%, transparent)
  font-size: 12px
  color: var(--color-text-secondary)
  margin-bottom: 8px

.task-board__depth-cap-count
  font-size: 10px
  color: var(--color-text-tertiary)
</style>
