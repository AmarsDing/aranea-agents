<template>
  <div
    class="team-progress-card"
    :class="`team-progress-card--${statusClass}`"
    role="button"
    tabindex="0"
    @click="$emit('click')"
    @keydown.enter="$emit('click')"
  >
    <div class="row items-center no-wrap q-gutter-sm">
      <div class="team-progress-card__icon">
        <q-spinner v-if="isRunning" size="16px" color="accent" />
        <q-icon v-else-if="team.status === 'completed'" name="check_circle" size="16px" color="positive" />
        <q-icon v-else-if="team.status === 'failed'" name="error" size="16px" color="negative" />
        <q-icon v-else-if="team.status === 'cancelled'" name="cancel" size="16px" color="grey-6" />
        <q-icon v-else-if="team.status === 'waiting_deps'" name="schedule" size="16px" color="warning" />
        <q-icon v-else name="groups" size="16px" color="accent" />
      </div>
      <div class="col min-width-0">
        <div class="team-progress-card__name ellipsis">{{ team.teamName }}</div>
        <div v-if="team.taskSummary" class="team-progress-card__summary ellipsis">{{ team.taskSummary }}</div>
      </div>
      <OrchestrationModeBadge
        v-if="topology && topology !== 'coordinator'"
        :topology="topology"
        :reason="team.topologyReason"
      />
      <q-btn
        v-if="canCancel"
        flat
        dense
        round
        icon="close"
        size="xs"
        class="team-progress-card__cancel"
        @click.stop="$emit('cancel')"
      >
        <q-tooltip>取消团队</q-tooltip>
      </q-btn>
    </div>

    <div v-if="isWaitingDeps" class="team-progress-card__deps q-mt-xs">
      <q-icon name="schedule" size="12px" color="warning" class="q-mr-xs" />
      <span class="text-caption">等待依赖完成</span>
      <span v-if="team.dependsOn && team.dependsOn.length > 0" class="text-caption text-grey-6 q-ml-xs">
        ({{ team.dependsOn.length }} 个前置任务)
      </span>
    </div>

    <div v-if="team.totalSteps > 0" class="team-progress-card__progress q-mt-xs">
      <q-linear-progress :value="progressValue" size="3px" rounded :color="progressColor" />
      <div class="row items-center justify-between q-mt-xs">
        <span class="text-caption text-grey-6"> {{ team.completedSteps }} / {{ team.totalSteps }} 步骤 </span>
        <span v-if="durationText" class="text-caption text-grey-6">
          {{ durationText }}
        </span>
      </div>
    </div>

    <div v-if="team.memberAvatars.length > 0" class="team-progress-card__avatars row items-center q-gutter-xs q-mt-xs">
      <q-avatar v-for="(url, idx) in team.memberAvatars.slice(0, 4)" :key="idx" size="18px">
        <img v-if="url" :src="url" alt="" />
        <q-icon v-else name="person" size="12px" color="grey-6" />
      </q-avatar>
      <span v-if="team.memberAvatars.length > 4" class="text-caption text-grey-6">
        +{{ team.memberAvatars.length - 4 }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { SpiritTeam, TopologyType } from '../../features/spirit/types';
import OrchestrationModeBadge from './OrchestrationModeBadge.vue';

const props = defineProps<{
  team: SpiritTeam;
}>();

defineEmits<{
  click: [];
  cancel: [];
}>();

const modeToTopology = (mode: SpiritTeam['mode']): TopologyType | null => {
  const mapping: Partial<Record<SpiritTeam['mode'], TopologyType>> = {
    parallel: 'parallel',
    sequential: 'sequential',
    coordinator: 'coordinator',
    critic_loop: 'hybrid',
    swarm: 'hybrid',
    adaptive: 'hybrid',
    direct: 'sequential',
  };
  return mapping[mode] ?? null;
};

const topology = computed(() => modeToTopology(props.team.mode));

const isRunning = computed(
  () => props.team.status === 'running' || props.team.status === 'assembled' || props.team.status === 'assembling',
);

const isWaitingDeps = computed(() => props.team.status === 'waiting_deps');

const canCancel = computed(
  () => props.team.status === 'running' || props.team.status === 'assembled' || props.team.status === 'waiting_deps',
);

const statusClass = computed(() => {
  switch (props.team.status) {
    case 'completed':
      return 'completed';
    case 'failed':
      return 'failed';
    case 'cancelled':
      return 'cancelled';
    case 'waiting_deps':
      return 'waiting';
    case 'running':
    case 'assembled':
    case 'assembling':
      return 'running';
    default:
      return 'idle';
  }
});

const progressValue = computed(() =>
  props.team.totalSteps > 0 ? props.team.completedSteps / props.team.totalSteps : 0,
);

const progressColor = computed(() => {
  if (props.team.status === 'completed') return 'positive';
  if (props.team.status === 'failed') return 'negative';
  return 'accent';
});

const durationText = computed(() => {
  if (!props.team.durationMs) return '';
  const seconds = Math.floor(props.team.durationMs / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  return `${minutes}m ${seconds % 60}s`;
});
</script>

<style scoped lang="sass">
.team-progress-card
  padding: var(--space-3)
  border-radius: 10px
  border: 1px solid transparent
  background: color-mix(in srgb, var(--glass-surface) 40%, transparent)
  cursor: pointer
  transition: background 0.15s ease, border-color 0.15s ease

.team-progress-card:hover
  background: color-mix(in srgb, var(--glass-surface) 65%, transparent)
  border-color: var(--glass-border)

.team-progress-card--running
  border-left: 3px solid color-mix(in srgb, var(--color-accent) 50%, var(--glass-border))

.team-progress-card--completed
  border-left: 3px solid color-mix(in srgb, var(--color-success) 50%, var(--glass-border))

.team-progress-card--failed
  border-left: 3px solid color-mix(in srgb, var(--color-danger) 50%, var(--glass-border))

.team-progress-card--waiting
  border-left: 3px solid color-mix(in srgb, var(--color-warning) 50%, var(--glass-border))

.team-progress-card--cancelled
  opacity: 0.6

.team-progress-card__icon
  display: flex
  align-items: center
  justify-content: center
  width: 24px
  height: 24px
  border-radius: 6px
  background: color-mix(in srgb, var(--color-accent) 8%, var(--glass-surface))
  flex-shrink: 0

.team-progress-card__name
  font-size: var(--text-xs)
  font-weight: 600
  color: var(--color-text-primary)
  line-height: 1.3

.team-progress-card__summary
  font-size: 11px
  color: var(--color-text-tertiary)
  line-height: 1.3

.team-progress-card__deps
  display: flex
  align-items: center
  margin-left: 32px
  padding: 2px 6px
  border-radius: 4px
  background: color-mix(in srgb, var(--color-warning) 8%, transparent)

.team-progress-card__cancel
  color: var(--color-text-tertiary)
  opacity: 0.6
  &:hover
    opacity: 1
    color: var(--color-danger)

.team-progress-card__progress
  margin-left: 32px

.team-progress-card__avatars
  margin-left: 32px
  flex-wrap: nowrap
</style>
