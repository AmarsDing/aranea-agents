<template>
  <div class="plan-block" :class="`plan-block--${activity.status}`">
    <!-- Header (aligned with m59 v7 panel-section-header) -->
    <div class="plan-block__header" @click="toggleCollapse">
      <span class="plan-block__icon">📋</span>
      <span class="plan-block__label">{{ t('chat.plan.label') }}</span>
      <span v-if="activity.steps?.length" class="plan-block__count">{{ activity.steps.length }}</span>
      <span class="plan-block__status" :class="statusClass">{{ statusIcon }}</span>
      <span
        v-if="activity.steps?.length && !isRunning"
        class="plan-block__chevron"
        :class="{ 'plan-block__chevron--expanded': !collapsed }"
      >
        ▾
      </span>
    </div>

    <div v-if="activity.title && !isRunning" class="plan-block__subtitle">
      {{ activity.title }}
    </div>

    <!-- Steps list (aligned with m59 v7 task-row design) -->
    <div v-if="showSteps" class="plan-block__steps">
      <div
        v-for="(step, idx) in activity.steps"
        :key="step.id"
        class="plan-block__step"
        :class="`plan-block__step--${step.status}`"
      >
        <div class="plan-block__step-row">
          <span class="plan-block__step-num">{{ idx + 1 }}</span>
          <span class="plan-block__step-name">{{ step.label }}</span>
          <span v-if="step.agentName" class="plan-block__step-team">{{ step.agentName }}</span>
          <span class="plan-block__step-status" :class="`plan-block__step-status--${step.status}`">
            <span v-if="step.status === 'running'" class="plan-block__pulse" />
            {{ stepStatusText(step.status) }}
          </span>
          <span
            v-if="step.durationMs != null && (step.status === 'completed' || step.status === 'partial_failure')"
            class="plan-block__step-duration"
          >
            {{ formatDuration(step.durationMs) }}
          </span>
        </div>
        <div v-if="step.dependsOn?.length" class="plan-block__step-dep">
          {{ t('chat.plan.dependsOn') }} {{ step.dependsOn.join(', ') }}
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
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { PlanEvent, PlanStep } from '../../features/chat/streamEventTypes';
import { formatDuration } from '../../features/chat/agentTreeUtils';

const props = defineProps<{
  activity: PlanEvent;
}>();

const { t } = useI18n();

const isRunning = computed(() => props.activity.status === 'planning' || props.activity.status === 'executing');

const collapsed = ref(!isRunning.value);

function toggleCollapse() {
  if (isRunning.value) return;
  collapsed.value = !collapsed.value;
}

const showSteps = computed(() => {
  if (!props.activity.steps?.length) return false;
  if (isRunning.value) return true;
  return !collapsed.value;
});

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

function stepStatusText(status: PlanStep['status']): string {
  switch (status) {
    case 'pending':
      return t('chat.plan.stepPending');
    case 'running':
      return t('chat.plan.stepRunning');
    case 'completed':
      return t('chat.plan.stepDone');
    case 'failed':
      return t('chat.plan.stepFailed');
    case 'partial_failure':
      return t('chat.plan.stepPartial');
    default:
      return '';
  }
}
</script>

<style lang="sass" scoped>
.plan-block
  padding: 8px 10px
  border-radius: 8px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)

  &--planning, &--executing
    border-color: color-mix(in srgb, var(--color-accent) 40%, var(--glass-border))
  &--failed
    border-color: color-mix(in srgb, var(--color-danger) 40%, var(--glass-border))

  &__header
    display: flex
    align-items: center
    gap: 6px
    cursor: default
    padding-bottom: 8px

  &__icon
    font-size: 13px
    flex-shrink: 0

  &__label
    font-size: 12px
    font-weight: 500
    color: var(--color-text-secondary)
    flex: 1

  &__count
    background: color-mix(in srgb, var(--color-accent) 12%, transparent)
    padding: 0 5px
    border-radius: 8px
    font-size: 10px
    color: var(--color-accent)

  &__status
    font-size: 12px
    flex-shrink: 0
    &--planning, &--executing
      color: var(--color-accent)
    &--completed
      color: var(--color-success)
    &--failed
      color: var(--color-danger)

  &__chevron
    font-size: 10px
    color: var(--color-text-tertiary)
    transition: transform 0.15s ease
    &--expanded
      transform: rotate(180deg)

  &__subtitle
    font-size: 11px
    color: var(--color-text-tertiary)
    margin-top: -4px
    margin-bottom: 6px
    padding-left: 22px

  &__steps
    display: flex
    flex-direction: column
    gap: 3px

  &__step
    padding: 4px 0

  &__step-row
    display: flex
    align-items: center
    gap: 8px
    padding: 6px 10px
    background: var(--color-bg, var(--glass-surface))
    border: 1px solid var(--glass-border, var(--color-border))
    border-radius: 6px
    font-size: 12px

  &__step-num
    width: 18px
    height: 18px
    border-radius: 50%
    background: color-mix(in srgb, var(--color-accent) 12%, transparent)
    color: var(--color-accent)
    display: flex
    align-items: center
    justify-content: center
    font-size: 9px
    font-weight: 600
    flex-shrink: 0

  &__step-name
    flex: 1
    color: var(--color-text-primary)
    font-size: 12px

  &__step-team
    font-size: 10px
    color: var(--color-text-tertiary)
    background: var(--glass-elevated, var(--glass-surface))
    padding: 1px 6px
    border-radius: 3px
    flex-shrink: 0

  &__step-status
    font-size: 10px
    color: var(--color-text-tertiary)
    display: inline-flex
    align-items: center
    gap: 3px
    flex-shrink: 0

    &--pending
      color: var(--color-text-tertiary)
    &--running
      color: var(--color-accent)
    &--completed
      color: var(--color-success)
    &--failed
      color: var(--color-danger)
    &--partial_failure
      color: var(--color-warning)

  &__step-duration
    font-size: 10px
    color: var(--color-text-tertiary)
    flex-shrink: 0
    font-variant-numeric: tabular-nums

  &__step-dep
    font-size: 10px
    color: var(--color-text-tertiary)
    margin-left: 36px
    margin-top: 2px

  &__pulse
    width: 5px
    height: 5px
    border-radius: 50%
    background: var(--color-accent)
    animation: plan-pulse 1.5s ease-in-out infinite

@keyframes plan-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3
</style>
