/**
 * metricsMatrix：把多个时间窗的 Runner 指标拼成「指标 × 窗口」矩阵（纯函数）。
 *
 * 为什么需要：单窗口 stat 卡只能回答「现在多少」，回答不了「变差了没有」。
 * 矩阵同屏给出 15m/1h/6h/24h 四列，以最长窗口为基准算差值，
 * 并把最严重的单元格（恶化）暴露给页面做置顶提示与下钻。
 */
import type { RunnerMetricsSummary } from './types';

export type MatrixMetricKey = 'total_runs' | 'error_runs' | 'error_rate' | 'success_rate' | 'p95_duration_ms';
export type MatrixTone = '' | 'warn' | 'danger';

export type MatrixWindowSlice = {
  windowMinutes: number;
  metrics: RunnerMetricsSummary | null;
  loading?: boolean;
  error?: string;
};

export type MatrixCell = {
  key: MatrixMetricKey;
  windowMinutes: number;
  /** 原始数值（比率类为 0-1）；缺数据显示 null */
  value: number | null;
  formatted: string;
  tone: MatrixTone;
  /** 与基准窗口（最长窗口）的差：比率为百分点，其余为同单位差值 */
  delta: number | null;
  /** 该行最差单元格 */
  worst: boolean;
};

export type MatrixRow = {
  key: MatrixMetricKey;
  /** 数值越大越差 = 'up'，越小越差 = 'down'，无好坏 = 'none' */
  badDirection: 'up' | 'down' | 'none';
  cells: MatrixCell[];
};

export type MatrixColumn = {
  windowMinutes: number;
  loading: boolean;
  error: string;
};

export type MetricsMatrix = {
  windows: number[];
  columns: MatrixColumn[];
  rows: MatrixRow[];
  /** 需要置顶提示的最严重单元格（failure rate 优先），无恶化时为 null */
  worstCell: MatrixCell | null;
  hasData: boolean;
};

/** 指标定义顺序（渲染顺序即此顺序）。 */
export const MATRIX_METRICS: { key: MatrixMetricKey; badDirection: 'up' | 'down' | 'none' }[] = [
  { key: 'total_runs', badDirection: 'none' },
  { key: 'error_runs', badDirection: 'up' },
  { key: 'error_rate', badDirection: 'up' },
  { key: 'success_rate', badDirection: 'down' },
  { key: 'p95_duration_ms', badDirection: 'up' },
];

/** 阈值：失败率 1%/5%，成功率 95%/90%，P95 3s/10s。 */
export function metricTone(key: MatrixMetricKey, value: number | null): MatrixTone {
  if (value == null || !Number.isFinite(value)) return '';
  switch (key) {
    case 'error_rate':
      if (value >= 0.05) return 'danger';
      return value >= 0.01 ? 'warn' : '';
    case 'success_rate':
      if (value <= 0.9) return 'danger';
      return value <= 0.95 ? 'warn' : '';
    case 'p95_duration_ms':
      if (value >= 10000) return 'danger';
      return value >= 3000 ? 'warn' : '';
    default:
      return '';
  }
}

export function formatMetric(key: MatrixMetricKey, value: number | null): string {
  if (value == null || !Number.isFinite(value)) return '—';
  switch (key) {
    case 'error_rate':
    case 'success_rate':
      return `${(value * 100).toFixed(1)}%`;
    case 'p95_duration_ms':
      return value >= 1000 ? `${(value / 1000).toFixed(1)}s` : `${Math.round(value)}ms`;
    default:
      return String(Math.round(value));
  }
}

/** 单元格差值展示：比率为百分点，其余为同单位差值。 */
export function formatDelta(key: MatrixMetricKey, delta: number | null): string {
  if (delta == null || !Number.isFinite(delta) || delta === 0) return '';
  const sign = delta > 0 ? '+' : '';
  const isRate = key === 'error_rate' || key === 'success_rate';
  const body = isRate ? delta.toFixed(1) : String(Math.round(delta));
  return `${sign}${body}${isRate ? 'pp' : key === 'p95_duration_ms' ? 'ms' : ''}`;
}

function rawValue(m: RunnerMetricsSummary, key: MatrixMetricKey): number | null {
  const v = m[key];
  return typeof v === 'number' && Number.isFinite(v) ? v : null;
}

/** 零运行窗口的比率/P95 一律视为无数据：0/0 不是「成功率 0%」。 */
function cellValue(m: RunnerMetricsSummary | null, key: MatrixMetricKey): number | null {
  if (!m) return null;
  const isRatioOrLatency = key === 'error_rate' || key === 'success_rate' || key === 'p95_duration_ms';
  if (isRatioOrLatency && !(m.total_runs > 0)) return null;
  return rawValue(m, key);
}

/** 差值：比率为百分点（×100），其余同单位。 */
function deltaOf(key: MatrixMetricKey, value: number | null, base: number | null): number | null {
  if (value == null || base == null) return null;
  const isRate = key === 'error_rate' || key === 'success_rate';
  const diff = isRate ? (value - base) * 100 : value - base;
  return Math.round(diff * 10) / 10;
}

function badness(key: MatrixMetricKey, value: number | null): number {
  if (value == null) return Number.NEGATIVE_INFINITY;
  if (key === 'success_rate') return -value;
  return value;
}

export function buildMetricsMatrix(slices: MatrixWindowSlice[]): MetricsMatrix {
  const ordered = [...slices].sort((a, b) => a.windowMinutes - b.windowMinutes);
  const windows = ordered.map((s) => s.windowMinutes);
  const baseline = [...ordered].reverse().find((s) => s.metrics) ?? null;

  const rows: MatrixRow[] = MATRIX_METRICS.map((def) => {
    const cells: MatrixCell[] = ordered.map((s) => {
      const value = cellValue(s.metrics, def.key);
      return {
        key: def.key,
        windowMinutes: s.windowMinutes,
        value,
        formatted: formatMetric(def.key, value),
        tone: metricTone(def.key, value),
        delta: baseline ? deltaOf(def.key, value, cellValue(baseline.metrics, def.key)) : null,
        worst: false,
      };
    });

    if (def.badDirection !== 'none') {
      const withData = cells.filter((c) => c.value != null);
      if (withData.length > 1) {
        const worst = withData.reduce((a, b) => (badness(def.key, b.value) > badness(def.key, a.value) ? b : a));
        if (worst.tone !== '') worst.worst = true;
      }
    }
    return { key: def.key, badDirection: def.badDirection, cells };
  });

  const worstCell =
    rows.find((r) => r.key === 'error_rate')?.cells.find((c) => c.tone === 'danger') ??
    rows.find((r) => r.key === 'success_rate')?.cells.find((c) => c.tone === 'danger') ??
    rows.find((r) => r.key === 'p95_duration_ms')?.cells.find((c) => c.tone === 'danger') ??
    null;

  return {
    windows,
    columns: ordered.map((s) => ({
      windowMinutes: s.windowMinutes,
      loading: Boolean(s.loading),
      error: s.error ?? '',
    })),
    rows,
    worstCell,
    hasData: ordered.some((s) => s.metrics != null),
  };
}
