<template>
  <div class="team-card" :class="[`team-card--${activity.status}`, { 'team-card--expanded': expanded }]">
    <!-- T8.5 / B方案: 单行树形行 — 去除 header/body/footer 多列卡片布局，
         仅保留：左侧状态色条 + 一行信息（图标/名称/任务/状态/时长/操作图标）。
         子节点通过下方 slot 的 ActivityStream 递归渲染，用缩进+左侧连接线表达层级。 -->
    <div class="team-card__row" @click="toggleExpand">
      <span class="team-card__icon">◆</span>
      <span class="team-card__name" :title="displayTeamName">{{ displayTeamName }}</span>
      <span
        v-if="activity.taskSummary && activity.taskSummary !== activity.title"
        class="team-card__task"
        :title="activity.taskSummary"
      >
        {{ activity.taskSummary }}
      </span>
      <span class="team-card__status" :class="`team-card__status--${statusColor}`">
        <span v-if="activity.status === 'running'" class="team-card__pulse" />
        {{ statusText }}
      </span>
      <span v-if="durationText" class="team-card__duration">{{ durationText }}</span>

      <!-- 操作图标：hover 时显示，保持功能可用且不破坏单行视觉 -->
      <span v-if="showFooter" class="team-card__actions">
        <button
          v-if="showPauseButton"
          class="team-card__action team-card__action--pause"
          :title="t('chat.teamStage.pause')"
          @click.stop="$emit('pause-team', activity.teamId || '')"
        >
          <q-icon name="pause" size="12px" />
        </button>
        <button
          v-else-if="showResumeButton"
          class="team-card__action team-card__action--resume"
          :title="t('chat.teamStage.resume')"
          @click.stop="$emit('unpause-team', activity.teamId || '')"
        >
          <q-icon name="play_arrow" size="12px" />
        </button>
        <button
          v-if="showCancelButton"
          class="team-card__action team-card__action--cancel"
          :title="t('chat.teamStage.cancel')"
          @click.stop="$emit('cancel-team', activity.teamId || '')"
        >
          <q-icon name="close" size="12px" />
        </button>
        <button
          v-else-if="activity.status === 'failed'"
          class="team-card__action team-card__action--retry"
          :title="t('chat.teamStage.retry')"
          @click.stop="$emit('retry-team', activity.teamId || '')"
        >
          <q-icon name="refresh" size="12px" />
        </button>
      </span>
    </div>

    <!-- Expanded detail (children rendered by recursive ActivityStream via slot) -->
    <div v-if="expanded" class="team-card__detail" @click.stop>
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { TeamStageEvent } from '../../features/chat/streamEventTypes';
import type { RunStatusValue } from '../../features/chat/types';
import { formatDuration } from '../../features/chat/agentTreeUtils';

const props = defineProps<{
  activity: TeamStageEvent;
  /** P1#1: agent key → display name lookup. */
  agentMap?: Map<string, { displayName: string; agentKey: string }>;
  /** P1#3: parent run status to gate cancel button visibility. */
  runStatus?: RunStatusValue;
  /** T8.6: 点击左侧 Agent 卡片定位时，自动展开目标 Team 卡片。 */
  autoExpand?: boolean;
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
// 用户点击行可展开查看子 Agent 活动。用户手动展开/折叠后状态由用户掌控，
// 不被状态变化自动覆盖（用户意图优先）。
const expanded = ref(false);

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

// T8.6: 外部（如左侧 Agent 卡片点击定位）触发自动展开，加载子活动。
watch(
  () => props.autoExpand,
  (newVal) => {
    if (newVal && !expanded.value) {
      toggleExpand();
    }
  },
);

// B.5.3: pause button visible when running and parent run allows cancel.
const showPauseButton = computed(
  () => props.activity.status === 'running' && props.runStatus === 'running' && Boolean(props.activity.teamId),
);

// B.5.3: resume button visible when paused (regardless of parent run status).
const showResumeButton = computed(() => props.activity.status === 'paused' && Boolean(props.activity.teamId));

// B.5.3: cancel button visible when running or paused.
const showCancelButton = computed(
  () => (props.activity.status === 'running' || props.activity.status === 'paused') && Boolean(props.activity.teamId),
);

// 操作图标仅在有可用动作时渲染。
const showFooter = computed(
  () =>
    (props.activity.status === 'running' || props.activity.status === 'paused' || props.activity.status === 'failed') &&
    Boolean(props.activity.teamId),
);

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
  // T8.5: 树形重构 — 去除 border+background+border-radius，改用左侧连接线
  border-left: 3px solid var(--glass-border)
  padding: 6px 10px 6px 8px
  transition: border-left-color 0.15s ease
  display: flex
  flex-direction: column
  height: auto
  min-height: fit-content
  overflow: visible

  // 覆盖 _entity-pages.sass 中针对 TeamsPage grid 的全局 team-card 样式，
  // 避免这些样式泄漏到 chat 活动流中，破坏树形设计。
  &.team-card
    height: auto
    overflow: visible
    background: transparent
    border-radius: 0
    box-shadow: none
    backdrop-filter: none
    -webkit-backdrop-filter: none
    cursor: default

    &:hover
      transform: none
      box-shadow: none

  &--running
    border-left-color: #00E5FF
  &--paused
    border-left-color: var(--color-warning, #f39c12)
  &--failed
    border-left-color: var(--color-danger)
  &--cancelled
    opacity: 0.7
    border-left-color: var(--color-text-tertiary)
  &--completed
    border-left-color: #4CAF7C

  &__row
    display: flex
    align-items: center
    gap: 8px
    min-width: 0
    cursor: pointer
    padding: 2px 0

  &__icon
    font-size: 12px
    color: var(--color-warning, #E9A23B)
    flex-shrink: 0

  &__name
    font-size: 13px
    font-weight: 600
    color: var(--color-text-primary)
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap
    flex-shrink: 0
    max-width: 220px

  &__task
    font-size: 11px
    color: var(--color-text-secondary)
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap
    flex: 1
    min-width: 0

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

  &__duration
    font-size: 11px
    color: var(--color-text-tertiary)
    font-variant-numeric: tabular-nums
    flex-shrink: 0

  &__pulse
    width: 6px
    height: 6px
    border-radius: 50%
    background: var(--color-accent)
    animation: team-card-pulse 1.5s ease-in-out infinite

  // T8.5 / B方案: 操作图标保持功能，但仅在 hover 时显示，避免破坏单行树形视觉
  &__actions
    display: inline-flex
    align-items: center
    gap: 2px
    opacity: 0
    transition: opacity 0.15s ease

  &:hover &__actions
    opacity: 1

  &__action
    width: 22px
    height: 22px
    display: inline-flex
    align-items: center
    justify-content: center
    border: none
    border-radius: 4px
    background: transparent
    color: var(--color-text-secondary)
    cursor: pointer
    transition: background 0.12s ease, color 0.12s ease

    &:hover
      background: var(--glass-surface-hover)

    &--cancel:hover
      color: var(--color-danger)
    &--retry:hover
      color: var(--color-accent)
    &--pause:hover
      color: var(--color-warning, #f39c12)
    &--resume:hover
      color: var(--color-accent)

  // === Expanded detail — T8.5: 树形缩进 + 左侧连接线 ===
  &__detail
    margin-top: 4px
    margin-left: 14px
    padding-left: 8px
    border-left: 2px solid var(--glass-border)
    display: flex
    flex-direction: column
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
