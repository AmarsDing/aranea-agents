<template>
  <div class="llm-retry-banner" :class="bannerClass" :role="liveRole" aria-live="polite">
    <div class="llm-retry-banner__icon">
      <q-icon :name="iconName" size="18px" />
    </div>
    <div class="llm-retry-banner__body">
      <div class="llm-retry-banner__title">{{ titleText }}</div>
      <div class="llm-retry-banner__detail" :title="error || undefined">
        {{ detailText }}
        <span v-if="error" class="llm-retry-banner__error">· {{ error }}</span>
      </div>
      <div class="llm-retry-banner__hint">{{ hintText }}</div>
    </div>
    <q-spinner-dots v-if="isTransient" size="22px" class="llm-retry-banner__spinner" />
    <q-btn
      v-else
      flat
      dense
      round
      icon="close"
      size="sm"
      class="llm-retry-banner__dismiss"
      :aria-label="t('chat.llmAlertDismiss')"
      @click="$emit('dismiss')"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { LlmAlertKind } from '../../stores/chat/llmRetryStore';

const props = withDefaults(
  defineProps<{
    kind?: LlmAlertKind;
    attempt: number;
    /** -1 means infinite retries (backend default). */
    maxRetries: number;
    delayMs: number;
    error?: string;
    message?: string;
  }>(),
  { kind: 'retry' },
);

defineEmits<{ dismiss: [] }>();

const { t } = useI18n();

const isTransient = computed(() => props.kind === 'retry' || props.kind === 'rate_limit');

const bannerClass = computed(() => {
  switch (props.kind) {
    case 'billing':
      return 'llm-retry-banner--billing';
    case 'auth':
      return 'llm-retry-banner--auth';
    case 'stall':
      return 'llm-retry-banner--stall';
    default:
      return 'llm-retry-banner--retry';
  }
});

const iconName = computed(() => {
  switch (props.kind) {
    case 'billing':
      return 'account_balance_wallet';
    case 'auth':
      return 'vpn_key';
    case 'stall':
      return 'hourglass_disabled';
    default:
      return 'cloud_off';
  }
});

const liveRole = computed(() => (isTransient.value ? 'status' : 'alert'));

const delaySec = computed(() => Math.max(1, Math.round(props.delayMs / 1000)));

const titleText = computed(() => {
  switch (props.kind) {
    case 'billing':
      return t('chat.llmBillingTitle');
    case 'auth':
      return t('chat.llmAuthTitle');
    case 'stall':
      return t('chat.llmStallTitle');
    default:
      return t('chat.llmRetryTitle');
  }
});

const hintText = computed(() => {
  switch (props.kind) {
    case 'billing':
      return t('chat.llmBillingHint');
    case 'auth':
      return t('chat.llmAuthHint');
    case 'stall':
      return t('chat.llmStallHint');
    default:
      return t('chat.llmRetryHint');
  }
});

const detailText = computed(() => {
  if (props.kind === 'billing' || props.kind === 'auth' || props.kind === 'stall') {
    return props.message || titleText.value;
  }
  return props.maxRetries > 0
    ? t('chat.llmRetryDetailLimited', { attempt: props.attempt, max: props.maxRetries, delay: delaySec.value })
    : t('chat.llmRetryDetail', { attempt: props.attempt, delay: delaySec.value });
});
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
  color: var(--color-text-primary)

  &--retry
    border-left: 3px solid var(--color-warning)

  &--stall
    border-left: 3px solid var(--color-warning)

  &--billing,
  &--auth
    border-left: 3px solid var(--color-danger)

  &__icon
    display: flex
    align-items: center
    justify-content: center

  &--retry &__icon,
  &--stall &__icon
    color: var(--color-warning)

  &--retry &__icon
    animation: llm-retry-pulse 1.6s ease-in-out infinite

  &--billing &__icon,
  &--auth &__icon
    color: var(--color-danger)

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

  &__dismiss
    flex-shrink: 0
    color: var(--color-icon-muted)

@keyframes llm-retry-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.35

@media (prefers-reduced-motion: reduce)
  .llm-retry-banner--retry .llm-retry-banner__icon
    animation: none
</style>
