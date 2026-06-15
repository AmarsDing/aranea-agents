<template>
  <div class="error-block" :class="`error-block--${event.type}`">
    <span class="error-block__icon">⚠️</span>
    <span class="error-block__message">{{ event.message }}</span>
    <span v-if="event.type" class="error-block__code">{{ event.type }}</span>
  </div>
</template>

<script setup lang="ts">
import type { ErrorEvent } from '../../features/chat/streamEventTypes';

defineProps<{
  event: ErrorEvent;
}>();
</script>

<style lang="sass" scoped>
.error-block
  display: flex
  align-items: center
  gap: 6px
  padding: 6px 10px
  border-radius: 8px
  border-left: 3px solid var(--color-danger)
  font-size: 13px

  &--degradation
    background: var(--chat-status-danger-bg)
    color: var(--color-danger)

  &--info
    background: var(--glass-surface)
    border-left-color: var(--color-text-tertiary)
    color: var(--color-text-secondary)

  &__icon
    font-size: 13px
    flex-shrink: 0

  &__message
    line-height: 1.4
    flex: 1

  &__code
    font-size: 11px
    color: var(--color-text-tertiary)
    flex-shrink: 0
</style>
