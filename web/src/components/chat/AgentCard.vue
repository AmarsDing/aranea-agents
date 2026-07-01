<template>
  <div class="agent-card" :class="[`agent-card--${activity.status}`, { 'agent-card--blocked': isBlocked }]">
    <!-- T8.5 / B方案: 单行树形行 — 左侧状态色条 + 头像/名称/状态/时间/操作图标一行。
         inject 输入保持功能，但折叠为行下方的紧凑单行。 -->
    <div class="agent-card__row" @click="toggleExpand">
      <span class="agent-card__avatar">{{ agentInitial }}</span>
      <span class="agent-card__name" :title="displayAgentName">{{ displayAgentName }}</span>
      <span class="agent-card__status-badge" :class="`agent-card__status-badge--${statusColor}`">
        <span v-if="activity.status === 'running' && !isBlocked" class="agent-card__pulse" />
        {{ statusText }}
      </span>
      <span v-if="createdTimeText" class="agent-card__time">{{ createdTimeText }}</span>

      <span v-if="showFooter" class="agent-card__actions">
        <button
          v-if="showPauseButton"
          class="agent-card__action agent-card__action--pause"
          :title="t('chat.sessionStage.pause')"
          @click.stop="$emit('pause-agent', activity.childSessionId || '')"
        >
          <q-icon name="pause" size="12px" />
        </button>
        <button
          v-else-if="showResumeButton"
          class="agent-card__action agent-card__action--resume"
          :title="t('chat.sessionStage.resume')"
          @click.stop="$emit('resume-agent', activity.childSessionId || '')"
        >
          <q-icon name="play_arrow" size="12px" />
        </button>
        <button
          v-if="showCancelButton"
          class="agent-card__action agent-card__action--cancel"
          :title="t('chat.sessionStage.cancel')"
          @click.stop="$emit('cancel-agent', activity.childSessionId || '')"
        >
          <q-icon name="close" size="12px" />
        </button>
        <button
          v-else-if="activity.status === 'failed'"
          class="agent-card__action agent-card__action--retry"
          :title="t('chat.sessionStage.retry')"
          @click.stop="$emit('retry-agent', activity.childSessionId || '')"
        >
          <q-icon name="refresh" size="12px" />
        </button>
      </span>
    </div>

    <!-- Inject dialog (B.5.3) — 保持功能，单行紧凑布局 -->
    <div v-if="showInjectDialog" class="agent-card__inject" @click.stop>
      <input
        v-model="injectMessage"
        type="text"
        class="agent-card__inject-input"
        :placeholder="t('chat.sessionStage.injectPlaceholder')"
        @keyup.enter="onInject"
      />
      <button class="agent-card__inject-send" :disabled="!injectMessage.trim()" @click.stop="onInject">
        <q-icon name="send" size="12px" />
      </button>
    </div>

    <!-- Expanded detail (children rendered by recursive ActivityStream) -->
    <div v-if="expanded" class="agent-card__detail" @click.stop>
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SessionStageEvent } from '../../features/chat/streamEventTypes';
import type { RunStatusValue } from '../../features/chat/types';
import type { BlockedResult } from '../../features/chat/composables/useBlockedStatus';
import { nameInitial } from '../../features/spirit/spiritUi';

const props = defineProps<{
  activity: SessionStageEvent;
  /** P1#2: agent key → display name lookup. */
  agentMap?: Map<string, { displayName: string; agentKey: string }>;
  /** P1#3: parent run status to gate cancel button visibility. */
  runStatus?: RunStatusValue;
  /** T8.8: 阻塞检测结果（由 ActivityStream 通过 findBlockedInTree 计算后传入）。
   *  当子活动存在阻塞（tool/confirm/llm）时，AgentCard 左边框变黄 + 显示 ⚠ 阻塞标签。 */
  blockedInfo?: BlockedResult;
  /** T8.6: 点击左侧 Agent 卡片定位时，自动展开目标 Agent 卡片。 */
  autoExpand?: boolean;
}>();

const emit = defineEmits<{
  'cancel-agent': [sessionId: string];
  'retry-agent': [sessionId: string];
  // B.5.3: pause/resume/inject events for sub-agent session lifecycle control.
  'pause-agent': [sessionId: string];
  'resume-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  // T5.3 / §B.7.2: Fired when the agent-card expands so the parent can
  // lazy-load the agent's child session activities. Payload is a single-element
  // list containing childSessionId (when present) — already-cached sessions are
  // skipped by `ensureActivitiesLoaded` (T5.4). Mirrors TeamCard's `expand` emit.
  expand: [sessionIds: string[]];
}>();

const { t } = useI18n();

// === Collapse state (B.4.5: agent-card 始终默认折叠，与 team-card 一致) ===
const expanded = ref(false);

// === Inject dialog state (B.5.3) ===
const injectMessage = ref('');

function toggleExpand() {
  expanded.value = !expanded.value;
  // T5.3: On expand, request lazy-load of the agent's child session.
  if (expanded.value && props.activity.childSessionId) {
    emit('expand', [props.activity.childSessionId]);
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

function onInject() {
  const message = injectMessage.value.trim();
  if (!message || !props.activity.childSessionId) return;
  emit('inject-agent', { sessionId: props.activity.childSessionId, message });
  injectMessage.value = '';
}

// === Derived display values (all from props, no store dependency — red line #1) ===
const isSystemAgent = computed(() => isSystemAgentKey(props.activity.agentKey));

// T8.8: 阻塞状态 — 子活动存在 tool/confirm/llm 阻塞时，覆盖 running 的蓝色显示。
const isBlocked = computed(() => props.blockedInfo?.blocked === true);

// B.5.3: pause button visible when running and parent run allows cancel.
// System agents (run-service/session-service status notices) never get pause.
const showPauseButton = computed(
  () =>
    !isSystemAgent.value &&
    props.activity.status === 'running' &&
    props.runStatus === 'running' &&
    Boolean(props.activity.childSessionId),
);

// B.5.3: resume button visible when paused (regardless of parent run status,
// since paused is a self-contained state).
const showResumeButton = computed(
  () => !isSystemAgent.value && props.activity.status === 'paused' && Boolean(props.activity.childSessionId),
);

// B.5.3: cancel button visible when running or paused (user can cancel from
// either state). Failed state shows retry instead.
const showCancelButton = computed(
  () =>
    !isSystemAgent.value &&
    (props.activity.status === 'running' || props.activity.status === 'paused') &&
    Boolean(props.activity.childSessionId),
);

// B.5.3: inject dialog visible when running or paused.
const showInjectDialog = computed(
  () =>
    !isSystemAgent.value &&
    (props.activity.status === 'running' || props.activity.status === 'paused') &&
    Boolean(props.activity.childSessionId),
);

// Footer is only rendered when there is at least one actionable control.
// completed/cancelled agents have no pause/resume/cancel/retry/inject UI,
// so the empty footer column (with its left border) is hidden to avoid an
// orphaned visual control at the card end.
const showFooter = computed(
  () =>
    !isSystemAgent.value &&
    (props.activity.status === 'running' || props.activity.status === 'paused' || props.activity.status === 'failed') &&
    Boolean(props.activity.childSessionId),
);

// System agent keys are infrastructure agents (orchestrator/memory/skills/admin)
// that don't have their own worker sessions and shouldn't show pause/cancel/inject.
// Note: department-lead agents (`__dept_lead_*__`) are REAL team members seeded
// by the system but acting as regular agents — they MUST keep their buttons.
const SYSTEM_AGENT_KEYS = new Set(['__spirit__', '__memory__', '__skills__', '__system_admin__']);

function isSystemAgentKey(key: string | undefined): boolean {
  if (!key) return false;
  return SYSTEM_AGENT_KEYS.has(key);
}

function readableAgentKey(key: string | undefined): string {
  if (!key) return '';
  return key
    .replace(/^agent___?/, '')
    .replace(/_/g, ' ')
    .trim();
}

const displayAgentName = computed(() => {
  // P1#2: system agents use the backend-provided agentName; never show raw key.
  if (isSystemAgent.value) {
    return props.activity.agentName || t('chat.sessionStage.systemStatus');
  }
  if (props.activity.agentName) return props.activity.agentName;
  const fromStore = props.agentMap?.get(props.activity.agentKey || '')?.displayName;
  if (fromStore) return fromStore;
  const readable = readableAgentKey(props.activity.agentKey);
  if (readable) return readable;
  if (props.activity.title) return props.activity.title;
  return t('chat.sessionStage.systemStatus');
});

const agentInitial = computed(() => nameInitial(displayAgentName.value));

const createdTimeText = computed(() => {
  if (!props.activity.timestamp) return '';
  try {
    const d = new Date(props.activity.timestamp);
    if (Number.isNaN(d.getTime())) return '';
    return d.toLocaleTimeString();
  } catch {
    return '';
  }
});

// Status display: map SessionStageEvent.status → localized text + color bucket.
// Note: 'interrupted' ActivityStatus is mapped to 'failed' by
// mapActivityStatusToStageStatus, so failed status covers both failed and
// interrupted cases — retry button shows for both (B.4.2/B.5.2).
// T8.8: 阻塞状态覆盖 running 显示 — 子活动阻塞时显示 ⚠ 阻塞 + 黄色。
const statusText = computed(() => {
  const who = displayAgentName.value || t('chat.sessionStage.systemStatus');
  if (isBlocked.value) {
    return `⚠ 阻塞 · ${props.blockedInfo?.message || ''}`;
  }
  switch (props.activity.status) {
    case 'running':
      return t('chat.sessionStage.executing', { name: who });
    case 'paused':
      return t('chat.sessionStage.paused', { name: who });
    case 'completed':
      return t('chat.sessionStage.completed', { name: who });
    case 'failed':
      return t('chat.sessionStage.failed', { name: who });
    case 'cancelled':
      return t('chat.sessionStage.cancelled', { name: who });
    default:
      return '';
  }
});

const statusColor = computed(() => {
  if (isBlocked.value) return 'orange';
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
.agent-card
  // T8.5: 树形重构 — 去除 border+background+border-radius，改用左侧连接线
  border-left: 3px solid var(--glass-border)
  padding: 4px 10px 4px 8px
  transition: border-left-color 0.15s ease
  display: flex
  flex-direction: column
  height: auto
  min-height: fit-content

  // 覆盖 _entity-pages.sass 中针对 AgentsPage grid 的全局 agent-card 样式，
  // 避免这些样式泄漏到 chat 活动流中，破坏树形设计。
  &.agent-card
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
  // T8.8: 阻塞状态 — 黄色左边框 + 脉冲发光，覆盖 running 蓝色
  &--blocked
    border-left-color: #E9A23B
    animation: agent-card-stuck-pulse 2s infinite

  &__row
    display: flex
    align-items: center
    gap: 8px
    min-width: 0
    cursor: pointer
    padding: 2px 0

  &__avatar
    width: 20px
    height: 20px
    border-radius: 50%
    background: var(--glass-elevated, var(--glass-surface))
    display: flex
    align-items: center
    justify-content: center
    font-size: 10px
    font-weight: 600
    color: var(--color-text-secondary)
    flex-shrink: 0

  &__name
    font-size: 13px
    font-weight: 500
    color: var(--color-text-primary)
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap
    flex: 1
    min-width: 0

  &__status-badge
    font-size: 11px
    font-weight: 500
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

  &__time
    font-size: 10px
    color: var(--color-text-tertiary)
    font-variant-numeric: tabular-nums
    flex-shrink: 0

  &__pulse
    width: 6px
    height: 6px
    border-radius: 50%
    background: var(--color-accent)
    animation: agent-card-pulse 1.5s ease-in-out infinite

  // T8.5 / B方案: 操作图标 hover 显示，保持功能
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

  // === Inject dialog (B.5.3) — 紧凑单行 ===
  &__inject
    display: flex
    gap: 6px
    align-items: center
    margin-top: 4px
    margin-left: 28px
    padding: 2px 0

  &__inject-input
    flex: 1
    min-width: 0
    background: transparent
    border: none
    border-bottom: 1px solid var(--glass-border)
    padding: 2px 0
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
    padding: 2px 6px
    background: transparent
    color: var(--color-accent)
    cursor: pointer
    transition: background 0.12s ease
    display: inline-flex
    align-items: center
    justify-content: center

    &:hover:not(:disabled)
      background: var(--glass-surface-hover)

    &:disabled
      opacity: 0.4
      cursor: not-allowed

  // === Expanded detail — T8.5: 树形缩进 + 左侧连接线 ===
  &__detail
    margin-top: 4px
    margin-left: 14px
    padding-left: 8px
    border-left: 2px solid var(--glass-border)
    display: flex
    flex-direction: column
    gap: 4px
    max-height: 480px
    overflow-y: auto
    overflow-x: hidden

@keyframes agent-card-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3

// T8.8: 阻塞状态黄色脉冲发光
@keyframes agent-card-stuck-pulse
  0%, 100%
    box-shadow: 0 0 0 0 rgba(233, 162, 59, 0)
  50%
    box-shadow: 0 0 6px 0 rgba(233, 162, 59, 0.35)
</style>
