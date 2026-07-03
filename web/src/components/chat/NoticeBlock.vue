<template>
  <div class="notice-block" :class="`notice-block--${noticeType}`">
    <span class="notice-block__icon">{{ iconForType }}</span>
    <span class="notice-block__message">{{ step.Content }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { Step } from '../../features/chat/v2Types';

const props = defineProps<{
  step: Step;
}>();

// v2 Step has no notice severity field; default to 'info'.
const noticeType = computed<'info' | 'warning' | 'success'>(() => 'info');

const iconForType = computed(() => {
  switch (noticeType.value) {
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
