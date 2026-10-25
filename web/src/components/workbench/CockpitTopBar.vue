<template>
  <header class="wb-topbar">
    <button type="button" class="wb-topbar__cmdk" :title="t('workbench.cmdk.hint')" @click="$emit('open-palette')">
      <q-icon name="keyboard_command_key" size="14px" />
      <span>⌘K</span>
    </button>
    <div class="wb-topbar__title">
      <span class="wb-topbar__name">{{ t('workbench.title') }}</span>
      <span class="wb-topbar__sub">{{ t('workbench.subtitle') }}</span>
    </div>
    <div class="wb-topbar__spacer" />
    <span class="wb-topbar__live" :class="{ off: !wsConnected }">
      <i />{{ wsConnected ? t('workbench.statusbar.live') : t('workbench.offline') }}
    </span>
    <span class="wb-topbar__clock">{{ clockText }}</span>
    <span v-if="todayAwait > 0" class="wb-await-badge" :title="t('workbench.awaitTip')">
      {{ todayAwait > 99 ? '99+' : todayAwait }}
    </span>
    <span v-else class="wb-topbar__clear">{{ t('workbench.allClear') }}</span>
    <TrustIndicator :state="trustState" :stale-seconds="staleSeconds" />
    <q-btn flat dense round size="sm" icon="refresh" :title="t('workbench.refresh')" @click="$emit('refresh')" />
  </header>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { TrustState } from '../../features/workbench/types';
import TrustIndicator from './TrustIndicator.vue';

const props = defineProps<{
  todayAwait: number;
  wsConnected: boolean;
  clockIso: string;
  trustState: TrustState;
  staleSeconds: number;
}>();

defineEmits<{
  'open-palette': [];
  refresh: [];
}>();

const { t } = useI18n();

const clockText = computed(() => {
  if (!props.clockIso) return '—';
  const d = new Date(props.clockIso);
  if (Number.isNaN(d.getTime())) return '—';
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');
  const ss = String(d.getSeconds()).padStart(2, '0');
  return `${hh}:${mm}:${ss}`;
});
</script>

<style scoped lang="sass">
.wb-topbar
  display: flex
  align-items: center
  gap: 10px
  height: 42px
  padding: 0 12px
  border-bottom: 1px solid var(--wb-line-hairline)
  flex: none
.wb-topbar__cmdk
  display: inline-flex
  align-items: center
  gap: 4px
  border: 1px solid var(--wb-line-hairline)
  border-radius: var(--wb-radius)
  background: transparent
  color: var(--color-text-secondary)
  padding: 3px 8px
  font-size: var(--wb-caption)
  cursor: pointer
  &:hover
    color: var(--color-text-primary)
    border-color: color-mix(in srgb, var(--color-text-primary) 25%, transparent)
.wb-topbar__title
  display: flex
  flex-direction: column
  line-height: 1.15
.wb-topbar__name
  font-size: 13px
  font-weight: 600
  letter-spacing: 0.04em
.wb-topbar__sub
  font-size: 10px
  color: var(--color-text-secondary)
  letter-spacing: 0.1em
  text-transform: uppercase
.wb-topbar__spacer
  flex: 1
.wb-topbar__live
  display: inline-flex
  align-items: center
  gap: 5px
  font-size: var(--wb-caption)
  letter-spacing: 0.1em
  color: var(--wb-ok)
  i
    width: 7px
    height: 7px
    border-radius: 50%
    background: var(--wb-ok)
  &.off
    color: var(--wb-block)
    i
      background: var(--wb-block)
.wb-topbar__clock
  font-size: var(--wb-caption)
  color: var(--color-text-secondary)
  font-variant-numeric: tabular-nums
.wb-topbar__clear
  font-size: var(--wb-caption)
  color: var(--wb-ok)
.letter-spaced
  letter-spacing: 0.1em
</style>
