import { describe, expect, it } from 'vitest';
import { successRateStackFromPoint, trendMetricValue, trendSuccessRate } from '../usageTrendMetrics';
import type { ModelUsageTrendPoint } from '../types';

const point: ModelUsageTrendPoint = {
  date_key: '2026-05-21',
  call_count: 10,
  input_tokens: 100,
  output_tokens: 50,
  total_tokens: 150,
  total_cost_micro_usd: 2_500_000,
  success_count: 8,
  failed_count: 2,
  cancelled_count: 0,
  avg_latency_ms: 120,
  avg_tokens_per_second: 12,
};

describe('usageTrendMetrics', () => {
  it('computes success rate from counts', () => {
    expect(trendSuccessRate(point)).toBeCloseTo(0.8);
  });

  it('builds stacked success/failure percentages', () => {
    const stack = successRateStackFromPoint(point);
    expect(stack.successPct).toBeCloseTo(80);
    expect(stack.failurePct).toBeCloseTo(20);
  });

  it('maps metric values', () => {
    expect(trendMetricValue(point, 'tokens')).toBe(150);
    expect(trendMetricValue(point, 'calls')).toBe(10);
    expect(trendMetricValue(point, 'cost')).toBeCloseTo(2.5);
    expect(trendMetricValue(point, 'success_rate')).toBeCloseTo(80);
  });
});
