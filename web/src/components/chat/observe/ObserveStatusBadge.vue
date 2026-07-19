<template>
  <span :class="['observe-status-badge', `observe-status-badge--${status}`]">
    {{ statusLabel }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { GraphNodeStatus } from '../../../features/chat/v2Types';

const { t } = useI18n();

const props = defineProps<{ status: GraphNodeStatus }>();

const STATUS_MAP: Record<GraphNodeStatus, string> = {
  pending: 'observe.statusPending',
  running: 'observe.statusRunning',
  completed: 'observe.statusCompleted',
  failed: 'observe.statusFailed',
  interrupted: 'observe.statusInterrupted',
};

const statusLabel = computed(() => t(STATUS_MAP[props.status] || props.status));
</script>

<style scoped lang="sass">
.observe-status-badge
  font-size: 10px
  padding: 1px 6px
  border-radius: 8px
  font-weight: 500

  &--pending
    background: var(--color-surface-soft)
    color: var(--color-text-tertiary)

  &--running
    background: color-mix(in srgb, var(--color-warning) 20%, transparent)
    color: var(--color-warning)

  &--completed
    background: color-mix(in srgb, var(--color-success) 20%, transparent)
    color: var(--color-success)

  &--failed
    background: color-mix(in srgb, var(--color-danger) 20%, transparent)
    color: var(--color-danger)

  &--interrupted
    background: color-mix(in srgb, var(--color-warning) 15%, transparent)
    color: var(--color-text-secondary)
</style>
