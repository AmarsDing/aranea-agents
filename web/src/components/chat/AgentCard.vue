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

      <!-- Footer (20%) — pause/resume button + inject box -->
      <div class="agent-card__footer">
        <div class="agent-card__actions">
          <button
            v-if="activity.status === 'running'"
            class="agent-card__action agent-card__action--pause"
            @click.stop="$emit('cancel-agent', activity.agentKey || '')"
          >
            {{ t('chat.sessionStage.pause') }}
          </button>
          <!-- resume button reserved for Phase T3 (interrupted status) -->
        </div>
        <div class="agent-card__inject" :class="{ 'agent-card__inject--expanded': injectExpanded }">
          <input
            v-model="injectText"
            class="agent-card__inject-input"
            :placeholder="t('chat.sessionStage.supplementPlaceholder')"
            @focus="injectExpanded = true"
            @keyup.enter="onInjectSend"
          />
          <button
            v-if="injectExpanded && injectText.trim()"
            class="agent-card__inject-send"
            @click="onInjectSend"
          >
            {{ t('chat.sessionStage.send') }}
          </button>
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
  'cancel-agent': [agentKey: string];
  'resume-agent': [agentKey: string];
  inject: [payload: { agentKey: string; message: string }];
}>();

const { t } = useI18n();

// === Collapse state (B.4.5: running default expanded, terminal default collapsed) ===
const expanded = ref(props.activity.status === 'running');
const injectExpanded = ref(false);
const injectText = ref('');

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
// Note: 'interrupted' is not in SessionStageEvent.status today (mapped to 'failed'
// by mapActivityStatusToStageStatus). Phase T3 may extend the type to surface
// interrupted for the resume button.
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

function onInjectSend() {
  const msg = injectText.value.trim();
  if (!msg || !props.activity.agentKey) return;
  emit('inject', { agentKey: props.activity.agentKey, message: msg });
  injectText.value = '';
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

  &__inject
    display: flex
    align-items: center
    gap: 4px
    transition: all 0.15s ease

    &--expanded
      .agent-card__inject-input
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
