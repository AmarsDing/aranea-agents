<template>
  <div class="plan-block">
    <!-- Header -->
    <div class="plan-block__header">
      <span class="plan-block__icon">📋</span>
      <span class="plan-block__label">{{ t('chat.plan.label', '执行计划') }}</span>
      <span v-if="activity.title" class="plan-block__title">{{ activity.title }}</span>
      <span class="plan-block__status" :class="statusClass">{{ statusIcon }}</span>
    </div>

    <!-- Steps list -->
    <div v-if="activity.steps?.length" class="plan-block__steps">
      <div
        v-for="(step, idx) in activity.steps"
        :key="step.id"
        class="plan-block__step"
        :class="`plan-block__step--${step.status}`"
      >
        <div class="plan-block__step-row">
          <span class="plan-block__step-dot" :class="`plan-block__step-dot--${step.status}`">
            <span v-if="step.status === 'running'" class="plan-block__pulse" />
          </span>
          <span class="plan-block__step-index">{{ idx + 1 }}</span>
          <span class="plan-block__step-task">{{ step.label }}</span>
          <span v-if="step.agentName" class="plan-block__step-agent">{{ step.agentName }}</span>
          <span
            v-if="step.durationMs != null && (step.status === 'completed' || step.status === 'partial_failure')"
            class="plan-block__step-duration"
          >
            {{ formatDuration(step.durationMs) }}
          </span>
        </div>
        <div v-if="step.dependsOn?.length" class="plan-block__step-dep">
          {{ t('chat.plan.dependsOn', '等待步骤') }} {{ step.dependsOn.join(', ') }}
        </div>
      </div>
    </div>

    <!-- B-04 / Phase A: Child activity rendering moved to ActivityStream's
         recursive nested-rendering layer. PlanBlock is now a leaf block —
         it renders only its header + steps; the executed sub-activities
         (thinking/action/reply under this plan) are rendered by ActivityStream
         in a nested indented container via `<ActivityStream :activity-tree="..." />`. -->
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { PlanEvent } from '../../features/chat/streamEventTypes';
import { formatDuration } from '../../features/chat/agentTreeUtils';

const props = defineProps<{
  activity: PlanEvent;
}>();

const { t } = useI18n();

const statusClass = computed(() => ({
  'plan-block__status--planning': props.activity.status === 'planning',
  'plan-block__status--executing': props.activity.status === 'executing',
  'plan-block__status--completed': props.activity.status === 'completed',
  'plan-block__status--failed': props.activity.status === 'failed',
}));

const statusIcon = computed(() => {
  switch (props.activity.status) {
    case 'planning':
      return '📝';
    case 'executing':
      return '⏳';
    case 'completed':
      return '✓';
    case 'failed':
      return '✗';
    default:
      return '📋';
  }
});
</script>

<style lang="sass" scoped>
.plan-block
  padding: 8px 10px
  border-radius: 8px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)

  &__header
    display: flex
    align-items: center
    gap: 6px
    margin-bottom: 8px

  &__icon
    font-size: 14px
    flex-shrink: 0

  &__label
    font-size: 13px
    font-weight: 500
    color: var(--color-text-secondary)

  &__title
    font-size: 13px
    color: var(--color-text-primary)
    font-weight: 500

  &__status
    font-size: 12px
    margin-left: auto
    &--planning
      color: var(--color-accent)
    &--executing
      color: var(--color-accent)
    &--completed
      color: var(--color-success)
    &--failed
      color: var(--color-danger)

  &__steps
    display: flex
    flex-direction: column
    gap: 4px

  &__step
    padding: 4px 0 4px 4px

  &__step-row
    display: flex
    align-items: center
    gap: 6px

  &__step-dot
    width: 8px
    height: 8px
    border-radius: 50%
    flex-shrink: 0
    display: inline-flex
    align-items: center
    justify-content: center
    position: relative

    &--pending
      border: 1.5px solid var(--color-text-tertiary)
      background: transparent

    &--running
      background: var(--color-accent)

    &--completed
      background: var(--color-success)

    &--failed
      background: var(--color-danger)

    &--partial_failure
      background: var(--color-warning)

  &__pulse
    position: absolute
    inset: -3px
    border-radius: 50%
    border: 1.5px solid var(--color-accent)
    animation: plan-pulse 1.5s ease-in-out infinite

  &__step-index
    font-size: 11px
    color: var(--color-text-tertiary)
    min-width: 14px
    text-align: right
    flex-shrink: 0

  &__step-task
    font-size: 13px
    color: var(--color-text-primary)
    flex: 1

  &__step-agent
    font-size: 11px
    color: var(--color-text-tertiary)
    flex-shrink: 0

  &__step-duration
    font-size: 11px
    color: var(--color-text-secondary)
    flex-shrink: 0

  &__step-dep
    font-size: 11px
    color: var(--color-text-tertiary)
    margin-left: 28px
    margin-top: 2px

@keyframes plan-pulse
  0%, 100%
    opacity: 1
    transform: scale(1)
  50%
    opacity: 0.3
    transform: scale(1.4)
</style>
