<template>
  <div class="team-stage-block" :class="`team-stage-block--${activity.status}`">
    <!-- Header -->
    <div class="team-stage-block__header" @click="toggleCollapse">
      <span class="team-stage-block__icon">👥</span>
      <span class="team-stage-block__label">{{ t('chat.teamStage.label') }}</span>
      <span class="team-stage-block__title">{{ displayTitle }}</span>
      <span v-if="progressText" class="team-stage-block__progress">{{ progressText }}</span>
      <span class="team-stage-block__status" :class="`team-stage-block__status--${activity.status}`">
        {{ statusIcon }}
      </span>
      <span
        v-if="activity.members?.length && activity.status !== 'running'"
        class="team-stage-block__chevron"
        :class="{ 'team-stage-block__chevron--expanded': !collapsed }"
      >
        ▸
      </span>
    </div>

    <!-- Duration -->
    <div v-if="activity.durationMs != null && isTerminal" class="team-stage-block__duration">
      {{ formatDuration(activity.durationMs) }}
    </div>

    <!-- Task summary -->
    <div v-if="activity.taskSummary" class="team-stage-block__summary">
      {{ activity.taskSummary }}
    </div>

    <!-- Members list (collapsible after terminal state) -->
    <div v-if="showMembers" class="team-stage-block__members">
      <div
        v-for="member in activity.members"
        :key="member.agentKey"
        class="team-stage-block__member team-stage-block__member--clickable"
        :class="`team-stage-block__member--${member.status}`"
        :title="t('chat.teamStage.expandMember', { name: member.agentName || member.agentKey })"
        @click="onMemberClick(member)"
      >
        <span class="team-stage-block__member-dot" :class="`team-stage-block__member-dot--${member.status}`">
          <span v-if="member.status === 'running'" class="team-stage-block__pulse" />
        </span>
        <span class="team-stage-block__member-name">{{ member.agentName || member.agentKey }}</span>
        <span v-if="member.status === 'running'" class="team-stage-block__member-badge">
          {{ t('chat.teamStage.executing') }}
        </span>
        <q-icon name="chevron_right" size="14px" class="team-stage-block__member-chevron" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { TeamStageEvent } from '../../features/chat/streamEventTypes';
import { formatDuration } from '../../features/chat/agentTreeUtils';

const props = defineProps<{
  activity: TeamStageEvent;
}>();

const emit = defineEmits<{
  'expand-member': [payload: { agentKey: string; agentName?: string }];
}>();

const { t } = useI18n();

/** Phase B-4 / §9.1.3: clicking a member row emits expand-member so the
 *  parent (ChatPage) can switch to member mode and lazy-load that member's
 *  session activities. Payload is intentionally minimal — the member session
 *  id is resolved later via the session tree (useChatWorkspace watcher). */
function onMemberClick(member: { agentKey: string; agentName?: string }) {
  emit('expand-member', { agentKey: member.agentKey, agentName: member.agentName });
}

const collapsed = ref(props.activity.status !== 'running');

function toggleCollapse() {
  // Only allow toggling after terminal state (running state always expanded)
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
  // Prefer explicit title; otherwise derive from status
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
      return '';
  }
});

const progressText = computed(() => {
  const members = props.activity.members;
  if (!members?.length) return '';
  const completed = members.filter((m) => m.status === 'completed' || m.status === 'failed').length;
  return `${completed}/${members.length}`;
});

const showMembers = computed(() => {
  if (!props.activity.members?.length) return false;
  // Always show during running; collapsible after terminal
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
      return '👥';
  }
});
</script>

<style lang="sass" scoped>
.team-stage-block
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

  &__summary
    font-size: 12px
    color: var(--color-text-secondary)
    margin-top: 6px
    line-height: 1.4

  &__members
    display: flex
    flex-direction: column
    gap: 4px
    margin-top: 6px
    padding-left: 22px

  &__member
    display: flex
    align-items: center
    gap: 6px
    padding: 2px 0

  &__member--clickable
    cursor: pointer

    &:hover
      background: var(--glass-surface)
      border-radius: 4px

  &__member-dot
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

  &__member-name
    font-size: 12px
    color: var(--color-text-primary)
    flex: 1

  &__member-badge
    font-size: 11px
    color: var(--color-accent)

  &__member-chevron
    color: var(--color-text-tertiary)
    margin-left: auto

  &__pulse
    position: absolute
    inset: -3px
    border-radius: 50%
    border: 1.5px solid var(--color-accent)
    animation: team-pulse 1.5s ease-in-out infinite

@keyframes team-pulse
  0%, 100%
    opacity: 1
    transform: scale(1)
  50%
    opacity: 0.3
    transform: scale(1.4)
</style>
