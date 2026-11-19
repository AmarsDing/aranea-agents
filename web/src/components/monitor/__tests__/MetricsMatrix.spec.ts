import { afterEach, describe, expect, it } from 'vitest';
import { mount, type VueWrapper } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import MetricsMatrix from '../MetricsMatrix.vue';
import type { MatrixWindowSlice } from '../../../features/monitor/metricsMatrix';
import type { RunnerMetricsSummary } from '../../../features/monitor/types';

const stubs = {
  'q-icon': { template: '<i />' },
  'q-btn': {
    props: ['label', 'icon'],
    emits: ['click'],
    template: '<button type="button" v-bind="$attrs" @click="$emit(\'click\')">{{ label || icon }}</button>',
  },
  'q-skeleton': { template: '<div class="skeleton" />' },
};

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  messages: {
    'zh-CN': {
      shell: { state: { emptyTitle: '暂无数据', emptyDesc: '', loading: '加载中' } },
      monitorPage: {
        runner: {
          totalRuns: '总运行次数',
          errorRuns: '错误次数',
          errorRate: '错误率',
          successRate: '成功率',
          p95Latency: 'P95 延迟',
          windowOption: { m15: '15 分钟', h1: '1 小时', h6: '6 小时', h24: '24 小时' },
          matrix: {
            title: '多窗口指标矩阵',
            subtitle: '同屏对比 15 分钟到 24 小时，标出恶化窗口',
            metricColumn: '指标',
            baselineHint: '差值基准：{window}',
            worst: '最差',
            worstHint: '{window} {metric} {value} 已越过危险阈值',
            reloadColumn: '重试该窗口',
            emptyTitle: '指标矩阵暂无数据',
            emptyDesc: '所有窗口都没有运行记录',
            columnError: '{window} 加载失败',
            drillHint: '点击单元格下钻到 Runs（Traces）',
          },
        },
      },
    },
  },
});

function metrics(windowMinutes: number, partial: Partial<RunnerMetricsSummary> = {}): RunnerMetricsSummary {
  return {
    window_minutes: windowMinutes,
    total_runs: 100,
    error_runs: 5,
    error_rate: 0.05,
    success_rate: 0.95,
    p95_duration_ms: 2000,
    ...partial,
  };
}

const slices: MatrixWindowSlice[] = [
  { windowMinutes: 15, metrics: metrics(15, { total_runs: 12, error_rate: 0.02, success_rate: 0.98 }) },
  { windowMinutes: 60, metrics: metrics(60, { total_runs: 80, error_rate: 0.12, success_rate: 0.88 }) },
  { windowMinutes: 360, metrics: metrics(360, { total_runs: 300 }) },
  { windowMinutes: 1440, metrics: metrics(1440, { total_runs: 900 }) },
];

const wrappers: VueWrapper[] = [];
function matrix(props: Record<string, unknown> = {}) {
  const wrapper = mount(MetricsMatrix, {
    props: { slices, activeWindow: 60, ...props },
    global: { plugins: [i18n], stubs },
  });
  wrappers.push(wrapper);
  return wrapper;
}

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount();
});

describe('MetricsMatrix', () => {
  it('渲染 4 个窗口列与固定指标行，当前窗口列标 aria-pressed', () => {
    const wrapper = matrix();

    const headers = wrapper.findAll('[data-test="matrix-window"]');
    expect(headers.map((h) => h.attributes('data-window'))).toEqual(['15', '60', '360', '1440']);
    expect(headers[1].attributes('aria-pressed')).toBe('true');
    expect(headers[0].attributes('aria-pressed')).toBe('false');

    const rows = wrapper.findAll('[data-test="matrix-row"]');
    expect(rows.map((r) => r.attributes('data-metric'))).toEqual([
      'total_runs',
      'error_runs',
      'error_rate',
      'success_rate',
      'p95_duration_ms',
    ]);
  });

  it('点窗口列头上抛 select-window（切到该窗口驱动下方明细）', async () => {
    const wrapper = matrix();

    await wrapper.get('[data-test="matrix-window"][data-window="360"]').trigger('click');
    expect(wrapper.emitted('select-window')).toEqual([[360]]);
  });

  it('恶化单元格带 tone=warn/danger 与 worst 标记，点击上抛 drill', async () => {
    const wrapper = matrix();

    const danger = wrapper.get('[data-test="matrix-cell"][data-window="60"][data-metric="error_rate"]');
    expect(danger.attributes('data-tone')).toBe('danger');
    expect(danger.attributes('data-worst')).toBe('true');
    expect(danger.text()).toContain('12.0%');

    await danger.trigger('click');
    expect(wrapper.emitted('drill')).toEqual([[{ windowMinutes: 60, metric: 'error_rate' }]]);
  });

  it('差值按最长窗口基准渲染（百分点/毫秒）', () => {
    const wrapper = matrix();
    const cell = wrapper.get('[data-test="matrix-cell"][data-window="60"][data-metric="error_rate"]');

    // 12% vs 基准 5% → +7.0pp
    expect(cell.text()).toContain('+7.0pp');
  });

  it('单个窗口加载失败时该列 role=alert 并可单独重试，其余列照常显示', async () => {
    const wrapper = matrix({
      slices: [
        { windowMinutes: 60, metrics: null, error: 'HTTP 500' },
        { windowMinutes: 1440, metrics: metrics(1440) },
      ],
    });

    const failed = wrapper.get('[data-test="matrix-column-error"][data-window="60"]');
    expect(failed.attributes('role')).toBe('alert');
    expect(wrapper.get('[data-test="matrix-cell"][data-window="1440"][data-metric="total_runs"]').text()).toContain('100');

    await wrapper.get('[data-test="matrix-retry"][data-window="60"]').trigger('click');
    expect(wrapper.emitted('retry')).toEqual([[60]]);
  });

  it('全部窗口无数据时渲染空态且不渲染表格', () => {
    const wrapper = matrix({ slices: [{ windowMinutes: 60, metrics: null }] });

    expect(wrapper.find('[data-test="matrix-table"]').exists()).toBe(false);
    expect(wrapper.text()).toContain('指标矩阵暂无数据');
  });

  it('加载中 aria-busy=true 且不误报空态', () => {
    const wrapper = matrix({ slices: [{ windowMinutes: 60, metrics: null, loading: true }] });

    expect(wrapper.get('[data-test="metrics-matrix"]').attributes('aria-busy')).toBe('true');
    expect(wrapper.text()).not.toContain('指标矩阵暂无数据');
  });
});
