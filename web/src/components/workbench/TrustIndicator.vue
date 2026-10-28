<template>
  <span class="wb-trust" :class="`wb-trust--${state}`" :title="tipText">
    <i class="wb-trust__dot" :class="{ 'wb-trust__dot--pulse': state === 'offline' }" />
    <span class="wb-trust__label">{{ label }}</span>
    <span v-if="state === 'stale' && staleSeconds > 0" class="wb-trust__num">{{ staleSeconds }}s</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { TrustState } from '../../features/workbench/types';

const props = defineProps<{
  state: TrustState;
  staleSeconds: number;
}>();

const { t } = useI18n();

const label = computed(() => t(`workbench.trust.${props.state}`));
const tipText = computed(() => t('workbench.trust.tip'));
</script>

<style scoped lang="sass">
.wb-trust
  display: inline-flex
  align-items: center
  gap: 6px
  font-size: var(--wb-caption)
  color: var(--color-text-secondary)
  white-space: nowrap
.wb-trust__dot
  width: 7px
  height: 7px
  border-radius: 50%
  background: var(--color-text-icon-muted)
.wb-trust--live .wb-trust__dot
  background: var(--wb-ok)
.wb-trust--offline .wb-trust__dot
  background: var(--wb-block)
.wb-trust--stale .wb-trust__dot
  background: var(--wb-flow)
.wb-trust--degraded .wb-trust__dot
  background: var(--wb-flow)
.wb-trust--backlog .wb-trust__dot
  background: var(--wb-await)
.wb-trust--replay .wb-trust__dot
  background: var(--color-violet)
.wb-trust__dot--pulse
  animation: wb-offline-pulse 1.2s ease-in-out infinite
.wb-trust__num
  font-variant-numeric: tabular-nums

// 事件态辉光豁免件 2：断连红脉冲（事件消散即灭）
@keyframes wb-offline-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.25
</style>
