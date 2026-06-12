<template>
  <div class="notice-activity" :class="`notice-activity--${activity.type}`">
    <span class="notice-activity__icon">{{ activity.type === 'degradation' ? '⚠️' : 'ℹ️' }}</span>
    <span class="notice-activity__message">{{ activity.message }}</span>
  </div>
</template>

<script setup lang="ts">
import type { NoticeActivity as NoticeActivityType } from '../../features/chat/activityTimelineTypes';

defineProps<{
  activity: NoticeActivityType;
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
    background: rgba(240, 155, 84, 0.1)
    border: 1px solid rgba(240, 155, 84, 0.3)
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
