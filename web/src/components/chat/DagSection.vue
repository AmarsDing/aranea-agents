<template>
  <div class="dag-section">
    <div class="dag-section__header">
      <span class="dag-section__icon">🔗</span>
      <span class="dag-section__title">依赖关系</span>
    </div>
    <div class="dag-section__graph">
      <div
        v-for="node in section.nodes"
        :key="node.id"
        class="dag-node"
        :class="`dag-node--${node.status}`"
      >
        <span class="dag-node__dot"></span>
        <span class="dag-node__label">{{ node.label }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { DagSection as DagSectionType } from '../../features/chat/activityTimelineTypes';

defineProps<{
  section: DagSectionType;
}>();
</script>

<style lang="sass" scoped>
.dag-section
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

  &__graph
    display: flex
    flex-wrap: wrap
    gap: 8px

.dag-node
  display: flex
  align-items: center
  gap: 6px
  padding: 4px 10px
  border-radius: 12px
  font-size: 12px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)

  &--done
    border-color: var(--color-success)
    .dag-node__dot
      background: var(--color-success)

  &--running
    border-color: var(--color-accent)
    .dag-node__dot
      background: var(--color-accent)
      animation: pulse 1.5s infinite

  &--pending
    .dag-node__dot
      background: var(--color-text-secondary)

  &--failed
    border-color: var(--color-danger)
    .dag-node__dot
      background: var(--color-danger)

  &__dot
    width: 8px
    height: 8px
    border-radius: 50%
    flex-shrink: 0

  &__label
    color: var(--color-text-primary)

@keyframes pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.4
</style>
