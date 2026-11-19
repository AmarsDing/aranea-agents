import { describe, expect, it } from 'vitest';
import { buildMetricsMatrix, metricTone, type MatrixWindowSlice } from '../metricsMatrix';
import type { RunnerMetricsSummary } from '../types';

function metrics(partial: Partial<RunnerMetricsSummary> = {}): RunnerMetricsSummary {
  return {
    window_minutes: 60,
    total_runs: 100,
    error_runs: 5,
    error_rate: 0.05,
    success_rate: 0.95,
    avg_duration_ms: 800,
    p50_duration_ms: 600,
    p95_duration_ms: 2000,
    p99_duration_ms: 3000,
    ...partial,
  };
}

function slice(windowMinutes: number, m: Partial<RunnerMetricsSummary> | null, extra: Partial<MatrixWindowSlice> = {}) {
  return {
    windowMinutes,
    metrics: m ? metrics({ window_minutes: windowMinutes, ...m }) : null,
    ...extra,
  } satisfies MatrixWindowSlice;
}

describe('metricTone', () => {
  it('失败率 / 成功率 / P95 各自阈值', () => {
    expect(metricTone('error_rate', 0.005)).toBe('');
    expect(metricTone('error_rate', 0.02)).toBe('warn');
    expect(metricTone('error_rate', 0.09)).toBe('danger');

    expect(metricTone('success_rate', 0.99)).toBe('');
    expect(metricTone('success_rate', 0.93)).toBe('warn');
    expect(metricTone('success_rate', 0.8)).toBe('danger');

    expect(metricTone('p95_duration_ms', 1200)).toBe('');
    expect(metricTone('p95_duration_ms', 4000)).toBe('warn');
    expect(metricTone('p95_duration_ms', 12000)).toBe('danger');
  });

  it('计数类指标不设阈值，null 一律无色', () => {
    expect(metricTone('total_runs', 999999)).toBe('');
    expect(metricTone('error_runs', 999999)).toBe('');
    expect(metricTone('error_rate', null)).toBe('');
  });
});

describe('buildMetricsMatrix', () => {
  it('窗口升序成列，指标顺序固定，缺数据的窗口整列空', () => {
    const matrix = buildMetricsMatrix([slice(360, { total_runs: 10 }), slice(60, null), slice(15, { total_runs: 3 })]);

    expect(matrix.windows).toEqual([15, 60, 360]);
    expect(matrix.rows.map((r) => r.key)).toEqual([
      'total_runs',
      'error_runs',
      'error_rate',
      'success_rate',
      'p95_duration_ms',
    ]);

    const totalRow = matrix.rows[0];
    expect(totalRow.cells.map((c) => c.value)).toEqual([3, null, 10]);
    expect(totalRow.cells[1].formatted).toBe('—');
  });

  it('delta 以最长窗口为基准，比率用百分点', () => {
    const matrix = buildMetricsMatrix([
      slice(60, { error_rate: 0.11, success_rate: 0.89 }),
      slice(1440, { error_rate: 0.05, success_rate: 0.95 }),
    ]);

    const errorRow = matrix.rows.find((r) => r.key === 'error_rate')!;
    expect(errorRow.cells[0].delta).toBe(6); // 11% - 5% = +6pp
    expect(errorRow.cells[1].delta).toBe(0);

    const successRow = matrix.rows.find((r) => r.key === 'success_rate')!;
    expect(successRow.cells[0].delta).toBe(-6);
  });

  it('每行最差单元格标记 worst，tone 与数值一致', () => {
    const matrix = buildMetricsMatrix([
      slice(15, { error_rate: 0.01 }),
      slice(1440, { error_rate: 0.08 }),
    ]);

    const errorRow = matrix.rows.find((r) => r.key === 'error_rate')!;
    expect(errorRow.cells[1].worst).toBe(true);
    expect(errorRow.cells[1].tone).toBe('danger');
    expect(errorRow.cells[0].worst).toBe(false);
  });

  it('worstCell 暴露需要置顶提示的最严重单元格（失败率优先）', () => {
    const matrix = buildMetricsMatrix([
      slice(60, { error_rate: 0.2, p95_duration_ms: 20000 }),
      slice(1440, { error_rate: 0.02, p95_duration_ms: 20000 }),
    ]);

    expect(matrix.worstCell?.key).toBe('error_rate');
    expect(matrix.worstCell?.windowMinutes).toBe(60);
    expect(matrix.worstCell?.tone).toBe('danger');
  });

  it('零运行窗口的比率与 P95 视为无数据，不误报「成功率 0%」危险', () => {
    const matrix = buildMetricsMatrix([
      slice(15, { total_runs: 0, error_runs: 0, error_rate: 0, success_rate: 0, p95_duration_ms: 0 }),
      slice(1440, { total_runs: 20 }),
    ]);

    const successRow = matrix.rows.find((r) => r.key === 'success_rate')!;
    expect(successRow.cells[0].value).toBeNull();
    expect(successRow.cells[0].formatted).toBe('—');
    expect(successRow.cells[0].tone).toBe('');
    // 危险提示只能来自真实有运行的窗口，不能由 0/0 触发
    expect(matrix.worstCell?.windowMinutes).not.toBe(15);

    // 运行数本身仍是真实 0，不隐藏
    expect(matrix.rows.find((r) => r.key === 'total_runs')?.cells[0].formatted).toBe('0');
  });

  it('全部窗口无数据时 worstCell 为 null 且 hasData=false', () => {
    const matrix = buildMetricsMatrix([slice(60, null), slice(1440, null)]);

    expect(matrix.worstCell).toBeNull();
    expect(matrix.hasData).toBe(false);
  });

  it('加载失败信息随列透出，供列头 role=alert 与重试', () => {
    const matrix = buildMetricsMatrix([slice(60, null, { error: 'HTTP 500' }), slice(1440, { total_runs: 1 })]);

    expect(matrix.columns.find((c) => c.windowMinutes === 60)?.error).toBe('HTTP 500');
    expect(matrix.columns.find((c) => c.windowMinutes === 1440)?.error).toBe('');
  });
});
