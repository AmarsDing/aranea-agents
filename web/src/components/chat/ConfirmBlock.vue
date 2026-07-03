<template>
  <!-- tool_blocked: expanded confirmation UI -->
  <div v-if="step.Status === 'tool_blocked'" class="confirm-block confirm-block--blocked">
    <div class="confirm-block__header">
      <span class="confirm-block__icon">⚠️</span>
      <span class="confirm-block__label">{{ t('chat.confirm.label', '需要确认') }}</span>
    </div>
    <div class="confirm-block__content">{{ step.Content }}</div>
    <div class="confirm-block__tool-name">
      <span class="confirm-block__tool-label">{{ t('chat.confirm.tool', '工具') }}</span>
      <span class="confirm-block__tool-value">{{ step.ToolName }}</span>
    </div>
    <div v-if="toolArgumentsJson" class="confirm-block__args">
      <div class="confirm-block__detail-label">{{ t('chat.confirm.arguments', '参数') }}</div>
      <pre class="confirm-block__code">{{ toolArgumentsJson }}</pre>
    </div>
    <div class="confirm-block__actions">
      <button class="confirm-block__btn confirm-block__btn--approve" :disabled="confirming" @click="onApprove">
        {{ confirming ? t('chat.confirm.submitting', '提交中…') : t('chat.confirm.approve', '批准') }}
      </button>
      <button class="confirm-block__btn confirm-block__btn--reject" :disabled="confirming" @click="onReject">
        {{ confirming ? t('chat.confirm.submitting', '提交中…') : t('chat.confirm.reject', '拒绝') }}
      </button>
    </div>
  </div>

  <!-- completed: collapsed approved -->
  <div v-else-if="step.Status === 'completed'" class="confirm-block confirm-block--approved">
    <span class="confirm-block__icon">✓</span>
    <span class="confirm-block__summary">{{ t('chat.confirm.approved', '已批准') }}</span>
    <span class="confirm-block__tool-inline">· {{ step.ToolName }}</span>
  </div>

  <!-- cancelled: collapsed rejected -->
  <div v-else-if="step.Status === 'cancelled'" class="confirm-block confirm-block--rejected">
    <span class="confirm-block__icon">✗</span>
    <span class="confirm-block__summary">{{ t('chat.confirm.rejected', '已拒绝') }}</span>
    <span class="confirm-block__tool-inline">· {{ step.ToolName }}</span>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Step } from '../../features/chat/v2Types';

const props = defineProps<{
  step: Step;
}>();

const emit = defineEmits<{
  confirm: [activityId: string, approved: boolean];
}>();

const { t } = useI18n();

// --- Confirm loading state ---
const confirming = ref(false);
const CONFIRM_TIMEOUT_MS = 15_000;
let confirmTimer: ReturnType<typeof setTimeout> | null = null;

// Reset confirming when step status changes (WebSocket will update status)
watch(
  () => props.step.Status,
  () => {
    confirming.value = false;
    if (confirmTimer) {
      clearTimeout(confirmTimer);
      confirmTimer = null;
    }
  },
);

// --- Helpers ---
const toolArgumentsJson = computed(() => {
  if (props.step.ToolArgs == null) return '';
  if (typeof props.step.ToolArgs === 'string') return props.step.ToolArgs;
  try {
    return JSON.stringify(props.step.ToolArgs, null, 2);
  } catch {
    return String(props.step.ToolArgs);
  }
});

function onApprove() {
  if (confirming.value) return;
  confirming.value = true;
  confirmTimer = setTimeout(() => {
    confirming.value = false;
  }, CONFIRM_TIMEOUT_MS);
  emit('confirm', props.step.ID, true);
}

function onReject() {
  if (confirming.value) return;
  confirming.value = true;
  confirmTimer = setTimeout(() => {
    confirming.value = false;
  }, CONFIRM_TIMEOUT_MS);
  emit('confirm', props.step.ID, false);
}
</script>

<style lang="sass" scoped>
.confirm-block
  border-radius: 8px
  font-size: 13px

  // -- Blocked (waiting for confirmation) --
  &--blocked
    padding: 10px 12px
    background: color-mix(in srgb, var(--color-warning) 8%, transparent)
    border: 1px solid color-mix(in srgb, var(--color-warning) 30%, transparent)
    border-left: 3px solid var(--color-warning)

  // -- Approved (collapsed) --
  &--approved
    display: flex
    align-items: center
    gap: 6px
    padding: 4px 10px
    border-left: 3px solid var(--color-success)
    background: var(--glass-surface)

  // -- Rejected (collapsed) --
  &--rejected
    display: flex
    align-items: center
    gap: 6px
    padding: 4px 10px
    border-left: 3px solid var(--color-danger)
    background: var(--glass-surface)

  &__header
    display: flex
    align-items: center
    gap: 6px
    margin-bottom: 6px

  &__icon
    font-size: 13px
    flex-shrink: 0

  &__label
    font-weight: 500
    color: var(--color-warning)

  &__countdown
    font-size: 11px
    color: var(--color-text-tertiary)
    margin-left: auto

  &__content
    color: var(--color-text-primary)
    line-height: 1.5
    margin-bottom: 8px

  &__tool-name
    display: flex
    align-items: center
    gap: 4px
    margin-bottom: 6px

  &__tool-label
    font-size: 11px
    color: var(--color-text-tertiary)

  &__tool-value
    font-size: 12px
    color: var(--color-text-secondary)
    font-weight: 500

  &__detail-label
    font-size: 11px
    color: var(--color-text-secondary)
    margin-bottom: 2px

  &__args
    margin-bottom: 8px

  &__code
    font-size: 12px
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)
    border-radius: 6px
    padding: 6px 8px
    overflow-x: auto
    max-height: 200px
    overflow-y: auto
    margin: 0

  &__actions
    display: flex
    gap: 8px

  &__btn
    padding: 4px 14px
    border-radius: 6px
    border: none
    font-size: 12px
    font-weight: 500
    cursor: pointer
    transition: opacity 0.15s ease

    &:hover
      opacity: 0.85

    &:disabled
      opacity: 0.5
      cursor: not-allowed

    &--approve
      background: var(--color-success)
      color: var(--color-on-accent, #fff)

    &--reject
      background: var(--color-danger)
      color: var(--color-on-accent, #fff)

  &__summary
    color: var(--color-text-secondary)

  &__tool-inline
    font-size: 12px
    color: var(--color-text-tertiary)
</style>
