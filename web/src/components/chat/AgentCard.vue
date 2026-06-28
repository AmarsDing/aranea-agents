<template>
  <div class="agent-card" :class="`agent-card--${activity.status}`">
    <div class="agent-card__row">
      <!-- Header (80%) — avatar + name + status badge + created time -->
      <div class="agent-card__header" @click="toggleExpand">
        <span class="agent-card__avatar">{{ agentInitial }}</span>
        <span class="agent-card__name" :title="displayAgentName">{{ displayAgentName }}</span>
        <span class="agent-card__status-badge" :class="`agent-card__status-badge--${statusColor}`">
          <span v-if="activity.status === 'running'" class="agent-card__pulse" />
          {{ statusText }}
        </span>
        <span v-if="createdTimeText" class="agent-card__time">{{ createdTimeText }}</span>
        <q-icon
          v-if="canEnter"
          name="chevron_right"
          size="14px"
          class="agent-card__enter-chevron"
          @click.stop="onEnter"
        />
      </div>

      <!-- Footer (20%) — cancel/retry buttons (B.4.2/B.5.2: cancel + retry only) -->
      <div class="agent-card__footer">
        <div class="agent-card__actions">
          <button
            v-if="activity.status === 'running'"
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
      </div>
    </div>

    <!-- Expanded detail (children rendered by recursive ActivityStream) -->
    <div v-if="expanded" class="agent-card__detail">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SessionStageEvent } from '../../features/chat/streamEventTypes';
import { nameInitial } from '../../features/spirit/spiritUi';

const props = defineProps<{
  activity: SessionStageEvent;
}>();

const emit = defineEmits<{
  'enter-session': [sessionId: string];
  'cancel-agent': [sessionId: string];
  'retry-agent': [sessionId: string];
}>();

const { t } = useI18n();

// === Collapse state (B.4.5: running default expanded, terminal default collapsed) ===
const expanded = ref(props.activity.status === 'running');

function toggleExpand() {
  expanded.value = !expanded.value;
}

// === Derived display values (all from props, no store dependency — red line #1) ===
const canEnter = computed(() => Boolean(props.activity.childSessionId));

const displayAgentName = computed(() => {
  if (props.activity.agentName) return props.activity.agentName;
  if (props.activity.title) return props.activity.title;
  return t('chat.sessionStage.member');
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
  const who = props.activity.agentName || t('chat.sessionStage.member');
  switch (props.activity.status) {
    case 'running':
      return t('chat.sessionStage.executing', { name: who });
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

function onEnter() {
  if (!canEnter.value) return;
  emit('enter-session', props.activity.childSessionId as string);
}

</script>

<style lang="sass" scoped>
.agent-card
  border-radius: 10px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
  padding: 6px 10px
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

  // === Header (80%) ===
  &__header
    flex: 0 0 calc(80% - 10px)
    min-width: 0
    display: flex
    align-items: center
    gap: 8px
    cursor: pointer

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

  &__enter-chevron
    color: var(--color-text-tertiary)
    flex-shrink: 0
    transition: transform 0.15s ease
    cursor: pointer

    &:hover
      color: var(--color-accent)

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
    gap: 4px
    padding-left: 8px
    border-left: 1px solid var(--glass-border)

  &__actions
    display: flex
    flex-direction: column
    gap: 4px
    align-items: stretch

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

  // === Expanded detail ===
  &__detail
    margin-top: 6px
    padding-top: 6px
    border-top: 1px dashed var(--glass-border)
    display: flex
    flex-direction: column
    gap: 4px

@keyframes agent-card-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3
</style>
