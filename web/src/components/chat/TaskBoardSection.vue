<template>
  <div class="task-board-section">
    <div class="task-board-section__header">
      <span class="task-board-section__icon">📋</span>
      <span class="task-board-section__title">任务拆解</span>
    </div>
    <div class="task-board-section__entries">
      <div
        v-for="entry in section.entries"
        :key="entry.id"
        class="task-board-entry"
        :class="`task-board-entry--${entry.status}`"
      >
        <span class="task-board-entry__num">{{ entry.num }}</span>
        <span class="task-board-entry__task">{{ entry.task }}</span>
        <span v-if="entry.agentName" class="task-board-entry__agent" :style="{ color: entry.agentColor || 'var(--color-text-secondary)' }">
          {{ entry.agentIcon || entry.agentName?.charAt(0) || '' }} {{ entry.agentName }}
        </span>
        <span class="task-board-entry__status-icon">{{ statusIcon(entry.status) }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { TaskBoardSection as TaskBoardSectionType } from '../../features/chat/activityTimelineTypes';

defineProps<{
  section: TaskBoardSectionType;
}>();

function statusIcon(status: string): string {
  switch (status) {
    case 'running': return '⏳';
    case 'completed': return '✓';
    case 'failed': return '✗';
    default: return '○';
  }
}
</script>

<style lang="sass" scoped>
.task-board-section
  margin-bottom: 12px

  &__header
    display: flex
    align-items: center
    gap: 6px
    margin-bottom: 8px

  &__icon
    font-size: 16px

  &__title
    font-size: 14px
    font-weight: 600
    color: var(--color-text-primary)

  &__entries
    display: flex
    flex-direction: column
    gap: 4px

.task-board-entry
  display: flex
  align-items: center
  gap: 8px
  padding: 6px 10px
  border-radius: 8px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
  font-size: 13px

  &--running
    border-color: var(--color-accent)
    border-left: 3px solid var(--color-accent)

  &--completed
    border-left: 3px solid var(--color-success)

  &--failed
    border-left: 3px solid var(--color-danger)

  &__num
    width: 20px
    height: 20px
    border-radius: 50%
    background: var(--glass-surface)
    display: flex
    align-items: center
    justify-content: center
    font-size: 11px
    font-weight: 600
    color: var(--color-text-secondary)
    flex-shrink: 0

  &__task
    flex: 1
    color: var(--color-text-primary)

  &__agent
    font-size: 12px
    flex-shrink: 0

  &__status-icon
    font-size: 12px
    flex-shrink: 0
</style>
