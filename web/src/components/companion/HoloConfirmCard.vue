<template>
  <div class="holo-confirm" role="alertdialog" aria-modal="false" :aria-label="titleText">
    <!-- 全息角框装饰 -->
    <span class="holo-confirm__corner holo-confirm__corner--tl" />
    <span class="holo-confirm__corner holo-confirm__corner--tr" />
    <span class="holo-confirm__corner holo-confirm__corner--bl" />
    <span class="holo-confirm__corner holo-confirm__corner--br" />
    <span class="holo-confirm__scanline" />

    <div class="holo-confirm__header">
      <q-icon name="warning_amber" size="16px" class="holo-confirm__icon" />
      <span class="holo-confirm__title">{{ titleText }}</span>
      <span v-if="countdownText" class="holo-confirm__countdown">{{ countdownText }}</span>
      <span v-if="queueSize > 1" class="holo-confirm__queue">
        {{ t('companion.confirmQueueMore', { n: queueSize - 1 }) }}
      </span>
    </div>

    <div v-if="card.target" class="holo-confirm__target">{{ card.target }}</div>
    <div class="holo-confirm__desc">{{ card.description }}</div>
    <div class="holo-confirm__tool">
      <q-icon name="construction" size="12px" class="q-mr-xs" />{{ card.toolName }}
    </div>

    <div class="holo-confirm__actions">
      <button class="holo-confirm__btn holo-confirm__btn--approve" :disabled="submitting || expired" @click="decide('approve')">
        {{ submitting ? t('companion.confirmSubmitting') : t('companion.confirmApprove') }}
      </button>
      <button class="holo-confirm__btn holo-confirm__btn--reject" :disabled="submitting || expired" @click="decide('deny')">
        {{ t('companion.confirmReject') }}
      </button>
      <button class="holo-confirm__btn holo-confirm__btn--always" :disabled="submitting || expired" @click="decide('always')">
        {{ t('companion.confirmApproveAlways') }}
      </button>
    </div>

    <div v-if="voiceModeOn" class="holo-confirm__voice-hint">
      <q-icon name="record_voice_over" size="12px" class="q-mr-xs" />{{ t('companion.confirmVoiceHint') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';

import type { ConfirmCardModel, ConfirmDecision } from '../../features/companion/types';

const props = defineProps<{
  card: ConfirmCardModel;
  /** 队列总长度（>1 时显示「还有 n 项」）。 */
  queueSize: number;
  /** 语音模式开启时显示语音确认提示。 */
  voiceModeOn: boolean;
}>();

const emit = defineEmits<{
  decide: [decision: ConfirmDecision];
}>();

const { t } = useI18n();

const titleText = computed(() => {
  if (props.card.source === 'external_coding') {
    const who = [props.card.agentKey, props.card.projectName].filter(Boolean).join(' · ');
    if (who) return t('companion.confirmExternalTitle', { who });
  }
  return t('companion.confirmTitle');
});

const submitting = ref(false);

// 确认窗口与后端 defaultToolConfirmationTimeout 一致（internal/agent/tool_confirmation.go）。
const CONFIRM_WINDOW_S = 5 * 60;
/** 提交超时兜底：WS 状态推进失败时恢复可点（与 ConfirmBlock 同策略）。 */
const SUBMIT_TIMEOUT_MS = 15_000;
let submitTimer: ReturnType<typeof setTimeout> | null = null;

const nowTick = ref(Date.now());
let tickTimer: ReturnType<typeof setInterval> | null = null;

const remainingSeconds = computed(() => {
  const started = Date.parse(props.card.startedAt);
  if (!Number.isFinite(started)) return 0;
  const elapsed = Math.floor((nowTick.value - started) / 1000);
  return Math.max(0, CONFIRM_WINDOW_S - elapsed);
});

// StartedAt 无效时不可判超时，保持可点（与 ConfirmBlock 一致）。
const expired = computed(() => Number.isFinite(Date.parse(props.card.startedAt)) && remainingSeconds.value <= 0);

const countdownText = computed(() => {
  if (!Number.isFinite(Date.parse(props.card.startedAt))) return '';
  const s = remainingSeconds.value;
  if (s <= 0) return t('companion.confirmExpired');
  const m = Math.floor(s / 60);
  const sec = String(s % 60).padStart(2, '0');
  return t('companion.confirmRemaining', { time: `${m}:${sec}` });
});

function decide(decision: ConfirmDecision) {
  if (submitting.value || expired.value) return;
  submitting.value = true;
  submitTimer = setTimeout(() => {
    submitting.value = false;
  }, SUBMIT_TIMEOUT_MS);
  emit('decide', decision);
}

// 卡切换（队列推进 / WS 状态更新）时复位提交态。
watch(
  () => props.card.id,
  () => {
    submitting.value = false;
    if (submitTimer) {
      clearTimeout(submitTimer);
      submitTimer = null;
    }
  },
);

tickTimer = setInterval(() => {
  nowTick.value = Date.now();
}, 1000);

onUnmounted(() => {
  if (tickTimer) clearInterval(tickTimer);
  if (submitTimer) clearTimeout(submitTimer);
});
</script>

<style scoped lang="sass">
.holo-confirm
  position: relative
  width: min(360px, 86vw)
  padding: 14px 16px 12px
  border-radius: 10px
  color: var(--color-text-primary)
  background: linear-gradient(160deg, rgba(10, 18, 28, 0.82), rgba(8, 12, 20, 0.88))
  border: 1px solid rgba(0, 229, 255, 0.35)
  box-shadow: 0 0 24px rgba(0, 229, 255, 0.18), inset 0 0 32px rgba(0, 229, 255, 0.05)
  backdrop-filter: blur(var(--glass-blur-default))
  overflow: hidden
  animation: holo-confirm-enter 0.28s cubic-bezier(0.2, 0.9, 0.25, 1.2)

  &__corner
    position: absolute
    width: 14px
    height: 14px
    border: 2px solid rgba(0, 229, 255, 0.8)
    filter: drop-shadow(0 0 4px rgba(0, 229, 255, 0.6))
    pointer-events: none

    &--tl
      top: 4px
      left: 4px
      border-right: none
      border-bottom: none
    &--tr
      top: 4px
      right: 4px
      border-left: none
      border-bottom: none
    &--bl
      bottom: 4px
      left: 4px
      border-right: none
      border-top: none
    &--br
      bottom: 4px
      right: 4px
      border-left: none
      border-top: none

  &__scanline
    position: absolute
    inset: 0
    pointer-events: none
    background: linear-gradient(180deg, transparent 0%, rgba(0, 229, 255, 0.08) 48%, rgba(0, 229, 255, 0.16) 50%, rgba(0, 229, 255, 0.08) 52%, transparent 100%)
    background-size: 100% 240%
    animation: holo-scan 3.2s linear infinite
    opacity: 0.7

  &__header
    display: flex
    align-items: center
    gap: 6px
    margin-bottom: 8px

  &__icon
    color: var(--color-warning)
    filter: drop-shadow(0 0 4px rgba(251, 191, 36, 0.6))

  &__title
    font-size: 13px
    font-weight: 600
    letter-spacing: 0.12em
    color: var(--color-neon-cyan)
    text-shadow: 0 0 8px rgba(0, 229, 255, 0.5)

  &__countdown
    margin-left: auto
    font-size: 11px
    font-variant-numeric: tabular-nums
    color: var(--color-text-tertiary)

  &__queue
    font-size: 11px
    padding: 1px 8px
    border-radius: 999px
    color: var(--color-neon-cyan)
    border: 1px solid rgba(0, 229, 255, 0.4)
    background: rgba(0, 229, 255, 0.08)

  &__target
    font-family: var(--font-mono, monospace)
    font-size: 18px
    font-weight: 600
    letter-spacing: 0.04em
    color: #fff
    text-shadow: 0 0 12px rgba(0, 229, 255, 0.75)
    margin-bottom: 6px
    word-break: break-all

  &__desc
    font-size: 13px
    line-height: 1.5
    color: var(--color-text-secondary)
    margin-bottom: 6px

  &__tool
    display: inline-flex
    align-items: center
    font-size: 11px
    color: var(--color-text-tertiary)
    padding: 2px 8px
    border-radius: 4px
    border: 1px solid var(--glass-border)
    background: rgba(255, 255, 255, 0.04)
    margin-bottom: 10px

  &__actions
    display: flex
    gap: 8px

  &__btn
    flex: 1
    padding: 7px 0
    border-radius: 6px
    font-size: 13px
    font-weight: 500
    letter-spacing: 0.05em
    cursor: pointer
    border: 1px solid transparent
    transition: box-shadow 0.2s ease, opacity 0.15s ease, transform 0.1s ease

    &:active:not(:disabled)
      transform: scale(0.97)

    &:disabled
      opacity: 0.45
      cursor: not-allowed

    &--approve
      color: #04121a
      background: linear-gradient(135deg, #22d3ee, #34d399)
      box-shadow: 0 0 14px rgba(0, 229, 255, 0.45)

      &:hover:not(:disabled)
        box-shadow: 0 0 22px rgba(0, 229, 255, 0.7)

    &--reject
      color: var(--color-danger)
      background: transparent
      border-color: color-mix(in srgb, var(--color-danger) 55%, transparent)

      &:hover:not(:disabled)
        box-shadow: 0 0 12px color-mix(in srgb, var(--color-danger) 35%, transparent)

    &--always
      color: var(--color-neon-cyan)
      background: rgba(0, 229, 255, 0.08)
      border-color: rgba(0, 229, 255, 0.35)

      &:hover:not(:disabled)
        box-shadow: 0 0 12px rgba(0, 229, 255, 0.35)

  &__voice-hint
    margin-top: 8px
    font-size: 11px
    display: flex
    align-items: center
    color: var(--color-text-tertiary)

@keyframes holo-confirm-enter
  from
    opacity: 0
    transform: translateY(10px) scale(0.96)
    clip-path: inset(40% 0 40% 0)
  to
    opacity: 1
    transform: translateY(0) scale(1)
    clip-path: inset(0 0 0 0)

@keyframes holo-scan
  from
    background-position: 0 -120%
  to
    background-position: 0 120%
</style>
