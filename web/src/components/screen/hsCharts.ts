/** 大屏显示的 ECharts 配置。颜色在每次渲染时从 --nt-* 令牌读取，昼夜切换后重画即跟着变。 */
import type { EChartsCoreOption } from 'echarts/core';
import { readCssVar } from '../../utils/cssVar';
import { formatCount, formatUsdCompactFromUsd } from '../../features/usage/moneyFormat';

const css = (name: string) => readCssVar(name, '');

/** 分类色：与图例圆点（.hs-dot--N）同一组令牌，顺序一致。 */
export function seriesColors(): string[] {
  return [1, 2, 3, 4, 5, 6].map((i) => css(`--nt-line-${i}`));
}

function tooltipBase() {
  return {
    backgroundColor: css('--nt-bg-2'),
    borderColor: css('--nt-border-2'),
    textStyle: { color: css('--nt-text-1'), fontSize: 12 },
  };
}

export function donutOption(items: Array<{ label: string; share: number; tip: string }>): EChartsCoreOption {
  const colors = seriesColors();
  return {
    tooltip: {
      ...tooltipBase(),
      trigger: 'item',
      formatter: (p: { dataIndex: number }) => items[p.dataIndex]?.tip ?? '',
    },
    series: [
      {
        type: 'pie',
        radius: ['64%', '88%'],
        avoidLabelOverlap: false,
        label: { show: false },
        labelLine: { show: false },
        itemStyle: { borderColor: css('--nt-bg-1'), borderWidth: 2 },
        emphasis: { scale: true, scaleSize: 4 },
        data: items.map((it, i) => ({
          name: it.label,
          value: it.share,
          itemStyle: { color: colors[i % colors.length] },
        })),
      },
    ],
  };
}

export function trendOption(
  labels: string[],
  calls: number[],
  costsUsd: number[],
  names: { calls: string; cost: string },
): EChartsCoreOption {
  const axisText = css('--nt-text-3');
  const grid = css('--nt-border-1');
  const accent = css('--nt-accent');
  return {
    textStyle: { fontFamily: 'inherit' },
    grid: { left: 8, right: 8, top: 30, bottom: 4, containLabel: true },
    legend: {
      top: 0,
      right: 0,
      itemWidth: 10,
      itemHeight: 10,
      textStyle: { color: css('--nt-text-2'), fontSize: 12 },
    },
    tooltip: {
      ...tooltipBase(),
      trigger: 'axis',
      formatter: (
        ps: Array<{ axisValue: string; seriesIndex: number; value: number; marker: string; seriesName: string }>,
      ) =>
        [
          ps[0]?.axisValue ?? '',
          ...ps.map(
            (p) =>
              `${p.marker}${p.seriesName}  ${p.seriesIndex === 0 ? formatCount(p.value) : formatUsdCompactFromUsd(p.value)}`,
          ),
        ].join('<br/>'),
    },
    xAxis: {
      type: 'category',
      data: labels,
      axisLine: { lineStyle: { color: grid } },
      axisTick: { show: false },
      axisLabel: { color: axisText, fontSize: 11, hideOverlap: true },
    },
    yAxis: [
      {
        type: 'value',
        splitLine: { lineStyle: { color: grid, type: 'dashed' } },
        axisLabel: { color: axisText, fontSize: 11, formatter: (v: number) => formatCount(v) },
      },
      {
        type: 'value',
        splitLine: { show: false },
        axisLabel: { color: axisText, fontSize: 11, formatter: (v: number) => formatUsdCompactFromUsd(v) },
      },
    ],
    series: [
      {
        name: names.calls,
        type: 'bar',
        data: calls,
        barMaxWidth: 14,
        itemStyle: { color: css('--nt-accent-2'), opacity: 0.55, borderRadius: [3, 3, 0, 0] },
      },
      {
        name: names.cost,
        type: 'line',
        yAxisIndex: 1,
        data: costsUsd,
        smooth: true,
        symbol: 'circle',
        symbolSize: 5,
        showSymbol: labels.length <= 31,
        lineStyle: { color: accent, width: 2 },
        itemStyle: { color: accent },
        areaStyle: { color: accent, opacity: 0.12 },
      },
    ],
  };
}
