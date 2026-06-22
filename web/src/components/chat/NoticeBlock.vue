<template>
  <div class="notice-block" :class="`notice-block--${activity.type}`">
    <span class="notice-block__icon">{{ iconForType }}</span>
    <span class="notice-block__message">{{ activity.message }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { NoticeEvent } from '../../features/chat/streamEventTypes';

const props = defineProps<{
  activity: NoticeEvent;
}>();

const iconForType = computed(() => {
  switch (props.activity.type) {
    case 'warning':
      return '⚠️';
    case 'success':
      return '✅';
    default:
      return 'ℹ️';
  }
});
</script>

<style lang="sass" scoped>
.notice-block
  display: flex
  align-items: center
  gap: 6px
  padding: 6px 10px
  border-radius: 8px
  font-size: 13px

  &--warning
    background: var(--chat-status-warning-bg)
    border: 1px solid var(--chat-status-warning-border)
    color: var(--color-warning)

  &--success
    background: color-mix(in srgb, var(--color-success) 8%, transparent)
    border: 1px solid color-mix(in srgb, var(--color-success) 30%, transparent)
    color: var(--color-success)

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
