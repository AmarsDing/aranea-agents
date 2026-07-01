<template>
  <div class="team-card" :class="[`team-card--${activity.status}`, { 'team-card--expanded': expanded }]">
    <div class="team-card__row">
      <!-- Header (20%) — vertical 1:1:1: team name / task name / created time -->
      <div class="team-card__header" @click="toggleExpand">
        <div class="team-card__name" :title="displayTeamName">{{ displayTeamName }}</div>
        <div v-if="activity.taskSummary" class="team-card__task" :title="activity.taskSummary">
          {{ t('chat.teamStage.taskLabel') }}: {{ activity.taskSummary }}
        </div>
        <div v-if="createdTimeText" class="team-card__time">{{ createdTimeText }}</div>
      </div>

      <!-- Body (60% when footer present, flex-fill when footer hidden) — vertical 1:2: members row / progress row (3:1:1) -->
      <div class="team-card__body" :class="{ 'team-card__body--wide': !showFooter }" @click="toggleExpand">
        <!-- Members row: avatars + names -->
        <div v-if="hasMembers" class="team-card__members">
          <span
            v-for="member in activity.members"
            :key="member.agentKey"
            class="team-card__member"
            :class="`team-card__member--${member.status}`"
            :title="t('chat.teamStage.expandMember', { name: memberDisplayName(member) })"
            @click.stop="onMemberClick(member)"
          >
            <span class="team-card__member-avatar">{{ memberInitial(member) }}</span>
            <span class="team-card__member-name">{{ memberDisplayName(member) }}</span>
            <span v-if="member.status === 'running'" class="team-card__member-dot team-card__member-dot--running" />
            <span v-else-if="member.status === 'completed'" class="team-card__member-mark">✓</span>
            <span v-else-if="member.status === 'failed'" class="team-card__member-mark team-card__member-mark--fail"
              >✗</span
            >
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

      <!-- Footer (20%) — pause/resume + cancel/retry buttons + inject dialog (B.4.1/B.5.3).
           Hidden for states that have no actionable controls (completed/cancelled),
           so an empty column with a border does not sit at the card end. -->
      <div v-if="showFooter" class="team-card__footer">
        <div class="team-card__actions">
          <button
            v-if="showPauseButton"
            class="team-card__action team-card__action--pause"
            @click.stop="$emit('pause-team', activity.teamId || '')"
          >
            <q-icon name="pause" size="12px" class="q-mr-xs" />
            {{ t('chat.teamStage.pause') }}
          </button>
          <button
            v-else-if="showResumeButton"
            class="team-card__action team-card__action--resume"
            @click.stop="$emit('unpause-team', activity.teamId || '')"
          >
            <q-icon name="play_arrow" size="12px" class="q-mr-xs" />
            {{ t('chat.teamStage.resume') }}
          </button>
          <button
            v-if="showCancelButton"
            class="team-card__action team-card__action--cancel"
            @click.stop="$emit('cancel-team', activity.teamId || '')"
          >
            <q-icon name="close" size="12px" class="q-mr-xs" />
            {{ t('chat.teamStage.cancel') }}
          </button>
          <button
            v-else-if="activity.status === 'failed'"
            class="team-card__action team-card__action--retry"
            @click.stop="$emit('retry-team', activity.teamId || '')"
          >
            <q-icon name="refresh" size="12px" class="q-mr-xs" />
            {{ t('chat.teamStage.retry') }}
          </button>
          <!-- completed/cancelled: hide buttons -->
        </div>

        <!-- Inject dialog (B.5.3) — inside footer, visible only when running or paused -->
        <div v-if="showInjectDialog" class="team-card__inject">
          <input
            v-model="injectMessage"
            type="text"
            class="team-card__inject-input"
            :placeholder="t('chat.teamStage.injectPlaceholder')"
            @keyup.enter="onInject"
          />
          <button
            class="team-card__inject-send"
            :disabled="!injectMessage.trim()"
            @click.stop="onInject"
          >
            <q-icon name="send" size="12px" />
          </button>
        </div>
      </div>
    </div>

    <!-- Expanded detail (children rendered by recursive ActivityStream via slot) -->
    <!-- @click.stop prevents clicks inside member agent-cards from bubbling up
         to team-card__body and collapsing the team while the user is interacting
         with an agent's activities. -->
    <div v-if="expanded" class="team-card__detail" @click.stop>
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { TeamStageEvent, TeamMemberStatus } from '../../features/chat/streamEventTypes';
import type { RunStatusValue } from '../../features/chat/types';
import { formatDuration } from '../../features/chat/agentTreeUtils';
import { nameInitial } from '../../features/spirit/spiritUi';

const props = defineProps<{
  activity: TeamStageEvent;
  /** P1#1: agent key → display name lookup. */
  agentMap?: Map<string, { displayName: string; agentKey: string }>;
  /** P1#3: parent run status to gate cancel button visibility. */
  runStatus?: RunStatusValue;
}>();

const emit = defineEmits<{
  'expand-member': [payload: { agentKey: string; agentName?: string; teamId?: string }];
  'cancel-team': [teamId: string];
  'retry-team': [teamId: string];
  // B.5.3: pause/unpause/inject events for team run lifecycle control.
  'pause-team': [teamId: string];
  'unpause-team': [teamId: string];
  'inject-team': [payload: { teamId: string; message: string }];
  // T5.2 / §B.7.2: Fired when the team-card expands so the parent can
  // lazy-load each member's worker session activities. Payload is the list
  // of member session_ids (worker sessions) — already cached sessions are
  // skipped by `ensureActivitiesLoaded` (T5.4).
  expand: [sessionIds: string[]];
}>();

const { t } = useI18n();

// === Collapse state (B.4.5: team-card 始终默认折叠，含 running 与终态) ===
// 设计依据：多个 team 同时展示时默认折叠（高度 100px）让用户聚焦于当前关注的 team；
// 用户点击 header 可展开查看完整进度与成员。用户手动展开/折叠后状态由用户掌控，
// 不被状态变化自动覆盖（用户意图优先）。
const expanded = ref(false);

// === Inject dialog state (B.5.3) ===
const injectMessage = ref('');

function toggleExpand() {
  expanded.value = !expanded.value;
  // T5.2: On expand, request lazy-load of member worker sessions.
  if (expanded.value) {
    const sessionIds = (props.activity.members ?? [])
      .map((m) => m.session_id)
      .filter((id): id is string => Boolean(id));
    if (sessionIds.length > 0) {
      emit('expand', sessionIds);
    }
  }
}

function onInject() {
  const message = injectMessage.value.trim();
  if (!message || !props.activity.teamId) return;
  emit('inject-team', { teamId: props.activity.teamId, message });
  injectMessage.value = '';
}

// === Derived display values (all from props, no store dependency — red line #1) ===
const hasMembers = computed(() => Boolean(props.activity.members?.length));

// B.5.3: pause button visible when running and parent run allows cancel.
const showPauseButton = computed(
  () =>
    props.activity.status === 'running' &&
    props.runStatus === 'running' &&
    Boolean(props.activity.teamId),
);

// B.5.3: resume button visible when paused (regardless of parent run status,
// since paused is a self-contained state).
const showResumeButton = computed(
  () => props.activity.status === 'paused' && Boolean(props.activity.teamId),
);

// B.5.3: cancel button visible when running or paused (user can cancel from
// either state). Failed state shows retry instead.
const showCancelButton = computed(
  () =>
    (props.activity.status === 'running' || props.activity.status === 'paused') &&
    Boolean(props.activity.teamId),
);

// B.5.3: inject dialog visible when running or paused.
const showInjectDialog = computed(
  () =>
    (props.activity.status === 'running' || props.activity.status === 'paused') &&
    Boolean(props.activity.teamId),
);

// Footer is only rendered when there is at least one actionable control.
// completed/cancelled teams have no pause/resume/cancel/retry/inject UI,
// so the empty footer column (with its left border) is hidden to avoid an
// orphaned visual control at the card end.
const showFooter = computed(
  () =>
    (props.activity.status === 'running' ||
      props.activity.status === 'paused' ||
      props.activity.status === 'failed') &&
    Boolean(props.activity.teamId),
);

function memberDisplayName(member: TeamMemberStatus): string {
  if (member.agentName) return member.agentName;
  const fromStore = props.agentMap?.get(member.agentKey)?.displayName;
  if (fromStore) return fromStore;
  return readableAgentKey(member.agentKey);
}

function readableAgentKey(key: string): string {
  return key
    .replace(/^agent___?/, '')
    .replace(/_/g, ' ')
    .trim();
}

function memberInitial(member: TeamMemberStatus): string {
  return nameInitial(memberDisplayName(member));
}

function onMemberClick(member: TeamMemberStatus) {
  // Issue 1 fix: emit 'expand' to lazy-load member session activities,
  // then expand the team card to show them inline. Previously emitted
  // 'expand-member' which triggered ChatPage.onExpandMember →
  // spiritStore.selectMember → page navigation.
  const memberSessionId = member.session_id;
  if (memberSessionId) {
    emit('expand', [memberSessionId]);
  }
  expanded.value = true;
}

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
  // Status-based override: a completed team is always 100%, even if
  // meta.progress_pct is stale (e.g. assembled event set 0 and the terminal
  // event's progress_pct=100 was lost in dedup). This must be checked BEFORE
  // the meta.progress_pct branch below, otherwise completed teams show 0%.
  if (props.activity.status === 'completed') return 100;
  if (props.activity.status === 'failed' || props.activity.status === 'cancelled') return 0;
  if (typeof props.activity.progressPct === 'number') return props.activity.progressPct;
  const members = props.activity.members;
  if (members?.length) {
    const done = members.filter((m) => m.status === 'completed' || m.status === 'failed').length;
    return Math.round((done / members.length) * 100);
  }
  return 0;
});

const durationText = computed(() => {
  const ms = props.activity.durationMs;
  if (ms == null || ms <= 0) return '';
  return formatDuration(ms);
});

// Status display: map TeamStageEvent.status → localized text + color bucket.
// Note: 'interrupted' ActivityStatus is mapped to 'failed' by
// mapActivityStatusToStageStatus, so failed status covers both failed and
// interrupted cases — retry button shows for both (B.4.1/B.5.2).
const statusText = computed(() => {
  switch (props.activity.status) {
    case 'running':
      return t('chat.teamStage.executing');
    case 'paused':
      return t('chat.teamStage.paused');
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
    case 'paused':
      return 'orange';
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
</script>

<style lang="sass" scoped>
.team-card
  border-radius: 10px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
  padding: 8px 10px
  transition: border-color 0.15s ease
  // B.4.1: 默认折叠时仍要完整展示头部/中部/尾部（含操作按钮与注入对话框），
  // 因此不限制高度；展开态显示成员子活动。
  // 必须使用 flex column 否则展开后的 team-card__detail 会溢出父容器高度，
  // 导致后续兄弟 team-card 与其内容重叠。
  // height: auto 覆盖全局 TeamsPage grid 的 230px 固定高度（_entity-pages.sass
  // 的 .team-card 选择器与 chat TeamCard 类名冲突）。
  display: flex
  flex-direction: column
  height: auto
  min-height: fit-content
  overflow: visible

  &--running
    border-color: color-mix(in srgb, var(--color-accent) 40%, var(--glass-border))
  &--paused
    border-color: color-mix(in srgb, var(--color-warning, #f39c12) 40%, var(--glass-border))
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

  // === Body (60% when footer present, flex-fill when footer hidden) ===
  &__body
    flex: 0 0 calc(60% - 20px)
    min-width: 0
    display: flex
    flex-direction: column
    gap: 6px
    cursor: pointer

    &--wide
      flex: 1

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
    &--orange
      color: var(--color-warning, #f39c12)
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
    justify-content: flex-start

  &__actions
    display: flex
    flex-direction: column
    gap: 4px
    align-items: stretch
    width: 100%

  &__action
    border: none
    border-radius: 4px
    padding: 4px 8px
    font-size: 11px
    cursor: pointer
    transition: opacity 0.12s ease
    display: inline-flex
    align-items: center
    justify-content: center

    &:hover
      opacity: 0.85

    &--cancel
      background: color-mix(in srgb, var(--color-danger) 15%, transparent)
      color: var(--color-danger)
      border: 1px solid color-mix(in srgb, var(--color-danger) 40%, transparent)

    &--retry
      background: color-mix(in srgb, var(--color-accent) 15%, transparent)
      color: var(--color-accent)
      border: 1px solid color-mix(in srgb, var(--color-accent) 40%, transparent)

    &--pause
      background: color-mix(in srgb, var(--color-warning, #f39c12) 15%, transparent)
      color: var(--color-warning, #f39c12)
      border: 1px solid color-mix(in srgb, var(--color-warning, #f39c12) 40%, transparent)

    &--resume
      background: color-mix(in srgb, var(--color-accent) 15%, transparent)
      color: var(--color-accent)
      border: 1px solid color-mix(in srgb, var(--color-accent) 40%, transparent)

  // === Inject dialog (B.5.3) ===
  &__inject
    display: flex
    gap: 6px
    align-items: center
    width: 100%
    padding-top: 4px
    border-top: 1px dashed var(--glass-border)

  &__inject-input
    flex: 1
    min-width: 0
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)
    border-radius: 4px
    padding: 4px 8px
    font-size: 11px
    color: var(--color-text-primary)
    outline: none
    transition: border-color 0.12s ease

    &:focus
      border-color: var(--color-accent)

    &::placeholder
      color: var(--color-text-tertiary)

  &__inject-send
    flex-shrink: 0
    border: none
    border-radius: 4px
    padding: 4px 8px
    background: var(--color-accent)
    color: white
    cursor: pointer
    transition: opacity 0.12s ease
    display: inline-flex
    align-items: center
    justify-content: center

    &:hover:not(:disabled)
      opacity: 0.85

    &:disabled
      opacity: 0.4
      cursor: not-allowed

  // === Expanded detail ===
  &__detail
    margin-top: 6px
    padding-top: 6px
    border-top: 1px dashed var(--glass-border)
    display: flex
    flex-direction: column
    // Cap the expanded panel height so many member agent-cards do not push
    // the rest of the stream too far. Each agent-card already scrolls its own
    // detail internally; the team panel itself scrolls when members exceed it.
    max-height: 720px
    overflow-y: auto
    overflow-x: hidden
    gap: 4px

@keyframes team-card-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3
</style>
