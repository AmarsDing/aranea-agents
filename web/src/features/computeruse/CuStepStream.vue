<template>
  <div class="cu-stream">
    <div class="cu-stream__header">
      <span class="cu-stream__title">{{ t('computeruse.stream.title') }}</span>
      <span v-if="sessionId" class="cu-stream__session">{{ sessionId }}</span>
      <button v-if="killState !== 'killed'" class="cu-stream__kill" :disabled="killState === 'killing'" @click="onKill">
        {{ killState === 'killing' ? t('computeruse.stream.killing') : t('computeruse.stream.kill') }}
      </button>
      <span v-else class="cu-stream__killed">{{ t('computeruse.stream.killed') }}</span>
    </div>

    <div v-if="steps.length === 0" class="cu-stream__empty">
      {{ t('computeruse.stream.empty') }}
    </div>

    <div v-for="step in steps" :key="step.stepIndex" class="cu-step" :class="`cu-step--${step.result}`">
      <div class="cu-step__head">
        <span class="cu-step__index">#{{ step.stepIndex }}</span>
        <span class="cu-step__target">{{ step.target || step.action }}</span>
        <span class="cu-step__badge" :class="`cu-step__badge--${step.path}`">{{ step.path }}</span>
        <span v-if="step.danger" class="cu-step__danger">{{ t('computeruse.stream.danger') }}</span>
        <span class="cu-step__duration">{{ step.durationMs }}ms</span>
      </div>
      <div class="cu-step__meta">
        <span class="cu-step__result">{{ resultLabel(step.result) }}</span>
        <span v-if="step.confirmedBy" class="cu-step__confirmed">
          {{ t('computeruse.stream.confirmedBy') }}: {{ step.confirmedBy }}
        </span>
      </div>
      <div v-if="step.error" class="cu-step__error">{{ step.error }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { toRef } from 'vue';
import { useI18n } from 'vue-i18n';
import { useCuStepStream } from './useCuStepStream';

const props = defineProps<{
  /** computeruse 会话 ID（事件 session_id 过滤 + 急停目标）。 */
  sessionId: string;
}>();

const { t } = useI18n();
const { steps, killState, kill } = useCuStepStream(toRef(props, 'sessionId'));

const RESULT_LABEL_KEYS: Record<string, string> = {
  ok: 'computeruse.stream.result.ok',
  retry: 'computeruse.stream.result.retry',
  failed: 'computeruse.stream.result.failed',
  cancelled: 'computeruse.stream.result.cancelled',
};

// 未登记的 result 原样展示（raw 值非中文，无 i18n 负担）。
function resultLabel(result: string): string {
  const key = RESULT_LABEL_KEYS[result];
  return key ? t(key) : result;
}

async function onKill() {
  try {
    await kill();
  } catch {
    // 失败时 killState 已回退为 active 允许重试；错误提示由调用方/全局拦截处理。
  }
}
</script>

<style lang="sass" scoped>
.cu-stream
  border-radius: 8px
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)
  font-size: 13px

  &__header
    display: flex
    align-items: center
    gap: 8px
    padding: 8px 12px
    border-bottom: 1px solid var(--glass-border)

  &__title
    font-weight: 600
    color: var(--color-text-primary)

  &__session
    font-size: 11px
    color: var(--color-text-tertiary)
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__kill
    margin-left: auto
    padding: 3px 12px
    border: none
    border-radius: 6px
    font-size: 12px
    font-weight: 600
    cursor: pointer
    background: var(--color-danger)
    color: var(--color-on-accent, #fff)
    transition: opacity 0.15s ease

    &:hover
      opacity: 0.85

    &:disabled
      opacity: 0.5
      cursor: not-allowed

  &__killed
    margin-left: auto
    font-size: 12px
    font-weight: 600
    color: var(--color-danger)

  &__empty
    padding: 12px
    color: var(--color-text-tertiary)
    text-align: center

.cu-step
  padding: 8px 12px
  border-bottom: 1px solid var(--glass-border)

  &:last-child
    border-bottom: none

  &--failed
    border-left: 3px solid var(--color-danger)

  &--cancelled
    border-left: 3px solid var(--color-warning)

  &--ok
    border-left: 3px solid var(--color-success)

  &--retry
    border-left: 3px solid var(--color-warning)

  &__head
    display: flex
    align-items: center
    gap: 6px
    min-width: 0

  &__index
    font-size: 11px
    color: var(--color-text-tertiary)
    flex-shrink: 0

  &__target
    color: var(--color-text-primary)
    font-weight: 500
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__badge
    font-size: 10px
    font-weight: 600
    padding: 0 5px
    border-radius: 4px
    flex-shrink: 0
    background: color-mix(in srgb, var(--color-primary) 15%, transparent)
    color: var(--color-primary)

    &--vision,
    &--vlm_direct
      background: color-mix(in srgb, var(--color-accent) 15%, transparent)
      color: var(--color-accent)

  &__danger
    font-size: 10px
    font-weight: 600
    padding: 0 5px
    border-radius: 4px
    flex-shrink: 0
    background: var(--color-danger)
    color: var(--color-on-accent, #fff)

  &__duration
    margin-left: auto
    font-size: 11px
    color: var(--color-text-tertiary)
    flex-shrink: 0

  &__meta
    display: flex
    gap: 8px
    margin-top: 3px
    font-size: 11px
    color: var(--color-text-secondary)

  &__error
    margin-top: 3px
    font-size: 11px
    color: var(--color-danger)
    word-break: break-all
</style>
