<template>
  <footer class="wb-statusbar" role="contentinfo">
    <span class="wb-statusbar__cell">
      <i class="wb-statusbar__ws" :class="wsConnected ? 'on' : 'off'" />
      {{ wsConnected ? t('workbench.statusbar.wsOn') : t('workbench.statusbar.wsOff') }}
    </span>
    <span class="wb-statusbar__cell wb-statusbar__clock">{{ clockText }}</span>
    <span class="wb-statusbar__cell" :class="{ 'wb-statusbar__cell--stale': freshnessSeconds < 0 }">
      {{ t('workbench.statusbar.freshness') }} {{ freshnessText }}
    </span>
    <span class="wb-statusbar__cell"> {{ t('workbench.statusbar.throughput') }} {{ eventRate }}/min </span>
    <span class="wb-statusbar__cell" :class="{ 'wb-statusbar__cell--backlog': backlogCount > 0 }">
      {{ t('workbench.statusbar.backlog') }} {{ backlogCount }}
    </span>
    <span class="wb-statusbar__cell wb-statusbar__mode">
      {{ trustState === 'replay' ? t('workbench.statusbar.replay') : t('workbench.statusbar.live') }}
    </span>
    <span class="wb-statusbar__cell wb-statusbar__note">{{ t('workbench.statusbar.note') }}</span>
  </footer>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { TrustState } from '../../features/workbench/types';

const props = defineProps<{
  wsConnected: boolean;
  /** 服务器时钟（ISO 字符串；页面按秒刷） */
  clockIso: string;
  /** 各 lane 最旧成功刷新（秒）；-1 表示尚无任何成功刷新 */
  freshnessSeconds: number;
  eventRate: number;
  backlogCount: number;
  trustState: TrustState;
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

const freshnessText = computed(() => {
  if (props.freshnessSeconds < 0) return '—';
  const s = props.freshnessSeconds;
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m`;
});
</script>

<style scoped lang="sass">
.wb-statusbar__cell
  display: inline-flex
  align-items: center
  gap: 5px
  flex: none
.wb-statusbar__ws
  width: 7px
  height: 7px
  border-radius: 50%
.wb-statusbar__ws.on
  background: var(--wb-ok)
.wb-statusbar__ws.off
  background: var(--wb-block)
.wb-statusbar__clock
  font-variant-numeric: tabular-nums
.wb-statusbar__cell--backlog
  color: var(--wb-await)
.wb-statusbar__cell--stale
  color: var(--wb-skip)
.wb-statusbar__mode
  margin-left: auto
  text-transform: uppercase
  letter-spacing: 0.08em
.wb-statusbar__note
  color: var(--color-text-tertiary, var(--color-text-secondary))
  overflow: hidden
  text-overflow: ellipsis
</style>
