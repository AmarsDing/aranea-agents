<template>
  <div class="agent-card" :class="`agent-card--${activity.status}`">
    <div class="agent-card__row">
      <!-- Header — avatar + name + status badge + created time.
           B.4.2: the entire header is clickable for inline expansion; no
           standalone expand icon is rendered (design shows only avatar/name/
           badge/time in the header row). -->
      <div class="agent-card__header" :class="{ 'agent-card__header--full': !showFooter }" @click="toggleExpand">
        <span class="agent-card__avatar">{{ agentInitial }}</span>
        <span class="agent-card__name" :title="displayAgentName">{{ displayAgentName }}</span>
        <span class="agent-card__status-badge" :class="`agent-card__status-badge--${statusColor}`">
          <span v-if="activity.status === 'running'" class="agent-card__pulse" />
          {{ statusText }}
        </span>
        <span v-if="createdTimeText" class="agent-card__time">{{ createdTimeText }}</span>
      </div>

      <!-- Footer (20%) — pause/resume + cancel/retry buttons + inject dialog (B.4.2/B.5.3).
           Hidden for states that have no actionable controls (completed/cancelled),
           so an empty column with a border does not sit at the card end. -->
      <div v-if="showFooter" class="agent-card__footer">
        <div class="agent-card__actions">
          <button
            v-if="showPauseButton"
            class="agent-card__action agent-card__action--pause"
            @click.stop="$emit('pause-agent', activity.childSessionId || '')"
          >
            <q-icon name="pause" size="12px" class="q-mr-xs" />
            {{ t('chat.sessionStage.pause') }}
          </button>
          <button
            v-else-if="showResumeButton"
            class="agent-card__action agent-card__action--resume"
            @click.stop="$emit('resume-agent', activity.childSessionId || '')"
          >
            <q-icon name="play_arrow" size="12px" class="q-mr-xs" />
            {{ t('chat.sessionStage.resume') }}
          </button>
          <button
            v-if="showCancelButton"
            class="agent-card__action agent-card__action--cancel"
            @click.stop="$emit('cancel-agent', activity.childSessionId || '')"
          >
            <q-icon name="close" size="12px" class="q-mr-xs" />
            {{ t('chat.sessionStage.cancel') }}
          </button>
          <button
            v-else-if="activity.status === 'failed'"
            class="agent-card__action agent-card__action--retry"
            @click.stop="$emit('retry-agent', activity.childSessionId || '')"
          >
            <q-icon name="refresh" size="12px" class="q-mr-xs" />
            {{ t('chat.sessionStage.retry') }}
          </button>
          <!-- completed/cancelled: hide buttons -->
        </div>

        <!-- Inject dialog (B.5.3) — inside footer, visible only when running or paused -->
        <div v-if="showInjectDialog" class="agent-card__inject">
          <input
            v-model="injectMessage"
            type="text"
            class="agent-card__inject-input"
            :placeholder="t('chat.sessionStage.injectPlaceholder')"
            @keyup.enter="onInject"
          />
          <button
            class="agent-card__inject-send"
            :disabled="!injectMessage.trim()"
            @click.stop="onInject"
          >
            <q-icon name="send" size="12px" />
          </button>
        </div>
      </div>
    </div>

    <!-- Expanded detail (children rendered by recursive ActivityStream) -->
    <!-- @click.stop keeps nested interactive children from bubbling up to the
         parent stage card's own toggle handler. -->
    <div v-if="expanded" class="agent-card__detail" @click.stop>
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SessionStageEvent } from '../../features/chat/streamEventTypes';
import type { RunStatusValue } from '../../features/chat/types';
import { nameInitial } from '../../features/spirit/spiritUi';

const props = defineProps<{
  activity: SessionStageEvent;
  /** P1#2: agent key → display name lookup. */
  agentMap?: Map<string, { displayName: string; agentKey: string }>;
  /** P1#3: parent run status to gate cancel button visibility. */
  runStatus?: RunStatusValue;
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

function onInject() {
  const message = injectMessage.value.trim();
  if (!message || !props.activity.childSessionId) return;
  emit('inject-agent', { sessionId: props.activity.childSessionId, message });
  injectMessage.value = '';
}

// === Derived display values (all from props, no store dependency — red line #1) ===
const isSystemAgent = computed(() => isSystemAgentKey(props.activity.agentKey));

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
  () =>
    !isSystemAgent.value &&
    props.activity.status === 'paused' &&
    Boolean(props.activity.childSessionId),
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
    (props.activity.status === 'running' ||
      props.activity.status === 'paused' ||
      props.activity.status === 'failed') &&
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
const statusText = computed(() => {
  const who = displayAgentName.value || t('chat.sessionStage.systemStatus');
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
  border-radius: 10px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
  padding: 6px 10px
  transition: border-color 0.15s ease
  // Use flex column so the expanded agent-card__detail contributes to the
  // card's total height; otherwise the detail overflows and subsequent
  // siblings overlap it.
  display: flex
  flex-direction: column
  height: auto
  min-height: fit-content

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

  // === Header (80% when footer present, 100% when footer hidden) ===
  &__header
    flex: 0 0 calc(80% - 10px)
    min-width: 0
    display: flex
    align-items: center
    gap: 8px
    cursor: pointer

    &--full
      flex: 1

  &__avatar
    width: 24px
    height: 24px
    border-radius: 50%
    background: var(--glass-elevated, var(--glass-surface))
    display: flex
    align-items: center
    justify-content: center
    font-size: 11px
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
    padding: 1px 6px
    border-radius: 4px
    font-size: 11px
    font-weight: 500
    display: inline-flex
    align-items: center
    gap: 4px
    flex-shrink: 0

    &--blue
      background: color-mix(in srgb, var(--color-accent) 15%, transparent)
      color: var(--color-accent)
    &--orange
      background: color-mix(in srgb, var(--color-warning, #f39c12) 12%, transparent)
      color: var(--color-warning, #f39c12)
    &--green
      background: color-mix(in srgb, var(--color-success) 12%, transparent)
      color: var(--color-success)
    &--red
      background: color-mix(in srgb, var(--color-danger) 12%, transparent)
      color: var(--color-danger)
    &--grey
      background: color-mix(in srgb, var(--color-text-tertiary) 10%, transparent)
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
    gap: 4px
    // Limit the expanded agent panel height so long execution traces scroll
    // internally instead of pushing the entire chat stream downward.
    max-height: 480px
    overflow-y: auto
    overflow-x: hidden

@keyframes agent-card-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3
</style>
