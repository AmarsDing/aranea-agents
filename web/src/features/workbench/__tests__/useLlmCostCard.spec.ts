// E阶段 W2b D6 V5 后半（F2 成本卡）：useLlmCostCard 组合式契约测试。
// 验收 #8 口径锚：数字映射自 GetUsageOverview 同 RPC（today/month/top_agents/quota_dashboard）。
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h, nextTick } from 'vue';
import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { BUDGET_WARN_RATIO, TOP_AGENTS_LIMIT, deriveBudgetState, useLlmCostCard } from '../useLlmCostCard';
import { getModelUsageOverview } from '../../usage/api';
import type { ModelUsageOverview } from '../../usage/types';

vi.mock('../../usage/api', () => ({ getModelUsageOverview: vi.fn() }));

function summary(cost: number) {
  return {
    call_count: 0,
    request_count: 0,
    success_count: 0,
    failed_count: 0,
    cancelled_count: 0,
    input_tokens: 0,
    output_tokens: 0,
    total_tokens: 0,
    total_cost_micro_usd: cost,
    avg_latency_ms: 0,
    avg_tokens_per_second: 0,
    success_rate: 0,
  };
}

function overview(overrides: Partial<ModelUsageOverview> = {}): ModelUsageOverview {
  return {
    today: summary(1_230_000),
    yesterday: summary(0),
    month: summary(45_000_000),
    range: summary(0),
    trends: [],
    top_models: [],
    top_agents: [],
    anomalies: [],
    ...overrides,
  };
}

function setup(opts: { pollMs?: number; autoStart?: boolean } = {}) {
  let view!: ReturnType<typeof useLlmCostCard>;
  mount(
    defineComponent({
      setup() {
        view = useLlmCostCard(opts);
        return () => h('div');
      },
    }),
  );
  return view;
}

beforeEach(() => {
  setActivePinia(createPinia());
  vi.resetAllMocks();
});

describe('deriveBudgetState', () => {
  it('无配额配置 → none（不显示，不假装安全）', () => {
    expect(deriveBudgetState(null)).toBe('none');
    expect(deriveBudgetState(undefined)).toBe('none');
    expect(
      deriveBudgetState({
        configured_count: 0,
        total_cap_micro_usd: 0,
        total_spent_micro_usd: 0,
        max_utilization_ratio: 2,
      }),
    ).toBe('none');
  });

  it('利用率分档：<0.8 ok / ≥0.8 warn / ≥1 over（边界钉死）', () => {
    const q = { configured_count: 2, total_cap_micro_usd: 100, total_spent_micro_usd: 0, max_utilization_ratio: 0 };
    expect(deriveBudgetState({ ...q, max_utilization_ratio: 0 })).toBe('ok');
    expect(deriveBudgetState({ ...q, max_utilization_ratio: BUDGET_WARN_RATIO - 0.01 })).toBe('ok');
    expect(deriveBudgetState({ ...q, max_utilization_ratio: BUDGET_WARN_RATIO })).toBe('warn');
    expect(deriveBudgetState({ ...q, max_utilization_ratio: 0.99 })).toBe('warn');
    expect(deriveBudgetState({ ...q, max_utilization_ratio: 1 })).toBe('over');
    expect(deriveBudgetState({ ...q, max_utilization_ratio: 1.5 })).toBe('over');
  });
});

describe('useLlmCostCard', () => {
  it('autoStart=false 时不发请求（测试锚：手动驱动）', async () => {
    setup({ autoStart: false });
    await flushPromises();
    expect(getModelUsageOverview).not.toHaveBeenCalled();
  });

  it('refresh 成功映射 today/month/top_agents/quota 并置 loaded', async () => {
    vi.mocked(getModelUsageOverview).mockResolvedValue(
      overview({
        top_agents: [
          {
            provider_code: '',
            model_api_id: '',
            model_display_name: '',
            agent_id: '1',
            agent_key: 'alpha',
            call_count: 1,
            input_tokens: 0,
            output_tokens: 0,
            total_tokens: 0,
            total_cost_micro_usd: 900_000,
            avg_latency_ms: 0,
            avg_tokens_per_second: 0,
            success_rate: 1,
          },
          {
            provider_code: '',
            model_api_id: '',
            model_display_name: '',
            agent_id: '2',
            agent_key: 'beta',
            call_count: 1,
            input_tokens: 0,
            output_tokens: 0,
            total_tokens: 0,
            total_cost_micro_usd: 500_000,
            avg_latency_ms: 0,
            avg_tokens_per_second: 0,
            success_rate: 1,
          },
          {
            provider_code: '',
            model_api_id: '',
            model_display_name: '',
            agent_id: '3',
            agent_key: 'gamma',
            call_count: 1,
            input_tokens: 0,
            output_tokens: 0,
            total_tokens: 0,
            total_cost_micro_usd: 100_000,
            avg_latency_ms: 0,
            avg_tokens_per_second: 0,
            success_rate: 1,
          },
          {
            provider_code: '',
            model_api_id: '',
            model_display_name: '',
            agent_id: '4',
            agent_key: 'delta',
            call_count: 1,
            input_tokens: 0,
            output_tokens: 0,
            total_tokens: 0,
            total_cost_micro_usd: 1,
            avg_latency_ms: 0,
            avg_tokens_per_second: 0,
            success_rate: 1,
          },
        ],
        quota_dashboard: {
          configured_count: 1,
          total_cap_micro_usd: 100,
          total_spent_micro_usd: 50,
          max_utilization_ratio: 0.5,
        },
      }),
    );
    const view = setup({ autoStart: false });
    await view.refresh();

    expect(view.loaded.value).toBe(true);
    expect(view.error.value).toBe('');
    expect(view.todayCostMicroUsd.value).toBe(1_230_000);
    expect(view.monthCostMicroUsd.value).toBe(45_000_000);
    expect(view.topAgents.value.map((a) => a.agentKey)).toEqual(['alpha', 'beta', 'gamma']);
    expect(view.topAgents.value.length).toBe(TOP_AGENTS_LIMIT);
    expect(view.budgetState.value).toBe('ok');
    expect(view.lastUpdatedAt.value).not.toBe('');
  });

  it('top_agents 过滤空 key 且按 TOP_AGENTS_LIMIT 截断', async () => {
    vi.mocked(getModelUsageOverview).mockResolvedValue(
      overview({
        top_agents: [
          {
            provider_code: '',
            model_api_id: '',
            model_display_name: '',
            agent_id: '',
            agent_key: '',
            call_count: 0,
            input_tokens: 0,
            output_tokens: 0,
            total_tokens: 0,
            total_cost_micro_usd: 9,
            avg_latency_ms: 0,
            avg_tokens_per_second: 0,
            success_rate: 0,
          },
          {
            provider_code: '',
            model_api_id: '',
            model_display_name: '',
            agent_id: '1',
            agent_key: 'solo',
            call_count: 0,
            input_tokens: 0,
            output_tokens: 0,
            total_tokens: 0,
            total_cost_micro_usd: 7,
            avg_latency_ms: 0,
            avg_tokens_per_second: 0,
            success_rate: 0,
          },
        ],
      }),
    );
    const view = setup({ autoStart: false });
    await view.refresh();
    expect(view.topAgents.value).toEqual([{ agentKey: 'solo', costMicroUsd: 7 }]);
  });

  it('刷新失败保留上次成功数据 + 挂错误注记（诚实口径，不伪造清零）', async () => {
    vi.mocked(getModelUsageOverview).mockResolvedValueOnce(overview());
    const view = setup({ autoStart: false });
    await view.refresh();
    expect(view.todayCostMicroUsd.value).toBe(1_230_000);

    vi.mocked(getModelUsageOverview).mockRejectedValueOnce(new Error('boom'));
    await view.refresh();
    await nextTick();
    expect(view.error.value).toBe('boom');
    expect(view.loaded.value).toBe(true);
    expect(view.todayCostMicroUsd.value).toBe(1_230_000);
  });

  it('首刷失败：loaded 保持 false（卡面显 —），数字不伪造', async () => {
    vi.mocked(getModelUsageOverview).mockRejectedValue(new Error('net down'));
    const view = setup({ autoStart: false });
    await view.refresh();
    expect(view.loaded.value).toBe(false);
    expect(view.error.value).toBe('net down');
    expect(view.todayCostMicroUsd.value).toBe(0);
  });

  it('成功后错误清空；quota 缺席 → budgetState none', async () => {
    vi.mocked(getModelUsageOverview).mockRejectedValueOnce(new Error('x'));
    const view = setup({ autoStart: false });
    await view.refresh();
    expect(view.error.value).toBe('x');
    vi.mocked(getModelUsageOverview).mockResolvedValueOnce(overview({ quota_dashboard: undefined }));
    await view.refresh();
    expect(view.error.value).toBe('');
    expect(view.budgetState.value).toBe('none');
  });

  it('onUnmounted 停轮询（防泄漏定时器）', () => {
    const view = setup({ autoStart: false, pollMs: 60_000 });
    view.start();
    view.stop();
    expect(getModelUsageOverview).not.toHaveBeenCalled();
  });
});
