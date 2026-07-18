<template>
  <div :class="['observe-node', `observe-node--${data.status}`]">
    <!-- Header: avatar + name + status -->
    <header class="observe-node__header">
      <span class="observe-node__avatar">{{ agentInitial }}</span>
      <span class="observe-node__name">{{ data.label }}</span>
      <ObserveStatusBadge :status="data.status" />
    </header>

    <!-- Progress bar (running) -->
    <div v-if="data.status === 'running'" class="observe-node__progress">
      <q-linear-progress :value="progressValue" color="warning" size="3px" rounded />
      <span class="observe-node__progress-label">{{ progressLabel }}</span>
    </div>

    <!-- Media preview -->
    <NodeMediaPreview
      v-if="data.mediaOutput?.length"
      :artifacts="data.mediaOutput"
      @preview="$emit('preview', $event)"
    />

    <!-- Latest activity -->
    <div class="observe-node__activity">
      <q-icon :name="activityIcon" size="12px" />
      <span class="observe-node__activity-text">{{ activitySummary }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { GraphNodeStatus } from '../../../features/chat/v2Types';
import type { MediaArtifact } from '../../../features/chat/mediaTypes';
import ObserveStatusBadge from './ObserveStatusBadge.vue';
import NodeMediaPreview from './NodeMediaPreview.vue';

interface ObserveNodeData {
  label: string;
  dagNodeId: string;
  teamStageId: string;
  status: GraphNodeStatus;
  dependsOn: string[];
  mediaOutput: MediaArtifact[];
  progress?: { value: number; max: number; label?: string };
}

const props = defineProps<{ data: ObserveNodeData }>();
defineEmits<{ preview: [art: MediaArtifact] }>();

const { t } = useI18n();

const agentInitial = computed(() => {
  const name = props.data.label || '?';
  return name.charAt(0).toUpperCase();
});

// Progress: defaults to indeterminate animation for running state.
// Phase 4 will add real progress from activity meta.
const progressValue = computed(() => {
  if (props.data.progress) {
    return props.data.progress.value / props.data.progress.max;
  }
  return 0.5; // indeterminate-like
});

const progressLabel = computed(() => {
  if (props.data.progress?.label) {
    return props.data.progress.label;
  }
  if (props.data.progress) {
    return `${Math.round((props.data.progress.value / props.data.progress.max) * 100)}%`;
  }
  return '';
});

const activityIcon = computed(() => {
  switch (props.data.status) {
    case 'running':
      return 'bolt';
    case 'completed':
      return 'check_circle';
    case 'failed':
      return 'error';
    default:
      return 'hourglass_empty';
  }
});

const ACTIVITY_KEY_MAP: Record<GraphNodeStatus, string> = {
  pending: 'observe.statusPending',
  running: 'observe.statusRunning',
  completed: 'observe.statusCompleted',
  failed: 'observe.statusFailed',
  interrupted: 'observe.statusInterrupted',
};

const activitySummary = computed(() => t(ACTIVITY_KEY_MAP[props.data.status] || ''));
</script>

<style scoped lang="sass">
.observe-node
  border: 2px solid var(--color-border)
  border-radius: 8px
  background: var(--color-surface)
  min-width: 180px
  max-width: 240px
  padding: 8px
  font-size: 12px
  color: var(--color-text-primary)
  transition: border-color 0.3s ease, box-shadow 0.3s ease

  &--pending
    border-color: var(--color-border)

  &--running
    border-color: var(--color-warning)
    animation: observe-pulse 1.5s infinite

  &--completed
    border-color: var(--color-positive)

  &--failed
    border-color: var(--color-negative)

  &--interrupted
    border-color: var(--color-warning)
    opacity: 0.7

.observe-node__header
  display: flex
  align-items: center
  gap: 6px
  margin-bottom: 4px

.observe-node__avatar
  width: 24px
  height: 24px
  border-radius: 50%
  background: var(--color-primary)
  color: white
  display: flex
  align-items: center
  justify-content: center
  font-size: 11px
  font-weight: 600
  flex-shrink: 0

.observe-node__name
  font-weight: 500
  flex: 1
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.observe-node__progress
  margin: 4px 0

.observe-node__progress-label
  font-size: 10px
  color: var(--color-text-tertiary)
  margin-top: 2px

.observe-node__activity
  display: flex
  align-items: center
  gap: 4px
  margin-top: 6px
  color: var(--color-text-tertiary)
  font-size: 11px

.observe-node__activity-text
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

@keyframes observe-pulse
  0%, 100%
    box-shadow: 0 0 0 0 rgba(255, 152, 0, 0.4)
  50%
    box-shadow: 0 0 0 6px rgba(255, 152, 0, 0)
</style>
