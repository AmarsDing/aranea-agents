<template>
  <div
    class="team-progress-card"
    :class="`team-progress-card--${statusClass}`"
    role="button"
    tabindex="0"
    @click="toggleExpand"
    @keydown.enter="toggleExpand"
  >
    <div class="row items-center no-wrap q-gutter-sm">
      <q-icon
        :name="expanded ? 'expand_more' : 'chevron_right'"
        size="16px"
        color="grey"
        class="team-progress-card__chevron"
      />
      <div class="team-progress-card__icon">
        <q-spinner v-if="isRunning" size="16px" color="accent" />
        <q-icon v-else-if="team.status === 'completed'" name="check_circle" size="16px" color="positive" />
        <q-icon v-else-if="team.status === 'failed'" name="error" size="16px" color="negative" />
        <q-icon v-else-if="team.status === 'cancelled'" name="cancel" size="16px" color="grey" />
        <q-icon v-else-if="team.status === 'interrupted'" name="pause_circle" size="16px" color="warning" />
        <q-icon v-else-if="team.status === 'pending'" name="schedule" size="16px" color="warning" />
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
      <template v-if="team.status === 'interrupted'">
        <button class="tp-action-btn resume" @click.stop="$emit('resume')">{{ t('spirit.resume') }}</button>
        <button class="tp-action-btn cancel" @click.stop="$emit('cancel')">{{ t('spirit.cancel') }}</button>
      </template>
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
        <q-tooltip>{{ t('spirit.cancelTeam') }}</q-tooltip>
      </q-btn>
      <q-btn
        v-if="canRetry"
        flat
        dense
        round
        icon="refresh"
        size="xs"
        class="team-progress-card__retry"
        @click.stop="$emit('retry')"
      >
        <q-tooltip>{{ t('spirit.retryTeam') }}</q-tooltip>
      </q-btn>
      <q-btn
        v-if="canArchive"
        flat
        dense
        round
        icon="archive"
        size="xs"
        class="team-progress-card__archive"
        @click.stop="$emit('archive')"
      >
        <q-tooltip>{{ t('spirit.archiveTeam') }}</q-tooltip>
      </q-btn>
    </div>

    <div v-if="team.status === 'pending'" class="team-progress-card__deps q-mt-xs">
      <q-icon v-if="isWaitingDeps" name="schedule" size="12px" color="warning" class="q-mr-xs" />
      <q-icon v-else name="hourglass_top" size="12px" color="grey" class="q-mr-xs" />
      <span class="text-caption">{{ isWaitingDeps ? t('spirit.waitingDeps') : t('spirit.waitingSchedule') }}</span>
      <span v-if="isWaitingDeps" class="text-caption text-grey q-ml-xs">
        ({{ t('spirit.prerequisiteTasks', { count: team.dependsOn!.length }) }})
      </span>
    </div>

    <div v-if="team.totalSteps > 0" class="team-progress-card__progress q-mt-xs">
      <q-linear-progress :value="progressValue" size="3px" rounded :color="progressColor" />
      <div class="row items-center justify-between q-mt-xs">
        <span class="text-caption text-grey"> {{ team.completedSteps }} / {{ team.totalSteps }} {{ t('spirit.steps') }} </span>
        <span class="text-caption text-grey">
          <template v-if="etaText">{{ t('spirit.estimated', { time: etaText }) }}</template>
          <template v-else-if="durationText">{{ durationText }}</template>
        </span>
      </div>
    </div>

    <div v-if="failedSummary" class="team-progress-card__error q-mt-xs">
      <q-icon name="error_outline" size="12px" class="q-mr-xs" />
      <span class="text-caption ellipsis">{{ failedSummary }}</span>
    </div>

    <div v-if="team.memberAvatars.length > 0" class="team-progress-card__avatars row items-center q-gutter-xs q-mt-xs">
      <q-avatar v-for="(url, idx) in team.memberAvatars.slice(0, 4)" :key="idx" size="18px">
        <img v-if="url" :src="url" alt="" />
        <q-icon v-else name="person" size="12px" color="grey" />
      </q-avatar>
      <span v-if="team.memberAvatars.length > 4" class="text-caption text-grey">
        +{{ team.memberAvatars.length - 4 }}
      </span>
    </div>

    <!-- Expandable agent details body -->
    <transition name="tp-expand">
      <div v-if="expanded" class="team-progress-card__details q-mt-sm" @click.stop>
        <div class="text-caption text-weight-medium text-grey q-mb-xs">{{ t('spirit.agentDetails') }}</div>
        <template v-if="team.members.length > 0">
          <div
            v-for="member in team.members"
            :key="member.agentKey"
            class="team-progress-card__detail-item row items-center q-gutter-xs"
          >
            <q-avatar size="16px">
              <img v-if="member.avatarUrl" :src="member.avatarUrl" alt="" />
              <q-icon v-else name="person" size="12px" color="grey" />
            </q-avatar>
            <span class="text-caption col ellipsis">{{ member.displayName }}</span>
            <AgentStatusLabel :label="spiritMemberStatusToLabel(member.status)" />
          </div>
        </template>
        <div v-else class="text-caption text-grey">{{ t('spirit.noAgentDetails') }}</div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SpiritTeam, TopologyType } from '../../features/spirit/types';
import OrchestrationModeBadge from './OrchestrationModeBadge.vue';
import AgentStatusLabel from './AgentStatusLabel.vue';
import { formatDuration, spiritMemberStatusToLabel } from '../../features/spirit/spiritUi';

const { t } = useI18n();

const props = defineProps<{
  team: SpiritTeam;
}>();

defineEmits<{
  click: [];
  cancel: [];
  retry: [];
  resume: [];
  archive: [];
}>();

const expanded = ref<boolean>(false);

function toggleExpand(): void {
  expanded.value = !expanded.value;
}

const modeToTopology = (mode: SpiritTeam['mode']): TopologyType | null => {
  const mapping: Partial<Record<SpiritTeam['mode'], TopologyType>> = {
    parallel: 'parallel',
    sequential: 'sequential',
    coordinator: 'coordinator',
    critic_loop: 'hybrid',
    swarm: 'hybrid',
    adaptive: 'hybrid',
  };
  return mapping[mode] ?? null;
};

const topology = computed(() => modeToTopology(props.team.mode));

const isRunning = computed(() => props.team.status === 'running' || props.team.status === 'pending');

const isWaitingDeps = computed(() => props.team.status === 'pending' && !!(props.team.dependsOn && props.team.dependsOn.length > 0));

const canCancel = computed(() => props.team.status === 'running' || props.team.status === 'pending');

const canRetry = computed(() => props.team.status === 'failed');

const canArchive = computed(() => props.team.status === 'completed' || props.team.status === 'failed' || props.team.status === 'cancelled');

const statusClass = computed(() => {
  switch (props.team.status) {
    case 'completed':
      return 'completed';
    case 'failed':
      return 'failed';
    case 'cancelled':
      return 'cancelled';
    case 'interrupted':
      return 'interrupted';
    case 'pending':
      return 'waiting';
    case 'running':
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

const durationText = computed(() => formatDuration(props.team.durationMs));

/** ETA: estimate remaining time based on completed steps and elapsed duration. */
const etaText = computed(() => {
  const { completedSteps, totalSteps, durationMs } = props.team;
  if (props.team.status !== 'running' || completedSteps <= 0 || totalSteps <= 0 || completedSteps >= totalSteps) return '';
  if (durationMs <= 0) return '';
  const msPerStep = durationMs / completedSteps;
  const remainingMs = Math.round(msPerStep * (totalSteps - completedSteps));
  return formatDuration(remainingMs);
});

/** Show error/interrupt summary when team failed or interrupted. */
const failedSummary = computed(() => {
  if (props.team.status === 'failed') return props.team.interruptReason || t('spirit.executionFailed');
  if (props.team.status === 'interrupted') return props.team.interruptReason || t('spirit.interrupted');
  return '';
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

.team-progress-card--interrupted
  border-left: 3px solid color-mix(in srgb, var(--color-warning) 50%, var(--glass-border))

.team-progress-card__chevron
  flex-shrink: 0
  transition: transform 0.15s ease

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

.team-progress-card__retry
  color: var(--color-text-tertiary)
  opacity: 0.6
  &:hover
    opacity: 1
    color: var(--color-accent)

.team-progress-card__archive
  color: var(--color-text-tertiary)
  opacity: 0.6
  &:hover
    opacity: 1
    color: var(--color-text-secondary)

.team-progress-card__progress
  margin-left: 32px

.team-progress-card__error
  display: flex
  align-items: center
  margin-left: 32px
  padding: 2px 6px
  border-radius: 4px
  color: var(--color-danger)
  background: color-mix(in srgb, var(--color-danger) 6%, transparent)

.team-progress-card__avatars
  margin-left: 32px
  flex-wrap: nowrap

.team-progress-card__details
  margin-left: 32px
  padding: var(--space-2) var(--space-3)
  border-radius: 6px
  background: color-mix(in srgb, var(--glass-surface) 50%, transparent)
  border: 1px solid var(--glass-border)

.team-progress-card__detail-item
  padding: 2px 0

.tp-action-btn
  border: none
  border-radius: 4px
  padding: 2px 8px
  font-size: 11px
  font-weight: 600
  line-height: 1.4
  cursor: pointer
  transition: opacity 0.15s ease
  flex-shrink: 0
  &:hover
    opacity: 0.85

.tp-action-btn.resume
  background: var(--color-accent)
  color: white

.tp-action-btn.cancel
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
  color: var(--color-text-secondary)

// Expand/collapse transition
.tp-expand-enter-active,
.tp-expand-leave-active
  transition: all 0.2s ease
  overflow: hidden

.tp-expand-enter-from,
.tp-expand-leave-to
  opacity: 0
  max-height: 0
  margin-top: 0

.tp-expand-enter-to,
.tp-expand-leave-from
  opacity: 1
  max-height: 300px
</style>
