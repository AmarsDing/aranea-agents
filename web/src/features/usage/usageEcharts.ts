import * as echarts from 'echarts/core';
import { BarChart, LineChart, PieChart } from 'echarts/charts';
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';
import type { EChartsCoreOption } from 'echarts/core';

let registered = false;

export function ensureUsageEcharts(): void {
  if (registered) return;
  echarts.use([BarChart, LineChart, PieChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer]);
  registered = true;
}

export function readCssVar(name: string, fallback: string): string {
  if (typeof document === 'undefined') return fallback;
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return v || fallback;
}

export function usageChartPalette() {
  return {
    accent: readCssVar('--color-accent', '#e9a23b'),
    positive: readCssVar('--color-success', '#4CAF7C'),
    negative: readCssVar('--color-danger', '#E55C5C'),
    text: readCssVar('--color-text-secondary', '#6b7280'),
    border: readCssVar('--color-border-subtle', 'rgba(0,0,0,0.08)'),
    series: [
      readCssVar('--color-accent', '#e9a23b'),
      readCssVar('--color-accent-hover', '#d48c1a'),
      '#60a5fa',
      '#a78bfa',
      '#34d399',
    ],
  };
}

export function baseChartOption(partial: EChartsCoreOption = {}): EChartsCoreOption {
  const palette = usageChartPalette();
  return {
    textStyle: { color: palette.text, fontFamily: 'inherit' },
    grid: { left: 48, right: 16, top: 40, bottom: 28 },
    tooltip: { trigger: 'axis' },
    ...partial,
  };
}

export { echarts };
