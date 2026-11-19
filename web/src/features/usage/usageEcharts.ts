import * as echarts from 'echarts/core';
import { BarChart, LineChart, PieChart } from 'echarts/charts';
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';
import type { EChartsCoreOption } from 'echarts/core';
import { readCssVar } from '../../utils/cssVar';

export { readCssVar };

let registered = false;

export function ensureUsageEcharts(): void {
  if (registered) return;
  echarts.use([BarChart, LineChart, PieChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer]);
  registered = true;
}

export function usageChartPalette() {
  return {
    accent: readCssVar('--color-accent', '#e9a23b'),
    positive: readCssVar('--color-success', '#4CAF7C'),
    negative: readCssVar('--color-danger', '#E55C5C'),
    text: readCssVar('--color-text-secondary', '#6b7280'),
    heading: readCssVar('--color-text-heading', '#1a2030'),
    border: readCssVar('--glass-border', 'rgba(128,128,128,0.18)'),
    surface: readCssVar('--canvas-base', '#ffffff'),
    /** 分类色板：复用 --chart-color-* token，昼夜各自协调。 */
    series: [
      readCssVar('--chart-color-memory', '#06b6d4'),
      readCssVar('--chart-color-skills', '#8b5cf6'),
      readCssVar('--chart-color-system-prompt', '#e9a23b'),
      readCssVar('--chart-color-session-summary', '#10b981'),
      readCssVar('--chart-color-user-message', '#ec4899'),
      readCssVar('--chart-color-history', '#6b7280'),
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
