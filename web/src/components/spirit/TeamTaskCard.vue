<template>
  <div
    class="team-task-card"
    :class="{
      'team-task-card--active': active,
      'team-task-card--expanded': expanded,
    }"
    role="button"
    tabindex="0"
    @click="$emit('click')"
    @keydown.enter="$emit('click')"
    @keydown.space.prevent="$emit('click')"
  >
    <div class="row items-center no-wrap q-gutter-sm">
      <div class="team-task-card__icon" :class="`team-task-card__icon--${teamStatusColor}`">
        <q-icon name="groups" size="18px" />
      </div>
      <div class="col min-width-0">
        <div class="team-task-card__name ellipsis">{{ team.teamName }}</div>
        <div class="team-task-card__summary ellipsis">
          <span :class="`team-task-card__status-text--${teamStatusColor}`">{{ teamStatusText }}</span>
          <span v-if="durationText" class="q-ml-xs">· {{ durationText }}</span>
          <span v-if="team.taskSummary" class="q-ml-xs">· {{ team.taskSummary }}</span>
        </div>
        <!-- F-7: Mini progress bar in collapsed state -->
        <q-linear-progress
          v-if="team.progressPct > 0 || team.totalSteps > 0"
          :value="team.progressPct > 0 ? team.progressPct / 100 : team.completedSteps / Math.max(team.totalSteps, 1)"
          size="3px"
          rounded
          color="accent"
          class="q-mt-xs"
        />
        <span v-if="team.totalSteps > 0" class="team-task-card__step-count">
          {{ team.completedSteps }}/{{ team.totalSteps }}
        </span>
        <span v-if="modeTag" class="team-task-card__mode-tag">{{ modeTag }}</span>
      </div>
      <!-- F-7: Mini avatars in collapsed state -->
      <div v-if="team.members.length > 0" class="team-task-card__mini-avatars row items-center no-wrap">
        <div v-for="(initial, idx) in memberInitials.slice(0, 3)" :key="idx" class="team-task-card__mini-initial">
          {{ initial }}
        </div>
        <span v-if="team.members.length > 3" class="team-task-card__mini-more"> +{{ team.members.length - 3 }} </span>
      </div>
      <div v-if="failedSummary" class="team-task-card__error">
        <q-icon name="error_outline" size="12px" class="q-mr-xs" />
        <span class="ellipsis">{{ failedSummary }}</span>
      </div>
      <span class="team-task-card__status-badge" :class="`team-task-card__status-badge--${teamStatusColor}`">
        <span v-if="teamStatusColor === 'blue'" class="team-task-card__status-pulse" />
        <q-icon :name="teamStatusIcon" size="11px" class="q-mr-xs" />
        {{ teamStatusText }}
      </span>
      <q-icon
        name="expand_more"
        size="16px"
        class="team-task-card__expand"
        :class="{ 'team-task-card__expand--collapsed': !expanded }"
        @click.stop="$emit('toggle-expand')"
      />
    </div>

    <div v-if="expanded" class="team-task-card__detail q-mt-sm">
      <div class="row items-center q-gutter-xs q-mb-xs">
        <SessionStatusBadge :status="mappedStatus" :status-reason="undefined" :status-changed-at="undefined" />
        <AgentStatusLabel :label="teamStatusLabel" />
        <OrchestrationModeBadge v-if="team.mode" :topology="topologyFromMode" :reason="team.topologyReason" />
      </div>

      <div v-if="team.memberAvatars.length > 0" class="team-task-card__avatars row items-center q-gutter-xs q-mb-xs">
        <q-avatar v-for="(url, idx) in team.memberAvatars.slice(0, 4)" :key="idx" size="22px">
          <img v-if="url" :src="url" alt="" />
          <q-icon v-else name="person" size="14px" color="grey" />
        </q-avatar>
        <span v-if="team.memberAvatars.length > 4" class="text-caption text-grey">
          +{{ team.memberAvatars.length - 4 }}
        </span>
      </div>

      <div v-if="team.progressPct > 0 || team.totalSteps > 0" class="team-task-card__progress">
        <q-linear-progress
          :value="team.progressPct > 0 ? team.progressPct / 100 : team.completedSteps / team.totalSteps"
          size="4px"
          rounded
          color="accent"
          class="q-mt-xs"
        />
        <div class="text-caption text-grey q-mt-xs">
          {{
            team.progressPct > 0
              ? `${Math.round(team.progressPct)}%`
              : `${team.completedSteps} / ${team.totalSteps} ${t('spirit.steps')}`
          }}
        </div>
      </div>

      <div v-if="team.dependsOn && team.dependsOn.length > 0" class="text-caption text-grey q-mt-xs">
        <q-icon name="account_tree" size="14px" class="q-mr-xs" />
        {{ t('spirit.prerequisiteTasks', { count: team.dependsOn.length }) }}
      </div>

      <div v-if="team.sharedAgentIds.length > 0" class="team-task-card__shared-agent text-caption q-mt-xs">
        <q-icon name="share" size="14px" class="q-mr-xs" />
        {{ t('spirit.sharedAgents', { count: team.sharedAgentIds.length }) }}
      </div>

      <TeamMemberTreeNode
        v-if="team.members.length > 0"
        :members="team.members"
        :selectable="true"
        class="q-mt-sm"
        @select="(memberId) => $emit('select-member', memberId)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import SessionStatusBadge from '../sessions/SessionStatusBadge.vue';
import AgentStatusLabel from './AgentStatusLabel.vue';
import OrchestrationModeBadge from './OrchestrationModeBadge.vue';
import TeamMemberTreeNode from './TeamMemberTreeNode.vue';
import type { SpiritTeam } from '../../features/spirit/types';
import type { TopologyType } from '../../features/spirit/types';
import {
  mapSpiritStatusToSession,
  spiritTeamStatusToLabel,
  STATUS_LABEL_CONFIG,
  formatDuration,
  nameInitial,
  modeToTopology,
} from '../../features/spirit/spiritUi';

const { t } = useI18n();

const props = defineProps<{
  team: SpiritTeam;
  expanded: boolean;
  active: boolean;
}>();

defineEmits<{
  click: [];
  'toggle-expand': [];
  'select-member': [memberId: string];
}>();

const mappedStatus = computed(() => mapSpiritStatusToSession(props.team.status));

/** Map SpiritTeamMode to TopologyType for OrchestrationModeBadge. */
const topologyFromMode = computed<TopologyType>(() => modeToTopology(props.team.mode));

const teamStatusLabel = computed(() => spiritTeamStatusToLabel(props.team.status));

const teamStatusColor = computed(() => STATUS_LABEL_CONFIG[teamStatusLabel.value]?.dotColor ?? 'grey');

const teamStatusText = computed(() => STATUS_LABEL_CONFIG[teamStatusLabel.value]?.text ?? props.team.status);

/** Status icon from STATUS_LABEL_CONFIG for badge rendering. */
const teamStatusIcon = computed(() => STATUS_LABEL_CONFIG[teamStatusLabel.value]?.icon ?? 'circle');

/** Short mode tag for collapsed state: seq/par/hyb/crit/swm/adt */
const modeTag = computed(() => {
  const mode = props.team.mode;
  if (!mode || mode === 'coordinator') return '';
  const tags: Record<string, string> = {
    sequential: 'seq',
    parallel: 'par',
    critic_loop: 'crit',
    swarm: 'swm',
    adaptive: 'adt',
  };
  return tags[mode] ?? '';
});

/** Show duration in sidebar when available (running/completed/failed). */
const durationText = computed(() => formatDuration(props.team.durationMs));

/** Show error summary when team failed. */
const failedSummary = computed(() => {
  if (props.team.status !== 'failed') return '';
  return props.team.interruptReason || t('spirit.executionFailed');
});

const memberInitials = computed(() => props.team.members.map((m) => nameInitial(m.displayName)));
</script>

<style scoped lang="sass">
.team-task-card
  padding: var(--space-3)
  border-radius: 12px
  border: 1px solid transparent
  background: color-mix(in srgb, var(--glass-surface) 40%, transparent)
  cursor: pointer
  transition: background 0.15s ease, border-color 0.15s ease

.team-task-card:hover
  background: color-mix(in srgb, var(--glass-surface) 65%, transparent)
  border-color: var(--glass-border)

.team-task-card--active
  background: color-mix(in srgb, var(--color-accent) 8%, var(--glass-surface))
  border-color: color-mix(in srgb, var(--color-accent) 30%, var(--glass-border))

.team-task-card__icon
  display: flex
  align-items: center
  justify-content: center
  width: 28px
  height: 28px
  border-radius: 8px
  background: color-mix(in srgb, var(--color-accent) 10%, var(--glass-surface))
  color: var(--color-accent)
  flex-shrink: 0

.team-task-card__icon--blue
  background: color-mix(in srgb, var(--color-accent) 15%, var(--glass-surface))
  color: var(--color-accent)
.team-task-card__icon--green
  background: color-mix(in srgb, var(--color-success) 15%, var(--glass-surface))
  color: var(--color-success)
.team-task-card__icon--red
  background: color-mix(in srgb, var(--color-danger) 15%, var(--glass-surface))
  color: var(--color-danger)
.team-task-card__icon--orange
  background: color-mix(in srgb, var(--color-warning) 15%, var(--glass-surface))
  color: var(--color-warning)

.team-task-card__name
  font-size: var(--text-sm)
  font-weight: 600
  color: var(--color-text-primary)
  line-height: 1.3

.team-task-card__summary
  font-size: var(--text-xs)
  color: var(--color-text-tertiary)
  line-height: 1.3

.team-task-card__expand
  color: var(--color-text-tertiary)
  transition: transform 0.2s ease
  cursor: pointer
  flex-shrink: 0

.team-task-card__expand--collapsed
  transform: rotate(-90deg)

.team-task-card__detail
  padding-top: var(--space-2)
  border-top: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)

.team-task-card__avatars
  flex-wrap: nowrap

.team-task-card__mini-avatars
  flex-shrink: 0
  gap: 2px

.team-task-card__mini-initial
  width: 20px
  height: 20px
  border-radius: 50%
  border: 1px solid var(--glass-border)
  margin-left: -6px
  font-size: 9px
  display: flex
  align-items: center
  justify-content: center
  background: var(--glass-elevated, var(--glass-surface))
  color: var(--color-text-secondary)
  flex-shrink: 0

  &:first-child
    margin-left: 0

.team-task-card__mini-more
  font-size: var(--text-xs)
  color: var(--color-text-tertiary)
  margin-left: 2px

.team-task-card__avatar-initial
  width: 22px
  height: 22px
  border-radius: 50%
  background: color-mix(in srgb, var(--color-accent) 10%, var(--glass-surface))
  color: var(--color-accent)
  font-size: 10px
  font-weight: 600
  display: flex
  align-items: center
  justify-content: center

.team-task-card__progress
  margin-top: var(--space-1)

.team-task-card__status-badge
  padding: 1px 6px
  border-radius: 4px
  font-size: 10px
  font-weight: 600
  display: inline-flex
  align-items: center
  gap: 3px
  flex-shrink: 0

.team-task-card__status-badge--grey
  background: color-mix(in srgb, var(--color-text-tertiary) 10%, transparent)
  color: var(--color-text-tertiary)
.team-task-card__status-badge--blue
  background: color-mix(in srgb, var(--color-accent) 15%, transparent)
  color: var(--color-accent)
.team-task-card__status-badge--orange
  background: color-mix(in srgb, var(--color-warning) 12%, transparent)
  color: var(--color-warning)
.team-task-card__status-badge--green
  background: color-mix(in srgb, var(--color-success) 12%, transparent)
  color: var(--color-success)
.team-task-card__status-badge--red
  background: color-mix(in srgb, var(--color-danger) 12%, transparent)
  color: var(--color-danger)

.team-task-card__status-pulse
  width: 5px
  height: 5px
  border-radius: 50%
  background: currentColor
  animation: ttc-pulse 1.5s ease-in-out infinite

.team-task-card__step-count
  font-size: 10px
  color: var(--color-text-tertiary)
  flex-shrink: 0
  margin-left: 4px

.team-task-card__mode-tag
  font-size: 10px
  font-weight: 600
  color: var(--color-text-tertiary)
  background: var(--glass-surface)
  padding: 0 4px
  border-radius: 3px
  flex-shrink: 0
  margin-left: 2px

.team-task-card__status-text--grey
  color: var(--color-text-tertiary)
.team-task-card__status-text--blue
  color: var(--color-accent)
.team-task-card__status-text--orange
  color: var(--color-warning)
.team-task-card__status-text--green
  color: var(--color-success)
.team-task-card__status-text--red
  color: var(--color-danger)

.team-task-card__shared-agent
  color: var(--color-warning)

.team-task-card__error
  display: flex
  align-items: center
  margin-top: 2px
  font-size: var(--text-xs)
  color: var(--color-danger)
  max-width: 100%

@keyframes ttc-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3
</style>
