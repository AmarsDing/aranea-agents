<template>
  <div class="graph-stage-block" :class="`graph-stage-block--${activity.status}`">
    <!-- Header -->
    <div class="graph-stage-block__header" @click="toggleCollapse">
      <span class="graph-stage-block__icon">🔬</span>
      <span class="graph-stage-block__label">{{ t('chat.graphStage.label') }}</span>
      <span class="graph-stage-block__title">{{ displayTitle }}</span>
      <span v-if="progressText" class="graph-stage-block__progress">{{ progressText }}</span>
      <span class="graph-stage-block__status" :class="`graph-stage-block__status--${activity.status}`">
        {{ statusIcon }}
      </span>
      <span
        v-if="activity.nodes?.length && activity.status !== 'running'"
        class="graph-stage-block__chevron"
        :class="{ 'graph-stage-block__chevron--expanded': !collapsed }"
      >
        ▸
      </span>
    </div>

    <!-- Duration -->
    <div v-if="activity.durationMs != null && isTerminal" class="graph-stage-block__duration">
      {{ formatDuration(activity.durationMs) }}
    </div>

    <!-- DAG node list (linearized; full DagView is a future enhancement) -->
    <div v-if="showNodes" class="graph-stage-block__nodes">
      <div
        v-for="node in activity.nodes"
        :key="node.nodeId"
        class="graph-stage-block__node"
        :class="`graph-stage-block__node--${node.status}`"
      >
        <span class="graph-stage-block__node-dot" :class="`graph-stage-block__node-dot--${node.status}`">
          <span v-if="node.status === 'running'" class="graph-stage-block__pulse" />
        </span>
        <span class="graph-stage-block__node-label">{{ node.label || node.nodeId }}</span>
        <span v-if="node.dependsOn?.length" class="graph-stage-block__node-dep">
          {{ t('chat.graphStage.dependsOn') }} {{ node.dependsOn.join(', ') }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { GraphStageEvent } from '../../features/chat/streamEventTypes';
import { formatDuration } from '../../features/chat/agentTreeUtils';

const props = defineProps<{
  activity: GraphStageEvent;
}>();

const { t } = useI18n();

const collapsed = ref(props.activity.status !== 'running');

function toggleCollapse() {
  if (props.activity.status === 'running') return;
  collapsed.value = !collapsed.value;
}

const isTerminal = computed(
  () =>
    props.activity.status === 'completed' ||
    props.activity.status === 'failed' ||
    props.activity.status === 'cancelled',
);

const displayTitle = computed(() => {
  if (props.activity.title) return props.activity.title;
  switch (props.activity.status) {
    case 'running':
      return t('chat.graphStage.executing');
    case 'completed':
      return t('chat.graphStage.completed');
    case 'failed':
      return t('chat.graphStage.failed');
    case 'cancelled':
      return t('chat.graphStage.cancelled');
    default:
      return '';
  }
});

const progressText = computed(() => {
  const nodes = props.activity.nodes;
  if (!nodes?.length) return '';
  const completed = nodes.filter(
    (n) => n.status === 'completed' || n.status === 'failed' || n.status === 'skipped',
  ).length;
  return `${completed}/${nodes.length}`;
});

const showNodes = computed(() => {
  if (!props.activity.nodes?.length) return false;
  if (props.activity.status === 'running') return true;
  return !collapsed.value;
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
      return '🔬';
  }
});
</script>

<style lang="sass" scoped>
.graph-stage-block
  padding: 8px 10px
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
    cursor: default

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
    flex: 1

  &__progress
    font-size: 11px
    color: var(--color-text-tertiary)
    font-variant-numeric: tabular-nums

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

  &__chevron
    font-size: 10px
    color: var(--color-text-tertiary)
    transition: transform 0.15s ease
    &--expanded
      transform: rotate(90deg)

  &__duration
    font-size: 11px
    color: var(--color-text-secondary)
    margin-top: 2px
    margin-left: 22px

  &__nodes
    display: flex
    flex-direction: column
    gap: 4px
    margin-top: 6px
    padding-left: 22px

  &__node
    display: flex
    align-items: center
    gap: 6px
    padding: 2px 0

  &__node-dot
    width: 8px
    height: 8px
    border-radius: 50%
    flex-shrink: 0
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
    &--skipped
      background: var(--color-text-tertiary)
      opacity: 0.5

  &__node-label
    font-size: 12px
    color: var(--color-text-primary)
    flex: 1

  &__node-dep
    font-size: 11px
    color: var(--color-text-tertiary)

  &__pulse
    position: absolute
    inset: -3px
    border-radius: 50%
    border: 1.5px solid var(--color-accent)
    animation: graph-pulse 1.5s ease-in-out infinite

@keyframes graph-pulse
  0%, 100%
    opacity: 1
    transform: scale(1)
  50%
    opacity: 0.3
    transform: scale(1.4)
</style>
