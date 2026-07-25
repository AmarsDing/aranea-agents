<template>
  <div class="llm-retry-banner" role="status" aria-live="polite">
    <div class="llm-retry-banner__icon">
      <q-icon name="cloud_off" size="18px" />
    </div>
    <div class="llm-retry-banner__body">
      <div class="llm-retry-banner__title">{{ t('chat.llmRetryTitle') }}</div>
      <div class="llm-retry-banner__detail" :title="error || undefined">
        {{ detailText }}
        <span v-if="error" class="llm-retry-banner__error">· {{ error }}</span>
      </div>
      <div class="llm-retry-banner__hint">{{ t('chat.llmRetryHint') }}</div>
    </div>
    <q-spinner-dots size="22px" class="llm-retry-banner__spinner" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

const props = defineProps<{
  attempt: number;
  /** -1 means infinite retries (backend default). */
  maxRetries: number;
  delayMs: number;
  error?: string;
}>();

const { t } = useI18n();

const delaySec = computed(() => Math.max(1, Math.round(props.delayMs / 1000)));

const detailText = computed(() =>
  props.maxRetries > 0
    ? t('chat.llmRetryDetailLimited', { attempt: props.attempt, max: props.maxRetries, delay: delaySec.value })
    : t('chat.llmRetryDetail', { attempt: props.attempt, delay: delaySec.value }),
);
</script>

<style scoped lang="sass">
.llm-retry-banner
  display: flex
  align-items: center
  gap: 10px
  margin: 8px 12px 0
  padding: 10px 14px
  border-radius: 12px
  background: var(--glass-surface)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))
  border: 1px solid var(--glass-border)
  border-left: 3px solid var(--color-warning)
  color: var(--color-text-primary)

  &__icon
    display: flex
    align-items: center
    justify-content: center
    color: var(--color-warning)
    animation: llm-retry-pulse 1.6s ease-in-out infinite

  &__body
    flex: 1
    min-width: 0
    display: flex
    flex-direction: column
    gap: 2px

  &__title
    font-size: 13px
    font-weight: 600
    line-height: 1.3

  &__detail
    font-size: 12px
    color: var(--color-text-secondary)
    white-space: nowrap
    overflow: hidden
    text-overflow: ellipsis

  &__error
    opacity: 0.75

  &__hint
    font-size: 11px
    color: var(--color-icon-muted)

  &__spinner
    color: var(--color-warning)
    flex-shrink: 0

@keyframes llm-retry-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.35

@media (prefers-reduced-motion: reduce)
  .llm-retry-banner__icon
    animation: none
</style>
