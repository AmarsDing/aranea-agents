import type { ModelUsageTrendPoint } from './types';

export type UsageTrendMetric = 'tokens' | 'calls' | 'cost' | 'success_rate';

export const USAGE_TREND_METRIC_OPTIONS: { label: string; value: UsageTrendMetric }[] = [
  { label: 'Token', value: 'tokens' },
  { label: '调用次数', value: 'calls' },
  { label: '费用', value: 'cost' },
  { label: '成功率', value: 'success_rate' },
];

export function formatTrendLabel(key: string, hourly: boolean): string {
  if (hourly && key.length >= 13) return key.slice(11, 16);
  return key.length >= 10 ? key.slice(5) : key;
}

export function trendSuccessRate(point: ModelUsageTrendPoint): number {
  const ok = point.success_count ?? 0;
  const bad = (point.failed_count ?? 0) + (point.cancelled_count ?? 0);
  const total = ok + bad;
  if (total <= 0) return 0;
  return ok / total;
}

export function trendMetricValue(point: ModelUsageTrendPoint, metric: UsageTrendMetric): number {
  switch (metric) {
    case 'tokens':
      return point.total_tokens ?? 0;
    case 'calls':
      return point.call_count ?? 0;
    case 'cost':
      return (point.total_cost_micro_usd ?? 0) / 1_000_000;
    // success_rate 由 successRateStackFromPoint 处理（堆叠柱状图），不在此函数取值
    default:
      return 0;
  }
}

export type SuccessRateStackPoint = { successPct: number; failurePct: number };

export function successRateStackFromPoint(point: ModelUsageTrendPoint): SuccessRateStackPoint {
  const rate = trendSuccessRate(point);
  return {
    successPct: Math.round(rate * 1000) / 10,
    failurePct: Math.round((1 - rate) * 1000) / 10,
  };
}

export function trendMetricYAxisName(metric: UsageTrendMetric): string {
  switch (metric) {
    case 'tokens':
      return 'Token';
    case 'calls':
      return '次';
    case 'cost':
      return 'USD';
    case 'success_rate':
      return '%';
    default:
      return '';
  }
}
