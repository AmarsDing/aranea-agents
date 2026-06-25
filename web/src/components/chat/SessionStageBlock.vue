<template>
  <div class="session-stage-block" :class="`session-stage-block--${activity.status}`">
    <!-- Header -->
    <div class="session-stage-block__header">
      <span class="session-stage-block__icon">🗂</span>
      <span class="session-stage-block__label">{{ t('chat.sessionStage.label') }}</span>
      <span class="session-stage-block__title">{{ displayTitle }}</span>
      <span v-if="activity.agentName" class="session-stage-block__agent">{{ activity.agentName }}</span>
      <span class="session-stage-block__status" :class="`session-stage-block__status--${activity.status}`">
        {{ statusIcon }}
      </span>
    </div>

    <!-- Duration -->
    <div v-if="activity.durationMs != null && isTerminal" class="session-stage-block__duration">
      {{ formatDuration(activity.durationMs) }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SessionStageEvent } from '../../features/chat/streamEventTypes';
import { formatDuration } from '../../features/chat/agentTreeUtils';

const props = defineProps<{
  activity: SessionStageEvent;
}>();

const { t } = useI18n();

const isTerminal = computed(
  () =>
    props.activity.status === 'completed' ||
    props.activity.status === 'failed' ||
    props.activity.status === 'cancelled',
);

const displayTitle = computed(() => {
  if (props.activity.title) return props.activity.title;
  // Derive from agent name + status
  const who = props.activity.agentName || t('chat.sessionStage.member');
  switch (props.activity.status) {
    case 'running':
      return t('chat.sessionStage.executing', { name: who });
    case 'completed':
      return t('chat.sessionStage.completed', { name: who });
    case 'failed':
      return t('chat.sessionStage.failed', { name: who });
    case 'cancelled':
      return t('chat.sessionStage.cancelled', { name: who });
    default:
      return '';
  }
});

const statusIcon = computed(() => {
  switch (props.activity.status) {
    case 'running':
      return '⏳';
    case 'completed':
      return '✓';
    case 'failed':
      return '✗';
    case 'cancelled':
      return '⊘';
    default:
      return '🗂';
  }
});
</script>

<style lang="sass" scoped>
.session-stage-block
  padding: 6px 10px
  border-radius: 8px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)

  &--running
    border-color: color-mix(in srgb, var(--color-accent) 40%, var(--glass-border))
  &--failed
    border-color: color-mix(in srgb, var(--color-danger) 40%, var(--glass-border))
  &--cancelled
    opacity: 0.7

  &__header
    display: flex
    align-items: center
    gap: 6px

  &__icon
    font-size: 13px
    flex-shrink: 0

  &__label
    font-size: 12px
    font-weight: 500
    color: var(--color-text-secondary)

  &__title
    font-size: 12px
    color: var(--color-text-primary)
    flex: 1

  &__agent
    font-size: 11px
    color: var(--color-text-tertiary)
    padding: 1px 6px
    border-radius: 4px
    background: color-mix(in srgb, var(--color-accent) 8%, transparent)

  &__status
    font-size: 12px
    flex-shrink: 0
    &--running
      color: var(--color-accent)
    &--completed
      color: var(--color-success)
    &--failed
      color: var(--color-danger)
    &--cancelled
      color: var(--color-text-tertiary)

  &__duration
    font-size: 11px
    color: var(--color-text-secondary)
    margin-top: 2px
    margin-left: 21px
</style>
