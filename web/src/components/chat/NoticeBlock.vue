<template>
  <div class="notice-activity" :class="`notice-activity--${event.type}`">
    <span class="notice-activity__icon">{{ event.type === 'degradation' ? '⚠️' : 'ℹ️' }}</span>
    <span class="notice-activity__message">{{ event.message }}</span>
  </div>
</template>

<script setup lang="ts">
import type { ErrorEvent } from '../../features/chat/streamEventTypes';

defineProps<{
  event: ErrorEvent;
}>();
</script>

<style lang="sass" scoped>
.notice-activity
  display: flex
  align-items: center
  gap: 6px
  padding: 6px 10px
  border-radius: 8px
  font-size: 13px

  &--degradation
    background: var(--chat-status-warning-bg)
    border: 1px solid var(--chat-status-warning-border)
    color: var(--color-warning)

  &--info
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)
    color: var(--color-text-secondary)

  &__icon
    font-size: 13px
    flex-shrink: 0

  &__message
    line-height: 1.4
</style>
