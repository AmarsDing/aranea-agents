import type { EChartsCoreOption } from 'echarts/core';
import { usageChartPalette } from './usageEcharts';
import { formatUsdCompact } from './moneyFormat';
import type { UsageBreakdownSlice } from './usageBreakdownSlices';

/** 圆环水平中心（与 legend 右栏布局对应），中心 overlay 定位复用同一常量。 */
export const USAGE_DONUT_CENTER_X = '25%';

const LEGEND_NAME_MAX = 16;

function truncateName(name: string): string {
  return name.length > LEGEND_NAME_MAX ? `${name.slice(0, LEGEND_NAME_MAX - 1)}…` : name;
}

/**
 * 费用占比圆环：无外部标签（避免窄面板重叠），
 * 右侧 legend 显示「截断名 + 百分比」，悬停 legend 可看全名。
 */
export function buildCostDonutOption(slices: UsageBreakdownSlice[]): EChartsCoreOption {
  const palette = usageChartPalette();
  const total = slices.reduce((sum, s) => sum + s.value, 0);
  const pctOf = (name: string) => {
    const s = slices.find((it) => it.name === name);
    return s && total > 0 ? (s.value / total) * 100 : 0;
  };
  return {
    textStyle: { color: palette.text, fontFamily: 'inherit' },
    tooltip: {
      trigger: 'item',
      valueFormatter: (v: number) => formatUsdCompact(v * 1_000_000),
    },
    legend: {
      orient: 'vertical',
      right: 0,
      top: 'middle',
      width: '56%',
      icon: 'circle',
      itemWidth: 8,
      itemHeight: 8,
      itemGap: 12,
      textStyle: { color: palette.text, fontSize: 11 },
      formatter: (name: string) => `${truncateName(name)}  ${pctOf(name).toFixed(1)}%`,
      tooltip: { show: true },
    },
    series: [
      {
        type: 'pie',
        radius: ['48%', '66%'],
        center: [USAGE_DONUT_CENTER_X, '50%'],
        itemStyle: {
          borderRadius: 6,
          borderColor: palette.surface,
          borderWidth: 2,
        },
        label: { show: false },
        labelLine: { show: false },
        emphasis: { scale: true, scaleSize: 4 },
        data: slices.map((s, i) => ({
          name: s.name,
          value: s.value,
          itemStyle: { color: palette.series[i % palette.series.length] },
        })),
      },
    ],
  };
}

/** 圆环中心合计文案（USD）。 */
export function costDonutTotalLabel(slices: UsageBreakdownSlice[]): string {
  const total = slices.reduce((sum, s) => sum + s.value, 0);
  return formatUsdCompact(total * 1_000_000);
}
