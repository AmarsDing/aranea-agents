<template>
  <div v-if="step.Status === 'tool_blocked'" class="confirm-block confirm-block--blocked">
    <div class="confirm-block__header">
      <span class="confirm-block__icon">⚠️</span>
      <span class="confirm-block__label">{{ t('chat.confirm.label', '需要确认') }}</span>
      <span v-if="step.Danger" class="confirm-block__danger">{{ t('chat.confirm.danger') }}</span>
      <span v-if="countdownText" class="confirm-block__countdown">{{ countdownText }}</span>
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
      <button
        class="confirm-block__btn confirm-block__btn--approve"
        :disabled="confirming || expired"
        @click="onConfirm(TOOL_CONFIRM_REPLY.approve)"
      >
        {{ confirming ? t('chat.confirm.submitting', '提交中…') : t('chat.confirm.approve', '允许本次') }}
      </button>
      <button
        class="confirm-block__btn confirm-block__btn--reject"
        :disabled="confirming || expired"
        @click="onConfirm(TOOL_CONFIRM_REPLY.deny)"
      >
        {{ confirming ? t('chat.confirm.submitting', '提交中…') : t('chat.confirm.reject', '拒绝') }}
      </button>
      <!-- 75 A5/需求 §5.3：高危确认只保留「允许本次/拒绝」——后端 danger 命中
           强制逐次确认（授权链不生效），隐藏会话/持久授权按钮避免误导。 -->
      <button
        v-if="!step.Danger"
        class="confirm-block__btn confirm-block__btn--approve-session"
        :disabled="confirming || expired"
        @click="onConfirm(TOOL_CONFIRM_REPLY.approveSession)"
      >
        {{ confirming ? t('chat.confirm.submitting', '提交中…') : t('chat.confirm.approveSession', '会话内始终允许') }}
      </button>
      <button
        v-if="!step.Danger"
        class="confirm-block__btn confirm-block__btn--approve-always"
        :disabled="confirming || expired"
        @click="onConfirm(TOOL_CONFIRM_REPLY.approveAlways)"
      >
        {{ confirming ? t('chat.confirm.submitting', '提交中…') : t('chat.confirm.approveAlways', '始终允许') }}
      </button>
    </div>
  </div>

  <div v-else-if="step.Status === 'completed'" class="confirm-block confirm-block--approved">
    <span class="confirm-block__icon">✓</span>
    <span class="confirm-block__summary">{{ t('chat.confirm.approved', '已批准') }}</span>
    <span class="confirm-block__tool-inline">· {{ step.ToolName }}</span>
  </div>

  <div v-else-if="step.Status === 'cancelled' && isConfirmTimeoutRetry" class="confirm-block confirm-block--timeout confirm-block--timeout-retry">
    <span class="confirm-block__icon">⏱</span>
    <span class="confirm-block__summary">{{ t('chat.confirm.timedOutRetry') }}</span>
    <span class="confirm-block__tool-inline">· {{ step.ToolName }}</span>
  </div>

  <div v-else-if="step.Status === 'cancelled' && isConfirmTimeout" class="confirm-block confirm-block--timeout">
    <span class="confirm-block__icon">⏱</span>
    <span class="confirm-block__summary">{{ t('chat.confirm.timedOut') }}</span>
    <span class="confirm-block__tool-inline">· {{ step.ToolName }}</span>
  </div>

  <div v-else-if="step.Status === 'cancelled'" class="confirm-block confirm-block--rejected">
    <span class="confirm-block__icon">✗</span>
    <span class="confirm-block__summary">{{ t('chat.confirm.rejected', '已拒绝') }}</span>
    <span class="confirm-block__tool-inline">· {{ step.ToolName }}</span>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Step } from '../../features/chat/v2Types';
import { TOOL_CONFIRM_REPLY, type ToolConfirmReply, type ConfirmStepPayload } from '../../features/chat/types';

const props = defineProps<{
  step: Step;
}>();

const emit = defineEmits<{
  confirm: [payload: ConfirmStepPayload];
}>();

const { t } = useI18n();

/** Backend marks deadline-expired confirmations with this ToolErrorCode
 * (agent/v2.ConfirmTimeoutErrorCode) so the UI can render "timed out"
 * instead of "rejected". */
const CONFIRM_TIMEOUT_ERROR_CODE = 'confirm_timeout';
const CONFIRM_TIMEOUT_RETRY_ERROR_CODE = 'confirm_timeout_retry';
const isConfirmTimeoutRetry = computed(() => props.step.ToolErrorCode === CONFIRM_TIMEOUT_RETRY_ERROR_CODE);
const isConfirmTimeout = computed(() => props.step.ToolErrorCode === CONFIRM_TIMEOUT_ERROR_CODE);

const confirming = ref(false);
const CONFIRM_TIMEOUT_MS = 15_000;
let confirmTimer: ReturnType<typeof setTimeout> | null = null;

// Confirmation window must match backend defaultToolConfirmationTimeout
// (internal/agent/tool_confirmation.go).
const CONFIRM_WINDOW_S = 5 * 60;
const nowTick = ref(Date.now());
let tickTimer: ReturnType<typeof setInterval> | null = null;

const remainingSeconds = computed(() => {
  if (props.step.Status !== 'tool_blocked') return 0;
  const started = Date.parse(props.step.StartedAt);
  if (!Number.isFinite(started)) return 0;
  const elapsed = Math.floor((nowTick.value - started) / 1000);
  return Math.max(0, CONFIRM_WINDOW_S - elapsed);
});

// Only treat the card as expired when the start timestamp is valid — a
// missing/unparseable StartedAt must not disable the buttons (remainingSeconds
// falls back to 0 in that case, which would otherwise read as "expired").
const expired = computed(
  () =>
    props.step.Status === 'tool_blocked' &&
    Number.isFinite(Date.parse(props.step.StartedAt)) &&
    remainingSeconds.value <= 0,
);

const countdownText = computed(() => {
  if (props.step.Status !== 'tool_blocked' || !Number.isFinite(Date.parse(props.step.StartedAt))) return '';
  const s = remainingSeconds.value;
  if (s <= 0) return t('chat.confirm.expired');
  const m = Math.floor(s / 60);
  const sec = String(s % 60).padStart(2, '0');
  return t('chat.confirm.remaining', { time: `${m}:${sec}` });
});

function startTick() {
  if (tickTimer) return;
  nowTick.value = Date.now();
  tickTimer = setInterval(() => {
    nowTick.value = Date.now();
  }, 1000);
}

function stopTick() {
  if (tickTimer) {
    clearInterval(tickTimer);
    tickTimer = null;
  }
}

watch(
  () => props.step.Status,
  (status) => {
    confirming.value = false;
    if (confirmTimer) {
      clearTimeout(confirmTimer);
      confirmTimer = null;
    }
    if (status === 'tool_blocked') startTick();
    else stopTick();
  },
  { immediate: true },
);

watch(remainingSeconds, (s) => {
  if (s <= 0) stopTick();
});

onUnmounted(() => {
  stopTick();
  if (confirmTimer) {
    clearTimeout(confirmTimer);
    confirmTimer = null;
  }
});

const toolArgumentsJson = computed(() => {
  if (props.step.ToolArgs == null) return '';
  if (typeof props.step.ToolArgs === 'string') return props.step.ToolArgs;
  try {
    return JSON.stringify(props.step.ToolArgs, null, 2);
  } catch {
    return String(props.step.ToolArgs);
  }
});

function onConfirm(reply: ToolConfirmReply) {
  if (confirming.value) return;
  confirming.value = true;
  confirmTimer = setTimeout(() => {
    confirming.value = false;
  }, CONFIRM_TIMEOUT_MS);
  emit('confirm', {
    sessionId: props.step.SessionID,
    activityId: props.step.ID,
    reply,
    // 结算回调：终点 handler 在 RPC 成功/失败后立即复位按钮，
    // 避免失败时按钮卡在「提交中…」直到 15s 兜底定时器到期。
    onSettled: () => {
      confirming.value = false;
      if (confirmTimer) {
        clearTimeout(confirmTimer);
        confirmTimer = null;
      }
    },
  });
}
</script>

<style lang="sass" scoped>
.confirm-block
  border-radius: 8px
  font-size: 13px

  &--blocked
    padding: 10px 12px
    background: color-mix(in srgb, var(--color-warning) 8%, transparent)
    border: 1px solid color-mix(in srgb, var(--color-warning) 30%, transparent)
    border-left: 3px solid var(--color-warning)

  &--approved
    display: flex
    align-items: center
    gap: 6px
    padding: 4px 10px
    border-left: 3px solid var(--color-success)
    background: var(--glass-surface)

  &--rejected
    display: flex
    align-items: center
    gap: 6px
    padding: 4px 10px
    border-left: 3px solid var(--color-danger)
    background: var(--glass-surface)

  &--timeout
    display: flex
    align-items: center
    gap: 6px
    padding: 4px 10px
    border-left: 3px solid var(--color-warning)
    background: var(--glass-surface)

  &--timeout-retry
    align-items: flex-start
    flex-wrap: wrap
    padding: 8px 10px

    .confirm-block__summary
      color: var(--color-text-primary)
      line-height: 1.4

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

  &__danger
    font-size: 11px
    font-weight: 600
    padding: 1px 6px
    border-radius: 4px
    color: var(--color-on-accent, #fff)
    background: var(--color-danger)

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
    flex-wrap: wrap
    gap: 6px

  &__btn
    padding: 4px 12px
    border-radius: 6px
    border: none
    font-size: 12px
    font-weight: 500
    cursor: pointer
    transition: opacity 0.15s ease
    white-space: nowrap

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

    &--approve-session
      background: var(--color-primary)
      color: var(--color-on-accent, #fff)

    &--approve-always
      background: var(--color-accent)
      color: var(--color-on-accent, #fff)

  &__summary
    color: var(--color-text-secondary)

  &__tool-inline
    font-size: 12px
    color: var(--color-text-tertiary)

// 移动端（<600px，72 §3.2 触控化）：确认按钮纵向堆叠、全宽、触控目标 ≥44px
@media (max-width: 599px)
  .confirm-block
    &__actions
      flex-direction: column
      flex-wrap: nowrap
      gap: 8px

    &__btn
      width: 100%
      min-height: 44px
      font-size: 14px
</style>
