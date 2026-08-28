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
      <div class="team-progress-card__avatar-wrap">
        <div class="team-progress-card__avatar">{{ teamInitial }}</div>
        <span class="team-progress-card__avatar-status" :class="`team-progress-card__avatar-status--${statusClass}`">
          <q-spinner v-if="isRunning" size="8px" color="accent" />
        </span>
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
        <span class="text-caption text-grey">
          {{ team.completedSteps }} / {{ team.totalSteps }} {{ t('spirit.steps') }}
        </span>
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

    <!-- 成员失败警告：partial_failure 持久态或旧数据 completed+失败成员时显示 -->
    <div
      v-if="hasFailedMember && (team.status === 'completed' || team.status === 'partial_failure')"
      class="team-progress-card__warning q-mt-xs"
    >
      <q-icon name="warning" size="12px" class="q-mr-xs" />
      <span class="text-caption">{{ t('spirit.partialMemberFailure', { count: failedMemberCount }) }}</span>
    </div>

    <div v-if="team.members.length > 0" class="team-progress-card__avatars row items-center q-gutter-xs q-mt-xs">
      <div v-for="(m, idx) in team.members.slice(0, 4)" :key="idx" class="team-progress-card__avatar-initial">
        {{ nameInitial(m.displayName) }}
      </div>
      <span v-if="team.members.length > 4" class="text-caption text-grey"> +{{ team.members.length - 4 }} </span>
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
            <div class="team-progress-card__detail-avatar">{{ memberInitial(member) }}</div>
            <div class="team-progress-card__detail-body col min-width-0">
              <span class="text-caption ellipsis">{{ member.displayName }}</span>
              <AgentStatusLabel :label="spiritMemberStatusToLabel(member.status)" />
              <div
                v-if="member.status === 'idle' && team.status === 'running'"
                class="team-progress-card__detail-waiting"
              >
                {{ t('spirit.waitingForPredecessor') }}
              </div>
            </div>
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
import type { SpiritTeam, SpiritMember } from '../../features/spirit/types';
import OrchestrationModeBadge from './OrchestrationModeBadge.vue';
import AgentStatusLabel from './AgentStatusLabel.vue';
import { formatDuration, spiritMemberStatusToLabel, nameInitial, modeToTopology } from '../../features/spirit/spiritUi';

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

function memberInitial(member: SpiritMember): string {
  return nameInitial(member.displayName);
}

const topology = computed(() => modeToTopology(props.team.mode));

const teamInitial = computed(() => nameInitial(props.team.teamName));

const isRunning = computed(() => props.team.status === 'running' || props.team.status === 'pending');

const isWaitingDeps = computed(
  () => props.team.status === 'pending' && !!(props.team.dependsOn && props.team.dependsOn.length > 0),
);

const canCancel = computed(() => props.team.status === 'running' || props.team.status === 'pending');

// partial_failure 调度语义等同 completed：可重试（recover→pending）、可归档。
const canRetry = computed(() => props.team.status === 'failed' || props.team.status === 'partial_failure');

const canArchive = computed(
  () =>
    props.team.status === 'completed' ||
    props.team.status === 'partial_failure' ||
    props.team.status === 'failed' ||
    props.team.status === 'cancelled',
);

const statusClass = computed(() => {
  switch (props.team.status) {
    case 'completed':
      return 'completed';
    case 'partial_failure':
      return 'partial_failure';
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
  if (props.team.status === 'partial_failure') return 'warning';
  if (props.team.status === 'failed') return 'negative';
  return 'accent';
});

const durationText = computed(() => formatDuration(props.team.durationMs));

/** ETA: estimate remaining time based on completed steps and elapsed duration. */
const etaText = computed(() => {
  const { completedSteps, totalSteps, durationMs } = props.team;
  if (props.team.status !== 'running' || completedSteps <= 0 || totalSteps <= 0 || completedSteps >= totalSteps)
    return '';
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

/** 检查是否有成员失败（用于显示部分失败警告） */
const hasFailedMember = computed(() => props.team.members.some((m) => m.status === 'failed'));

/** 失败的成员数量 */
const failedMemberCount = computed(() => props.team.members.filter((m) => m.status === 'failed').length);
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

.team-progress-card__avatar-wrap
  position: relative
  flex-shrink: 0

.team-progress-card__avatar
  width: 24px
  height: 24px
  border-radius: 50%
  background: color-mix(in srgb, var(--color-accent) 10%, var(--glass-surface))
  color: var(--color-accent)
  font-size: 10px
  font-weight: 600
  display: flex
  align-items: center
  justify-content: center

.team-progress-card__avatar-status
  position: absolute
  bottom: -1px
  right: -1px
  width: 10px
  height: 10px
  border-radius: 50%
  border: 1.5px solid var(--canvas-base)
  display: flex
  align-items: center
  justify-content: center

.team-progress-card__avatar-status--running
  background: var(--color-accent)
.team-progress-card__avatar-status--completed
  background: var(--color-success)
.team-progress-card__avatar-status--partial_failure
  background: var(--color-warning)
.team-progress-card__avatar-status--failed
  background: var(--color-danger)
.team-progress-card__avatar-status--interrupted
  background: var(--color-warning)
.team-progress-card__avatar-status--waiting
  background: var(--color-warning)
.team-progress-card__avatar-status--cancelled
  background: var(--color-text-tertiary)
.team-progress-card__avatar-status--idle
  background: var(--color-text-tertiary)

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

.team-progress-card__warning
  display: flex
  align-items: center
  margin-left: 32px
  padding: 2px 6px
  border-radius: 4px
  color: var(--color-warning)
  background: color-mix(in srgb, var(--color-warning) 8%, transparent)

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

.team-progress-card__detail-avatar
  width: 16px
  height: 16px
  border-radius: 50%
  background: var(--glass-elevated, var(--glass-surface))
  display: flex
  align-items: center
  justify-content: center
  font-size: 8px
  font-weight: 600
  color: var(--color-text-secondary)
  flex-shrink: 0

.team-progress-card__detail-body
  display: flex
  align-items: center
  gap: 4px
  min-width: 0

.team-progress-card__detail-waiting
  font-size: var(--text-xs)
  color: var(--color-text-tertiary)
  font-style: italic

.team-progress-card__avatar-initial
  width: 18px
  height: 18px
  border-radius: 50%
  background: color-mix(in srgb, var(--color-accent) 10%, var(--glass-surface))
  color: var(--color-accent)
  font-size: 9px
  font-weight: 600
  display: flex
  align-items: center
  justify-content: center

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
  color: var(--color-on-accent, white)

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
