<template>
  <div v-if="timeline && timeline.phases.length > 0" class="orchestration-timeline">
    <div class="timeline-header">
      <span class="text-caption text-grey">{{ t('orchestration.timeline.title') }}</span>
      <span class="text-caption text-grey-6">{{ formatDuration(timeline.totalDurationMs) }}</span>
    </div>
    <div class="timeline-phases">
      <div v-for="phase in timeline.phases" :key="phase.phase" class="timeline-phase">
        <!-- 阶段头 -->
        <div class="phase-header" @click="togglePhase(phase.phase)">
          <q-icon :name="phaseIcon(phase.status)" :color="phaseColor(phase.status)" />
          <span class="phase-name">{{ t(`orchestration.timeline.phases.${phase.phase}`) }}</span>
          <span class="phase-duration">{{ formatDuration(phase.durationMs) }}</span>
          <q-icon v-if="phase.steps.length > 0" :name="expandedPhases.has(phase.phase) ? 'expand_less' : 'expand_more'" />
        </div>
        <!-- 步骤列表 -->
        <div v-if="expandedPhases.has(phase.phase) && phase.steps.length > 0" class="phase-steps">
          <div v-for="(step, idx) in phase.steps" :key="idx" class="timeline-step">
            <q-icon :name="stepIcon(step.status)" :color="stepColor(step.status)" size="16px" />
            <span class="step-name">{{ step.name }}</span>
            <span class="step-duration">{{ formatDuration(step.durationMs) }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type {
  OrchestrationTimelineData,
  TimelinePhaseType,
  TimelineStepStatus,
} from './timelineTypes';

defineProps<{
  timeline: OrchestrationTimelineData | null;
}>();

const { t } = useI18n();

const expandedPhases = ref<Set<TimelinePhaseType>>(new Set());

function togglePhase(phase: TimelinePhaseType) {
  if (expandedPhases.value.has(phase)) {
    expandedPhases.value.delete(phase);
  } else {
    expandedPhases.value.add(phase);
  }
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  const totalSec = Math.floor(ms / 1000);
  const minutes = Math.floor(totalSec / 60);
  const seconds = totalSec % 60;
  return `${minutes}m${seconds.toString().padStart(2, '0')}s`;
}

function phaseIcon(status: TimelineStepStatus): string {
  switch (status) {
    case 'running':
      return 'schedule';
    case 'completed':
      return 'check_circle';
    case 'failed':
      return 'error';
    case 'skipped':
      return 'skip_next';
    default:
      return 'schedule';
  }
}

function phaseColor(status: TimelineStepStatus): string {
  switch (status) {
    case 'running':
      return 'primary';
    case 'completed':
      return 'positive';
    case 'failed':
      return 'negative';
    case 'skipped':
      return 'grey';
    default:
      return 'grey';
  }
}

function stepIcon(status: TimelineStepStatus): string {
  return phaseIcon(status);
}

function stepColor(status: TimelineStepStatus): string {
  return phaseColor(status);
}
</script>

<style scoped lang="sass">
.orchestration-timeline
  padding: 8px 12px
  border-top: 1px solid var(--glass-border)
  background: color-mix(in srgb, var(--glass-surface) 40%, transparent)

.timeline-header
  display: flex
  align-items: center
  justify-content: space-between
  margin-bottom: 6px

.timeline-phases
  display: flex
  flex-direction: column
  gap: 4px

.timeline-phase
  border-radius: 6px

.phase-header
  display: flex
  align-items: center
  gap: 6px
  padding: 4px 6px
  cursor: pointer
  border-radius: 4px
  transition: background 0.15s ease

  &:hover
    background: color-mix(in srgb, var(--color-accent) 8%, transparent)

.phase-name
  font-size: 12px
  color: var(--color-text-primary)
  flex: 1

.phase-duration
  font-size: 11px
  color: var(--color-text-secondary)

.phase-steps
  padding: 2px 0 4px 24px
  display: flex
  flex-direction: column
  gap: 2px

.timeline-step
  display: flex
  align-items: center
  gap: 6px
  padding: 2px 4px

.step-name
  font-size: 11px
  color: var(--color-text-secondary)
  flex: 1

.step-duration
  font-size: 10px
  color: var(--color-text-tertiary)
</style>
