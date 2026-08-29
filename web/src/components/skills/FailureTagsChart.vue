<template>
  <q-card flat class="overview-panel">
    <q-card-section>
      <div class="text-h6 overview-section-title">失败标签分布</div>
      <div class="text-caption overview-section-caption">仅统计当前筛选条件下的失败记录</div>
    </q-card-section>
    <q-separator class="overview-separator" />
    <q-card-section>
      <div v-if="!slices.length" class="overview-empty">暂无失败标签数据</div>
      <template v-else>
        <div ref="chartEl" class="failure-tags-chart" />
        <div v-if="unknownOnly" class="failure-tags-chart__hint">
          {{ unknownCount }} 条失败记录未能自动归因，建议检查归因规则或补充根因分析。
        </div>
      </template>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { EChartsCoreOption } from 'echarts/core';
import { usageChartPalette } from '../../features/usage/usageEcharts';
import { useUsageChart } from '../../features/usage/useUsageChart';

const { t } = useI18n();

const props = defineProps<{
  failureTags: Record<string, number>;
}>();

const chartEl = ref<HTMLElement | null>(null);

/** 失败标签 → i18n key 映射（与 biz/skill_scoring.go 的 FailureTag 常量集对齐；未收录的标签原样展示） */
const TAG_KEYS: Record<string, string> = {
  param_mismatch: 'skillsPage.failureTagParamMismatch',
  wrong_tool_choice: 'skillsPage.failureTagWrongToolChoice',
  tool_timeout: 'skillsPage.failureTagToolTimeout',
  tool_api_error: 'skillsPage.failureTagToolApiError',
  context_overflow: 'skillsPage.failureTagContextOverflow',
  instruction_ambiguity: 'skillsPage.failureTagInstructionAmbiguity',
  user_cancelled: 'skillsPage.failureTagUserCancelled',
  unknown: 'skillsPage.failureTagUnknown',
};

const UNKNOWN_TAG = 'unknown';

const slices = computed(() =>
  Object.entries(props.failureTags)
    .map(([name, value]) => ({ name, label: TAG_KEYS[name] ? t(TAG_KEYS[name]) : name, value }))
    .sort((a, b) => b.value - a.value),
);

const unknownOnly = computed(() => slices.value.length > 0 && slices.value.every((s) => s.name === UNKNOWN_TAG));
const unknownCount = computed(() => slices.value.find((s) => s.name === UNKNOWN_TAG)?.value ?? 0);

function barOption(): EChartsCoreOption {
  const palette = usageChartPalette();
  // 未分类用灰色弱化，有效归因标签使用分类色板
  const knownSlices = slices.value.filter((s) => s.name !== UNKNOWN_TAG);
  return {
    textStyle: { color: palette.text, fontFamily: 'inherit' },
    grid: { left: 8, right: 40, top: 8, bottom: 8, containLabel: true },
    tooltip: { trigger: 'item', valueFormatter: (v: number) => t('skillsPage.statsTimes', { n: v }) },
    xAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel: { color: palette.text },
      splitLine: { lineStyle: { color: palette.border } },
    },
    yAxis: {
      type: 'category',
      inverse: true,
      data: slices.value.map((s) => s.label),
      axisLabel: { color: palette.text },
      axisLine: { lineStyle: { color: palette.border } },
      axisTick: { show: false },
    },
    series: [
      {
        type: 'bar',
        barWidth: 16,
        itemStyle: { borderRadius: [0, 4, 4, 0] },
        label: { show: true, position: 'right', color: palette.text },
        data: slices.value.map((s) => ({
          name: s.label,
          value: s.value,
          itemStyle: {
            color:
              s.name === UNKNOWN_TAG
                ? palette.series[5]
                : palette.series[knownSlices.findIndex((k) => k.name === s.name) % palette.series.length],
          },
        })),
      },
    ],
  };
}

useUsageChart(chartEl, barOption, () => [slices.value]);
</script>

<style scoped lang="sass">
.failure-tags-chart
  width: 100%
  height: 260px

.failure-tags-chart__hint
  margin-top: 8px
  font-size: 0.78rem
  color: var(--color-text-secondary)
</style>
