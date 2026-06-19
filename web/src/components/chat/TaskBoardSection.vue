<template>
  <div class="task-board-section">
    <div class="task-board-section__header">
      <q-icon name="account_tree" size="16px" color="accent" class="task-board-section__icon" />
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
          <agent-avatar-q :icon="entry.agentIcon || ''" size="16px" avatar-class="task-board-entry__agent-avatar" /> {{ entry.agentName }}
        </span>
        <q-icon
          :name="statusIconName(entry.status)"
          size="14px"
          :color="statusIconColor(entry.status)"
          class="task-board-entry__status-icon"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { TaskBoardSection as TaskBoardSectionType } from '../../features/chat/activityTimelineTypes';
import AgentAvatarQ from '../avatar/AgentAvatarQ.vue';

defineProps<{
  section: TaskBoardSectionType;
}>();

function statusIconName(status: string): string {
  switch (status) {
    case 'running': return 'hourglass_top';
    case 'completed': return 'check_circle';
    case 'failed': return 'cancel';
    default: return 'radio_button_unchecked';
  }
}

function statusIconColor(status: string): string {
  switch (status) {
    case 'running': return 'warning';
    case 'completed': return 'positive';
    case 'failed': return 'negative';
    default: return 'grey';
  }
}
</script>

<style lang="sass" scoped>
.task-board-section
  margin-bottom: 12px

  &__header
    display: flex
    align-items: center
    gap: 8px
    margin-bottom: 8px

  &__icon
    flex-shrink: 0

  &__title
    font-size: 13px
    font-weight: 600
    color: var(--color-text-primary)

  &__entries
    display: flex
    flex-direction: column
    gap: 3px

.task-board-entry
  display: flex
  align-items: center
  gap: 8px
  padding: 6px 10px
  border-radius: 8px
  background: color-mix(in srgb, var(--glass-surface) 40%, transparent)
  border: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)
  font-size: 13px
  transition: background 0.15s ease, border-color 0.15s ease

  &:hover
    background: color-mix(in srgb, var(--glass-surface-hover) 50%, transparent)

  &--running
    border-color: color-mix(in srgb, var(--color-accent) 25%, transparent)
    border-left: 3px solid var(--color-accent)
    background: color-mix(in srgb, var(--color-accent) 5%, transparent)

  &--completed
    border-left: 3px solid var(--color-success)
    opacity: 0.65

  &--failed
    border-left: 3px solid var(--color-danger)
    background: color-mix(in srgb, var(--color-danger) 4%, transparent)

  &__num
    width: 20px
    height: 20px
    border-radius: 50%
    background: color-mix(in srgb, var(--color-text-secondary) 10%, transparent)
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
    display: inline-flex
    align-items: center
    gap: 4px

  &__agent-avatar
    flex-shrink: 0

  &__status-icon
    flex-shrink: 0
</style>
