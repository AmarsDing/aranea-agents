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

/**
 * 读取 CSS 变量。夜间 token 定义在 body.body--dark 上，
 * 必须从 body 读取才能拿到昼夜覆盖（html 读不到 body 上的变量）。
 */
export function readCssVar(name: string, fallback: string): string {
  if (typeof document === 'undefined') return fallback;
  const el = document.body ?? document.documentElement;
  const v = getComputedStyle(el).getPropertyValue(name).trim();
  return v || fallback;
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
