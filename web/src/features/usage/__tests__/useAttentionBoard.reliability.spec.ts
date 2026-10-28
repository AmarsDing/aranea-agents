import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h, nextTick, reactive } from 'vue';
import { createPinia, setActivePinia } from 'pinia';
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import zh from '../../../i18n/locales/zh-CN';
import en from '../../../i18n/locales/en-US';
import { useAttentionBoard } from '../useAttentionBoard';
import { useAttentionStore } from '../../../stores/usage/attention';
import { listMonitorAlertRules, listAlertMetrics, getRunnerMetrics } from '../../monitor/api';
import { listTaskDeadLetters } from '../../teams/api';
import { listDecisionRecords } from '../../decision/api';
import { listCronTaskRuns } from '../../cron/api';
import type { ModelTokenUsageEvent } from '../types';

vi.mock('../../monitor/api', () => ({
  listMonitorAlertRules: vi.fn(),
  listAlertMetrics: vi.fn(),
  getRunnerMetrics: vi.fn(),
}));
vi.mock('../../teams/api', () => ({ listTaskDeadLetters: vi.fn() }));
vi.mock('../../decision/api', () => ({ listDecisionRecords: vi.fn() }));
vi.mock('../../cron/api', () => ({ listCronTaskRuns: vi.fn() }));
vi.mock('../../../stores/usage', () => ({ useUsageStore: () => usage }));

const usage = reactive<{ overview: { anomalies: ModelTokenUsageEvent[] } | null; error: string }>({
  overview: null,
  error: '',
});
let wrapper: VueWrapper | undefined;

function setup() {
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zh, 'en-US': en } });
  let view!: ReturnType<typeof useAttentionBoard>;
  wrapper = mount(
    defineComponent({
      setup() {
        view = useAttentionBoard();
        return () => h('div');
      },
    }),
    { global: { plugins: [i18n] } },
  );
  return { view, i18n, store: useAttentionStore() };
}

beforeEach(() => {
  setActivePinia(createPinia());
  vi.resetAllMocks();
  usage.overview = { anomalies: [] };
  usage.error = '';
  vi.mocked(listMonitorAlertRules).mockResolvedValue([]);
  vi.mocked(listAlertMetrics).mockResolvedValue([]);
  vi.mocked(getRunnerMetrics).mockResolvedValue({
    window_minutes: 1440,
    total_runs: 0,
    error_runs: 0,
    error_rate: 0,
    success_rate: 0,
  });
  vi.mocked(listTaskDeadLetters).mockResolvedValue([]);
  vi.mocked(listDecisionRecords).mockResolvedValue({ items: [], total: 0 });
  vi.mocked(listCronTaskRuns).mockResolvedValue({ items: [], total: 0 });
});
afterEach(() => {
  wrapper?.unmount();
});

describe('attention board availability', () => {
  it('starts unknown and reports healthy only after all required reads succeed', async () => {
    const { view } = setup();
    expect(view.board.value.worstSeverity).toBe('unknown');
    expect(view.board.value.lanes).toHaveLength(4);
    await flushPromises();
    expect(view.board.value.worstSeverity).toBe('ok');
    expect(view.board.value.lanes.every((lane) => lane.status === 'ready')).toBe(true);
  });

  it('keeps four lanes and never reports healthy when all APIs fail', async () => {
    for (const api of [
      listMonitorAlertRules,
      listAlertMetrics,
      getRunnerMetrics,
      listTaskDeadLetters,
      listDecisionRecords,
      listCronTaskRuns,
    ])
      vi.mocked(api).mockRejectedValue(new Error('offline'));
    usage.overview = null;
    usage.error = 'offline';
    const { view, store } = setup();
    await flushPromises();
    expect(view.board.value.worstSeverity).toBe('unknown');
    expect(view.board.value.lanes.map((lane) => lane.status)).toEqual([
      'unavailable',
      'unavailable',
      'unavailable',
      'unavailable',
    ]);
    expect(store.updatedAt).toBe('');
    expect(view.loading.value).toBe(false);
  });

  it('retains a known critical alert and freshness time when its next refresh fails', async () => {
    vi.mocked(listMonitorAlertRules).mockResolvedValue([
      {
        id: 'alert',
        name: 'Errors',
        metric_key: 'errors',
        threshold: 1,
        window_minutes: 5,
        enabled: true,
        severity: 'critical',
      },
    ]);
    vi.mocked(listAlertMetrics).mockResolvedValue([
      {
        key: 'errors',
        name: 'Errors',
        description: '',
        unit: 'count',
        default_window_minutes: 5,
        suggested_threshold: 1,
        current_value: 2,
        evaluated_at: '',
      },
    ]);
    const { view, store } = setup();
    await flushPromises();
    const timestamp = store.updatedAt;
    vi.mocked(listAlertMetrics).mockRejectedValueOnce(new Error('offline'));
    await view.refresh();
    expect(view.board.value.worstSeverity).toBe('critical');
    expect(view.board.value.lanes[0].status).toBe('partial');
    expect(view.board.value.lanes[0].items).toHaveLength(1);
    expect(store.updatedAt).toBe(timestamp);
  });

  it('recovers failed sources through retry without restarting the page', async () => {
    vi.mocked(listDecisionRecords).mockRejectedValueOnce(new Error('offline'));
    const { view } = setup();
    await flushPromises();
    expect(view.board.value.lanes[3].status).toBe('partial');
    await view.refresh();
    expect(view.board.value.incomplete).toBe(false);
    expect(view.board.value.worstSeverity).toBe('ok');
  });

  it('coalesces overlapping refreshes into one set of source reads', async () => {
    let finish!: (value: Awaited<ReturnType<typeof listAlertMetrics>>) => void;
    vi.mocked(listAlertMetrics).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const { view } = setup();
    const first = view.refresh();
    const second = view.refresh();
    expect(listAlertMetrics).toHaveBeenCalledTimes(1);
    finish([]);
    await Promise.all([first, second]);
    expect(view.loading.value).toBe(false);
  });

  it('reprojects localized messages immediately when the language changes', async () => {
    vi.mocked(listMonitorAlertRules).mockResolvedValue([
      {
        id: 'alert',
        name: 'Errors',
        metric_key: 'errors',
        threshold: 1,
        window_minutes: 5,
        enabled: true,
        severity: 'critical',
      },
    ]);
    vi.mocked(listAlertMetrics).mockResolvedValue([
      {
        key: 'errors',
        name: 'Errors',
        description: '',
        unit: 'count',
        default_window_minutes: 5,
        suggested_threshold: 1,
        current_value: 2,
        evaluated_at: '',
      },
    ]);
    const { view, i18n } = setup();
    await flushPromises();
    expect(view.board.value.lanes[0].items[0].detail).toContain('当前');
    i18n.global.locale.value = 'en-US';
    await nextTick();
    expect(view.board.value.lanes[0].items[0].detail).toContain('current');
    expect(listAlertMetrics).toHaveBeenCalledTimes(1);
  });

  it('does not render usage read failures as an empty successful call queue', async () => {
    const { view } = setup();
    await flushPromises();
    usage.error = 'offline';
    expect(view.board.value.lanes[2].status).toBe('partial');
    expect(view.board.value.worstSeverity).toBe('unknown');
  });

  it('cannot confirm alert health when an enabled rule has no matching metric', async () => {
    vi.mocked(listMonitorAlertRules).mockResolvedValue([
      {
        id: 'alert',
        name: 'Unknown metric',
        metric_key: 'missing',
        threshold: 1,
        window_minutes: 5,
        enabled: true,
        severity: 'critical',
      },
    ]);
    const { view } = setup();
    await flushPromises();
    expect(view.board.value.lanes[0].status).toBe('partial');
    expect(view.board.value.worstSeverity).toBe('unknown');
  });
});
