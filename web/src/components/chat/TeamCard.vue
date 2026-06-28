<template>
  <div class="team-card" :class="`team-card--${activity.status}`">
    <div class="team-card__row">
      <!-- Header (20%) — vertical 1:1:1: team name / task name / created time -->
      <div class="team-card__header" @click="toggleExpand">
        <div class="team-card__name" :title="displayTeamName">{{ displayTeamName }}</div>
        <div v-if="activity.taskSummary" class="team-card__task" :title="activity.taskSummary">
          {{ t('chat.teamStage.taskLabel') }}: {{ activity.taskSummary }}
        </div>
        <div v-if="createdTimeText" class="team-card__time">{{ createdTimeText }}</div>
      </div>

      <!-- Body (60%) — vertical 1:2: members row / progress row (3:1:1) -->
      <div class="team-card__body" @click="toggleExpand">
        <!-- Members row: avatars + names -->
        <div v-if="hasMembers" class="team-card__members">
          <span
            v-for="member in activity.members"
            :key="member.agentKey"
            class="team-card__member"
            :class="`team-card__member--${member.status}`"
            :title="t('chat.teamStage.expandMember', { name: member.agentName || member.agentKey })"
            @click.stop="onMemberClick(member)"
          >
            <span class="team-card__member-avatar">{{ memberInitial(member) }}</span>
            <span class="team-card__member-name">{{ member.agentName || member.agentKey }}</span>
            <span v-if="member.status === 'running'" class="team-card__member-dot team-card__member-dot--running" />
            <span v-else-if="member.status === 'completed'" class="team-card__member-mark">✓</span>
            <span v-else-if="member.status === 'failed'" class="team-card__member-mark team-card__member-mark--fail">✗</span>
          </span>
        </div>
        <div v-else class="team-card__members team-card__members--empty">
          {{ t('chat.teamStage.assembling') }}
        </div>

        <!-- Progress row: progress bar (3) | status (1) | duration (1) -->
        <div class="team-card__progress">
          <div class="team-card__bar">
            <div class="team-card__bar-fill" :style="{ width: `${progressPct}%` }" />
          </div>
          <span class="team-card__status" :class="`team-card__status--${statusColor}`">
            <span v-if="activity.status === 'running'" class="team-card__pulse" />
            {{ statusText }}
          </span>
          <span v-if="durationText" class="team-card__duration">{{ durationText }}</span>
        </div>
      </div>

      <!-- Footer (20%) — inject box + pause/resume buttons -->
      <div class="team-card__footer">
        <div class="team-card__inject" :class="{ 'team-card__inject--expanded': injectExpanded }">
          <input
            v-model="injectText"
            class="team-card__inject-input"
            :placeholder="t('chat.teamStage.supplementPlaceholder')"
            @focus="injectExpanded = true"
            @keyup.enter="onInjectSend"
          />
          <button
            v-if="injectExpanded && injectText.trim()"
            class="team-card__inject-send"
            @click="onInjectSend"
          >
            {{ t('chat.teamStage.send') }}
          </button>
        </div>
        <div class="team-card__actions">
          <button
            v-if="activity.status === 'running'"
            class="team-card__action team-card__action--pause"
            @click.stop="$emit('cancel-team', activity.teamId || '')"
          >
            {{ t('chat.teamStage.pause') }}
          </button>
          <!-- resume button reserved for Phase T3 (interrupted status) -->
        </div>
      </div>
    </div>

    <!-- Expanded detail (children rendered by recursive ActivityStream via slot) -->
    <div v-if="expanded" class="team-card__detail">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { TeamStageEvent, TeamMemberStatus } from '../../features/chat/streamEventTypes';
import { formatDuration } from '../../features/chat/agentTreeUtils';
import { nameInitial } from '../../features/spirit/spiritUi';

const props = defineProps<{
  activity: TeamStageEvent;
}>();

const emit = defineEmits<{
  'expand-member': [payload: { agentKey: string; agentName?: string; teamId?: string }];
  'resume-team': [teamId: string];
  'cancel-team': [teamId: string];
  inject: [payload: { teamId: string; message: string }];
}>();

const { t } = useI18n();

// === Collapse state (B.4.5: running default expanded, terminal default collapsed) ===
// User intent priority: once toggled, status changes do not override.
const expanded = ref(props.activity.status === 'running');
const injectExpanded = ref(false);
const injectText = ref('');

function toggleExpand() {
  expanded.value = !expanded.value;
}

// === Derived display values (all from props, no store dependency — red line #1) ===
const hasMembers = computed(() => Boolean(props.activity.members?.length));

const displayTeamName = computed(() => {
  if (props.activity.title) return props.activity.title;
  switch (props.activity.status) {
    case 'running':
      return t('chat.teamStage.assembling');
    case 'completed':
      return t('chat.teamStage.completed');
    case 'failed':
      return t('chat.teamStage.failed');
    case 'cancelled':
      return t('chat.teamStage.cancelled');
    default:
      return t('chat.teamStage.label');
  }
});

const createdTimeText = computed(() => {
  if (!props.activity.timestamp) return '';
  try {
    const d = new Date(props.activity.timestamp);
    if (Number.isNaN(d.getTime())) return '';
    return `${t('chat.teamStage.createdAt')}: ${d.toLocaleTimeString()}`;
  } catch {
    return '';
  }
});

// Progress: prefer explicit progressPct from meta, else derive from members (B.5.1).
const progressPct = computed(() => {
  if (typeof props.activity.progressPct === 'number') return props.activity.progressPct;
  const members = props.activity.members;
  if (members?.length) {
    const done = members.filter((m) => m.status === 'completed' || m.status === 'failed').length;
    return Math.round((done / members.length) * 100);
  }
  if (props.activity.status === 'completed') return 100;
  return 0;
});

const durationText = computed(() => {
  const ms = props.activity.durationMs;
  if (ms == null || ms <= 0) return '';
  return formatDuration(ms);
});

// Status display: map TeamStageEvent.status → localized text + color bucket.
// Note: 'interrupted' is not in TeamStageEvent.status today (mapped to 'failed'
// by mapActivityStatusToStageStatus in useActivityTimeline). Phase T3 may extend
// the type to surface interrupted for the resume button.
const statusText = computed(() => {
  switch (props.activity.status) {
    case 'running':
      return t('chat.teamStage.executing');
    case 'completed':
      return t('chat.teamStage.completed');
    case 'failed':
      return t('chat.teamStage.failed');
    case 'cancelled':
      return t('chat.teamStage.cancelled');
    default:
      return '';
  }
});

const statusColor = computed(() => {
  switch (props.activity.status) {
    case 'running':
      return 'blue';
    case 'completed':
      return 'green';
    case 'failed':
      return 'red';
    case 'cancelled':
      return 'grey';
    default:
      return 'grey';
  }
});

function memberInitial(member: TeamMemberStatus): string {
  return nameInitial(member.agentName || member.agentKey);
}

function onMemberClick(member: { agentKey: string; agentName?: string }) {
  emit('expand-member', {
    agentKey: member.agentKey,
    agentName: member.agentName,
    teamId: props.activity.teamId,
  });
}

function onInjectSend() {
  const msg = injectText.value.trim();
  if (!msg || !props.activity.teamId) return;
  emit('inject', { teamId: props.activity.teamId, message: msg });
  injectText.value = '';
}
</script>

<style lang="sass" scoped>
.team-card
  border-radius: 10px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
  padding: 8px 10px
  transition: border-color 0.15s ease

  &--running
    border-color: color-mix(in srgb, var(--color-accent) 40%, var(--glass-border))
  &--failed
    border-color: color-mix(in srgb, var(--color-danger) 40%, var(--glass-border))
  &--cancelled
    opacity: 0.7
    border-color: color-mix(in srgb, var(--color-text-tertiary) 30%, var(--glass-border))

  &__row
    display: flex
    gap: 10px
    align-items: stretch

  // === Header (20%) ===
  &__header
    flex: 0 0 20%
    min-width: 0
    display: flex
    flex-direction: column
    gap: 3px
    cursor: pointer
    padding-right: 8px
    border-right: 1px solid var(--glass-border)

  &__name
    font-size: 13px
    font-weight: 600
    color: var(--color-text-primary)
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__task
    font-size: 11px
    color: var(--color-text-secondary)
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__time
    font-size: 10px
    color: var(--color-text-tertiary)
    font-variant-numeric: tabular-nums

  // === Body (60%) ===
  &__body
    flex: 0 0 calc(60% - 20px)
    min-width: 0
    display: flex
    flex-direction: column
    gap: 6px
    cursor: pointer

  &__members
    flex: 1
    display: flex
    flex-wrap: wrap
    gap: 6px
    align-items: center
    min-height: 22px

    &--empty
      font-size: 11px
      color: var(--color-text-tertiary)
      font-style: italic

  &__member
    display: inline-flex
    align-items: center
    gap: 4px
    padding: 2px 6px
    border-radius: 4px
    cursor: pointer
    transition: background 0.12s ease

    &:hover
      background: var(--glass-surface-hover)

  &__member-avatar
    width: 18px
    height: 18px
    border-radius: 50%
    background: var(--glass-elevated, var(--glass-surface))
    display: flex
    align-items: center
    justify-content: center
    font-size: 9px
    font-weight: 600
    color: var(--color-text-secondary)
    flex-shrink: 0

  &__member-name
    font-size: 11px
    color: var(--color-text-primary)
    max-width: 80px
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__member-dot
    width: 6px
    height: 6px
    border-radius: 50%
    flex-shrink: 0
    &--running
      background: var(--color-accent)
      animation: team-card-pulse 1.5s ease-in-out infinite

  &__member-mark
    font-size: 10px
    color: var(--color-success)
    &--fail
      color: var(--color-danger)

  &__progress
    flex: 2
    display: flex
    align-items: center
    gap: 8px
    // 3:1:1 ratio — progress bar : status : duration
    .team-card__bar
      flex: 3
    .team-card__status
      flex: 1
    .team-card__duration
      flex: 1

  &__bar
    height: 4px
    background: var(--glass-border)
    border-radius: 2px
    overflow: hidden

  &__bar-fill
    height: 100%
    border-radius: 2px
    background: var(--color-accent)
    transition: width 0.3s ease

  &__status
    font-size: 11px
    display: inline-flex
    align-items: center
    gap: 4px
    flex-shrink: 0

    &--blue
      color: var(--color-accent)
    &--green
      color: var(--color-success)
    &--red
      color: var(--color-danger)
    &--grey
      color: var(--color-text-tertiary)

  &__pulse
    width: 6px
    height: 6px
    border-radius: 50%
    background: var(--color-accent)
    animation: team-card-pulse 1.5s ease-in-out infinite

  &__duration
    font-size: 11px
    color: var(--color-text-tertiary)
    text-align: right
    font-variant-numeric: tabular-nums
    flex-shrink: 0

  // === Footer (20%) ===
  &__footer
    flex: 0 0 20%
    min-width: 0
    display: flex
    flex-direction: column
    gap: 6px
    padding-left: 8px
    border-left: 1px solid var(--glass-border)

  &__inject
    display: flex
    align-items: center
    gap: 4px
    transition: all 0.15s ease

    &--expanded
      .team-card__inject-input
        flex: 1

  &__inject-input
    flex: 0 1 100%
    min-width: 0
    padding: 3px 6px
    border-radius: 4px
    border: 1px solid var(--glass-border)
    background: var(--glass-elevated, var(--glass-surface))
    color: var(--color-text-primary)
    font-size: 11px
    transition: all 0.15s ease

    &:focus
      border-color: var(--color-accent)
      outline: none
      flex: 1

  &__inject-send
    flex-shrink: 0
    padding: 3px 8px
    border-radius: 4px
    border: none
    background: var(--color-accent)
    color: var(--color-on-accent, white)
    font-size: 11px
    cursor: pointer
    transition: opacity 0.12s ease

    &:hover
      opacity: 0.85

  &__actions
    display: flex
    gap: 4px
    justify-content: flex-end

  &__action
    border: none
    border-radius: 4px
    padding: 2px 8px
    font-size: 11px
    cursor: pointer
    transition: opacity 0.12s ease

    &:hover
      opacity: 0.85

    &--pause
      background: color-mix(in srgb, var(--color-warning) 20%, transparent)
      color: var(--color-warning)
      border: 1px solid color-mix(in srgb, var(--color-warning) 40%, transparent)

  // === Expanded detail ===
  &__detail
    margin-top: 6px
    padding-top: 6px
    border-top: 1px dashed var(--glass-border)
    display: flex
    flex-direction: column
    gap: 4px

@keyframes team-card-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3
</style>
