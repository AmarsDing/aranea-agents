<template>
  <div class="skill-stats-hover" @mouseenter="ensureLoaded">
    <skill-stats-strip :skill="skill" />
    <q-tooltip
      class="skill-stats-tooltip"
      anchor="bottom middle"
      self="top middle"
      :offset="[0, 6]"
      :delay="150"
      @before-show="ensureLoaded"
    >
      <div class="skill-stats-panel">
        <div class="skill-stats-panel__head">
          <span class="skill-stats-panel__title">{{ t('skillsPage.statsTitle') }}</span>
          <span class="skill-stats-panel__name">{{ skill.name }}</span>
        </div>
        <div v-if="loadingHealth" class="skill-stats-panel__state">{{ t('skillsPage.statsLoading') }}</div>
        <div v-else-if="!hasData" class="skill-stats-panel__state">{{ t('skillsPage.statsEmpty') }}</div>
        <template v-else>
          <div class="skill-stats-panel__section">{{ t('skillsPage.statsTrendTitle') }}</div>
          <div ref="trendEl" class="skill-stats-panel__trend" />
          <div class="skill-stats-panel__row">
            <div class="skill-stats-panel__donut-wrap">
              <div ref="donutEl" class="skill-stats-panel__donut" />
              <div class="skill-stats-panel__donut-center">
                <b>{{ formatPct(successRate7d) }}</b>
                <span>{{ t('skillsPage.statsSuccessRate') }}</span>
              </div>
            </div>
            <div class="skill-stats-panel__metrics">
              <div class="skill-stats-panel__metric">
                <span>{{ t('skillsPage.statsCalls7d') }}</span
                ><b>{{ health?.total_invocations_7d ?? 0 }}</b>
              </div>
              <div class="skill-stats-panel__metric">
                <span>{{ t('skillsPage.statsCalls30d') }}</span
                ><b>{{ health?.total_invocations_30d ?? 0 }}</b>
              </div>
              <div class="skill-stats-panel__metric">
                <span>{{ t('skillsPage.statsP95') }}</span
                ><b>{{ formatMs(health?.p95_duration_ms_7d) }}</b>
              </div>
              <div class="skill-stats-panel__metric">
                <span>{{ t('skillsPage.statsRouteHit') }}</span
                ><b>{{ formatPct(health?.route_hit_rate_7d ?? 0) }}</b>
              </div>
            </div>
          </div>
        </template>
      </div>
    </q-tooltip>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { EChartsCoreOption } from 'echarts/core';
import SkillStatsStrip from './SkillStatsStrip.vue';
import type { Skill, SkillHealthMetric } from '../../features/skills/types';
import { usageChartPalette } from '../../features/usage/usageEcharts';
import { useUsageChart } from '../../features/usage/useUsageChart';

const { t, locale } = useI18n();

const props = defineProps<{
  skill: Skill;
  /** 懒加载健康数据（由 Page/composable 注入 store 方法，展示层不直连 store）。 */
  loadHealth: (skillId: string) => Promise<SkillHealthMetric>;
}>();

const health = ref<SkillHealthMetric | null>(null);
const loadingHealth = ref(false);
let requested = false;

/** 首次悬停才拉取；组件实例随行复用，天然按行缓存。 */
function ensureLoaded() {
  if (requested) return;
  requested = true;
  loadingHealth.value = true;
  props
    .loadHealth(props.skill.id)
    .then((h) => {
      health.value = h;
    })
    .catch(() => {
      health.value = null;
    })
    .finally(() => {
      loadingHealth.value = false;
    });
}

const trendDays = computed(() => (health.value?.daily_metrics ?? []).slice(-7));
const successRate7d = computed(() => health.value?.success_rate_7d ?? 0);
const hasData = computed(() => (health.value?.total_invocations_7d ?? 0) > 0 || trendDays.value.length > 0);

function formatPct(v: number) {
  return `${Math.round((v || 0) * 100)}%`;
}

function formatMs(v?: number) {
  if (!v || v <= 0) return '-';
  if (v < 1000) return `${Math.round(v)}ms`;
  return `${(v / 1000).toFixed(1)}s`;
}

const trendEl = ref<HTMLElement | null>(null);
const donutEl = ref<HTMLElement | null>(null);

function trendOption(): EChartsCoreOption {
  const p = usageChartPalette();
  return {
    textStyle: { color: p.text, fontFamily: 'inherit' },
    grid: { left: 32, right: 8, top: 24, bottom: 20 },
    tooltip: { trigger: 'axis' },
    legend: {
      top: 0,
      right: 0,
      itemWidth: 10,
      itemHeight: 10,
      textStyle: { color: p.text, fontSize: 11 },
    },
    xAxis: {
      type: 'category',
      data: trendDays.value.map((d) => d.date.slice(5)),
      axisLabel: { color: p.text, fontSize: 10 },
      axisLine: { lineStyle: { color: p.border } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel: { color: p.text, fontSize: 10 },
      splitLine: { lineStyle: { color: p.border } },
    },
    series: [
      {
        name: t('skillsPage.statsSuccess'),
        type: 'bar',
        stack: 'calls',
        barWidth: 14,
        itemStyle: { color: p.positive },
        data: trendDays.value.map((d) => d.successes),
      },
      {
        name: t('skillsPage.statsFailure'),
        type: 'bar',
        stack: 'calls',
        barWidth: 14,
        itemStyle: { color: p.negative, borderRadius: [3, 3, 0, 0] },
        data: trendDays.value.map((d) => Math.max(0, d.invocations - d.successes)),
      },
    ],
  };
}

function donutOption(): EChartsCoreOption {
  const p = usageChartPalette();
  const ok = health.value?.success_count_7d ?? 0;
  const total = health.value?.total_invocations_7d ?? 0;
  const fail = Math.max(0, total - ok);
  return {
    textStyle: { color: p.text, fontFamily: 'inherit' },
    tooltip: { trigger: 'item', valueFormatter: (v: number) => t('skillsPage.statsTimes', { n: v }) },
    series: [
      {
        type: 'pie',
        radius: ['58%', '82%'],
        center: ['50%', '50%'],
        label: { show: false },
        itemStyle: { borderRadius: 3 },
        data: [
          { name: t('skillsPage.statsSuccess'), value: ok, itemStyle: { color: p.positive } },
          { name: t('skillsPage.statsFailure'), value: fail, itemStyle: { color: p.negative } },
        ],
      },
    ],
  };
}

useUsageChart(trendEl, trendOption, () => [trendDays.value, locale.value]);
useUsageChart(donutEl, donutOption, () => [health.value, locale.value]);
</script>

<style scoped lang="sass">
.skill-stats-hover
  display: inline-block

// q-tooltip 默认字号/宽度偏小，图形面板需要自定义
:global(.skill-stats-tooltip)
  max-width: 420px
  padding: 12px 14px
  font-size: 12px
  line-height: 1.5
  border-radius: 14px

.skill-stats-panel
  width: 380px
  max-width: 78vw

.skill-stats-panel__head
  display: flex
  align-items: baseline
  gap: 8px
  margin-bottom: 6px

.skill-stats-panel__title
  font-weight: 700

.skill-stats-panel__name
  opacity: 0.75
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.skill-stats-panel__section
  margin: 6px 0 2px
  font-weight: 600
  opacity: 0.8

.skill-stats-panel__state
  padding: 18px 0
  text-align: center
  opacity: 0.75

.skill-stats-panel__trend
  width: 100%
  height: 130px

.skill-stats-panel__row
  display: flex
  align-items: center
  gap: 14px
  margin-top: 8px

.skill-stats-panel__donut-wrap
  position: relative
  flex-shrink: 0
  width: 108px
  height: 108px

.skill-stats-panel__donut
  width: 108px
  height: 108px

.skill-stats-panel__donut-center
  position: absolute
  inset: 0
  display: flex
  flex-direction: column
  align-items: center
  justify-content: center
  pointer-events: none

  b
    font-size: 15px

  span
    font-size: 10px
    opacity: 0.7

.skill-stats-panel__metrics
  flex: 1
  display: grid
  grid-template-columns: 1fr 1fr
  gap: 6px 12px

.skill-stats-panel__metric
  display: flex
  flex-direction: column

  span
    opacity: 0.7

  b
    font-size: 14px
</style>
