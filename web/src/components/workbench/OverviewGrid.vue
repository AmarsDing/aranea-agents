<template>
  <section class="wb-og">
    <div class="wb-og__kpi">
      <div class="wb-og__tile" :class="{ gold: awaitCount > 0 }">
        <span class="wb-og__micro">{{ t('workbench.grid.kpiAwait') }}</span>
        <span class="wb-og__num">{{ awaitCount }}</span>
        <span class="wb-og__foot">{{ t('workbench.grid.kpiAwaitFoot') }}</span>
      </div>
      <div class="wb-og__tile">
        <span class="wb-og__micro">{{ t('workbench.grid.kpiDuty') }}</span>
        <span class="wb-og__num">{{ agentCount }}</span>
        <span class="wb-og__foot">{{ t('workbench.grid.kpiDutyFoot') }}</span>
      </div>
      <div class="wb-og__tile wb-og__tile--glow">
        <span class="wb-og__micro">{{ t('workbench.grid.kpiDone') }}</span>
        <!-- 回填未落地前显 —（诚实口径，不作 0）；落地后点阵心跳领稳态唯一辉光 -->
        <span v-if="!backfillReady" class="wb-og__num">—</span>
        <HeartbeatCounter v-else class="wb-kpi-glow" :value="completions" :tip-text="t('workbench.grid.kpiDoneFoot')" />
        <span class="wb-og__foot">{{ t('workbench.grid.kpiDoneFoot') }}</span>
      </div>
      <div class="wb-og__tile">
        <span class="wb-og__micro">{{ t('workbench.grid.kpiRate') }}</span>
        <span class="wb-og__num">{{ eventRate }}</span>
        <span class="wb-og__foot">{{ t('workbench.grid.kpiRateFoot') }}</span>
      </div>
    </div>

    <div class="wb-og__hero"><slot /></div>

    <div class="wb-og__lines">
      <div v-for="line in bizLines" :key="line.id" class="wb-og__line" :class="{ dim: !line.connected }">
        <span class="wb-og__micro">{{ t(`workbench.lines.${line.id}`) }}</span>
        <span class="wb-og__line-num">
          <template v-if="line.connected">{{ line.pending }}</template>
          <template v-else>—</template>
        </span>
        <span class="wb-og__foot">
          {{ line.connected ? t('workbench.grid.pendingFoot') : t('workbench.sources.pending') }}
        </span>
        <span v-if="line.pending > 0" class="wb-await-badge wb-og__line-badge">{{ line.pending }}</span>
      </div>
    </div>

    <div class="wb-og__loghead">
      <span class="wb-og__micro">{{ t('workbench.grid.logTitle') }}</span>
    </div>
    <div class="wb-og__logs">
      <div v-for="l in logs" :key="l.id" class="wb-og__log" :class="`is-${l.kind}`">
        <span class="wb-og__log-ts">{{ shortTime(l.at) }}</span>
        <span v-if="l.agentKey" class="wb-og__log-agent">@{{ l.agentKey }}</span>
        <span class="wb-og__log-text">{{ l.text }}</span>
      </div>
      <div v-if="!logs.length" class="wb-og__log-empty">{{ t('workbench.grid.logEmpty') }}</div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { BizLineCard, LogLine } from '../../features/workbench/types';
import HeartbeatCounter from './HeartbeatCounter.vue';

defineProps<{
  awaitCount: number;
  agentCount: number;
  completions: number;
  /** 冷开回填是否落地：false → done 计数显 —（禁把未回填当 0） */
  backfillReady: boolean;
  eventRate: number;
  bizLines: BizLineCard[];
  logs: LogLine[];
}>();

const { t } = useI18n();

function shortTime(iso: string): string {
  if (!iso) return '--:--';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '--:--';
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}
</script>

<style scoped lang="sass">
.wb-og
  display: flex
  flex-direction: column
  gap: 8px
  min-height: 0
  height: 100%
.wb-og__kpi
  flex: none
  display: grid
  grid-template-columns: repeat(4, 1fr)
  gap: 8px
.wb-og__tile
  position: relative
  padding: 8px 10px
  border: 1px solid var(--wb-line-hairline)
  border-radius: var(--wb-radius)
  background: color-mix(in srgb, var(--color-text-primary) 3%, transparent)
  display: flex
  flex-direction: column
  gap: 1px
  &.gold
    border-color: var(--wb-await-line)
.wb-og__micro
  font-size: var(--wb-micro-label)
  letter-spacing: 0.14em
  text-transform: uppercase
  color: var(--color-text-tertiary, var(--color-text-secondary))
.wb-og__num
  font-size: 26px
  line-height: 1.1
  font-weight: 700
  font-variant-numeric: tabular-nums
  color: var(--color-text-primary)
  &.wb-kpi-glow
    color: var(--wb-flow)
.wb-og__tile.gold .wb-og__num
  color: var(--wb-await)
.wb-og__foot
  font-size: 10px
  color: var(--color-text-secondary)
.wb-og__hero
  flex: 1
  min-height: 220px
  display: flex
  flex-direction: column
.wb-og__lines
  flex: none
  display: grid
  grid-template-columns: repeat(4, 1fr)
  gap: 8px
.wb-og__line
  position: relative
  padding: 6px 10px
  border: 1px solid var(--wb-line-hairline)
  border-radius: var(--wb-radius)
  display: flex
  flex-direction: column
  gap: 1px
  &.dim
    opacity: 0.45
.wb-og__line-num
  font-size: 16px
  font-weight: 700
  font-variant-numeric: tabular-nums
.wb-og__line-badge
  position: absolute
  top: 6px
  right: 8px
.wb-og__loghead
  flex: none
  padding-top: 2px
.wb-og__logs
  flex: none
  height: 132px
  overflow-y: auto
  border-top: 1px solid var(--wb-line-hairline)
  padding-top: 4px
.wb-og__log
  display: flex
  gap: 8px
  padding: 2px 2px
  font-size: var(--wb-caption)
  color: var(--color-text-secondary)
  animation: wb-flash-bg var(--wb-dur-flash) ease-out
  &.is-you
    color: var(--wb-ok)
  &.is-gold
    color: var(--wb-await)
  &.is-error
    color: var(--wb-block)
.wb-og__log-ts
  flex: none
  font-variant-numeric: tabular-nums
.wb-og__log-agent
  flex: none
  font-weight: 600
.wb-og__log-text
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis
.wb-og__log-empty
  padding: 8px 2px
  font-size: var(--wb-caption)
  color: var(--color-text-secondary)
</style>
