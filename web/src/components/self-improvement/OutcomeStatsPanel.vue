<template>
  <div class="outcome-stats">
    <!-- 统计接口失败（区别于真实 0 数据）：整面板降级为「不可用」 -->
    <div v-if="failed" class="outcome-stats__unavailable overview-panel">
      <q-icon name="cloud_off" size="sm" class="outcome-stats__unavailable-icon" />
      <span class="overview-section-caption">{{ t('selfImprovementPage.statsUnavailable') }}</span>
    </div>
    <template v-else>
      <div class="overview-stats-row q-mb-md">
        <div class="overview-stat-card">
          <div class="overview-stat-card__label">{{ t('selfImprovementPage.statsTotal') }}</div>
          <div class="overview-stat-card__value">{{ stats?.total ?? 0 }}</div>
        </div>
        <div class="overview-stat-card">
          <div class="overview-stat-card__label">{{ t('selfImprovementPage.statsEffectiveRate') }}</div>
          <div class="overview-stat-card__value text-positive">{{ formatPct(stats?.effectiveRate) }}</div>
        </div>
        <div class="overview-stat-card">
          <div class="overview-stat-card__label">{{ t('selfImprovementPage.statsRollbackRate') }}</div>
          <div class="overview-stat-card__value text-negative">{{ formatPct(stats?.rollbackRate) }}</div>
        </div>
        <div class="overview-stat-card">
          <div class="overview-stat-card__label">{{ t('selfImprovementPage.statsVerdicts') }}</div>
          <div class="overview-stat-card__value outcome-stats__verdicts">
            <span class="text-positive">{{ stats?.effective ?? 0 }}</span>
            <span class="overview-section-caption">/</span>
            <span>{{ stats?.neutral ?? 0 }}</span>
            <span class="overview-section-caption">/</span>
            <span class="text-negative">{{ stats?.regressed ?? 0 }}</span>
          </div>
        </div>
      </div>

      <div class="outcome-stats__charts">
        <q-card flat class="overview-panel outcome-stats__chart-card">
          <q-card-section>
            <div class="overview-section-title">{{ t('selfImprovementPage.statsVerdictDist') }}</div>
            <div class="overview-section-caption">{{ t('selfImprovementPage.statsVerdictDistHint') }}</div>
          </q-card-section>
          <q-card-section>
            <div v-if="!hasVerdicts" class="outcome-stats__empty overview-section-caption">
              {{ t('selfImprovementPage.statsEmpty') }}
            </div>
            <div v-else ref="verdictEl" class="outcome-stats__chart" />
          </q-card-section>
        </q-card>

        <q-card flat class="overview-panel outcome-stats__chart-card">
          <q-card-section>
            <div class="overview-section-title">{{ t('selfImprovementPage.statsByTrigger') }}</div>
            <div class="overview-section-caption">{{ t('selfImprovementPage.statsByTriggerHint') }}</div>
          </q-card-section>
          <q-card-section>
            <div v-if="!triggerRows.length" class="outcome-stats__empty overview-section-caption">
              {{ t('selfImprovementPage.statsEmpty') }}
            </div>
            <div v-else ref="triggerEl" class="outcome-stats__chart" />
          </q-card-section>
        </q-card>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { EChartsCoreOption } from 'echarts/core';
import type { SIOutcomeStats } from '../../features/self-improvement/types';
import { usageChartPalette } from '../../features/usage/usageEcharts';
import { useUsageChart } from '../../features/usage/useUsageChart';
import { siTriggerLabel } from './selfImprovementUi';

// OutcomeStatsPanel（73-self-iteration-v3 design §八）：Learn 阶段成效面板 —
// 4 统计卡 + verdict 分布环图 + 触发源堆叠条形图（复用 usage echarts 封装）。

const props = withDefaults(
  defineProps<{
    stats: SIOutcomeStats | null;
    /** 统计接口失败（P5.5）：区别于真实的 0 数据，整面板降级为「不可用」。 */
    failed?: boolean;
  }>(),
  { failed: false },
);

const { t } = useI18n();

const verdictEl = ref<HTMLElement | null>(null);
const triggerEl = ref<HTMLElement | null>(null);

const hasVerdicts = computed(() => (props.stats?.total ?? 0) > 0);
const triggerRows = computed(() => props.stats?.byTrigger ?? []);

function formatPct(v: number | undefined): string {
  return `${Math.round((v ?? 0) * 100)}%`;
}

function verdictOption(): EChartsCoreOption {
  const palette = usageChartPalette();
  const s = props.stats;
  return {
    textStyle: { color: palette.text, fontFamily: 'inherit' },
    tooltip: { trigger: 'item' },
    legend: { orient: 'vertical', right: 0, top: 'middle', textStyle: { color: palette.text } },
    series: [
      {
        type: 'pie',
        radius: ['42%', '68%'],
        center: ['38%', '50%'],
        avoidLabelOverlap: true,
        itemStyle: { borderRadius: 4 },
        label: { color: palette.text, formatter: '{b}\n{d}%' },
        data: [
          {
            name: t('selfImprovementPage.verdict.effective'),
            value: s?.effective ?? 0,
            itemStyle: { color: palette.positive },
          },
          {
            name: t('selfImprovementPage.verdict.neutral'),
            value: s?.neutral ?? 0,
            itemStyle: { color: palette.text },
          },
          {
            name: t('selfImprovementPage.verdict.regressed'),
            value: s?.regressed ?? 0,
            itemStyle: { color: palette.negative },
          },
        ],
      },
    ],
  };
}

function triggerOption(): EChartsCoreOption {
  const palette = usageChartPalette();
  const rows = triggerRows.value;
  const names = rows.map((r) => siTriggerLabel(t, r.triggerSource));
  const mk = (label: string, pick: (r: (typeof rows)[number]) => number, color: string) => ({
    name: label,
    type: 'bar' as const,
    stack: 'verdict',
    barMaxWidth: 28,
    itemStyle: { color },
    data: rows.map(pick),
  });
  return {
    textStyle: { color: palette.text, fontFamily: 'inherit' },
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    legend: { top: 0, textStyle: { color: palette.text } },
    grid: { left: 40, right: 16, top: 32, bottom: 28 },
    xAxis: {
      type: 'category',
      data: names,
      axisLabel: { color: palette.text, interval: 0 },
      axisLine: { lineStyle: { color: palette.border } },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel: { color: palette.text },
      splitLine: { lineStyle: { color: palette.border } },
    },
    series: [
      mk(t('selfImprovementPage.verdict.effective'), (r) => r.effective, palette.positive),
      mk(t('selfImprovementPage.verdict.neutral'), (r) => r.neutral, palette.text),
      mk(t('selfImprovementPage.verdict.regressed'), (r) => r.regressed, palette.negative),
    ],
  };
}

useUsageChart(verdictEl, verdictOption, () => [props.stats]);
useUsageChart(triggerEl, triggerOption, () => [props.stats]);
</script>

<style scoped lang="sass">
.outcome-stats__charts
  display: grid
  gap: 16px
  grid-template-columns: repeat(2, minmax(0, 1fr))

  @media (width <= 1023px)
    grid-template-columns: 1fr

.outcome-stats__chart
  width: 100%
  height: 260px

.outcome-stats__empty
  padding: 32px 0
  text-align: center

.outcome-stats__verdicts
  display: flex
  gap: 6px
  align-items: baseline

.outcome-stats__unavailable
  display: flex
  gap: 8px
  align-items: center
  justify-content: center
  padding: 24px 16px
  border-radius: 12px

.outcome-stats__unavailable-icon
  opacity: 0.6
</style>
