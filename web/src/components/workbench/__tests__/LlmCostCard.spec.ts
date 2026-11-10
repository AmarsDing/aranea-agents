// E阶段 W2b D6 V5 后半（F2 成本卡）：LlmCostCard 展示契约测试。
// 锚定：诚实口径（未加载显 —）、金额格式化与用量页同函数、预算三态、错误行+重试。
import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import LlmCostCard from '../LlmCostCard.vue';

const base = {
  loaded: true,
  todayCostMicroUsd: 1_230_000, // $1.23
  monthCostMicroUsd: 45_000_000, // $45.00
  topAgents: [
    { agentKey: 'alpha', costMicroUsd: 900_000 },
    { agentKey: 'beta', costMicroUsd: 500_000 },
  ],
  budgetState: 'ok' as const,
  quotaRatio: 0.5,
};

describe('LlmCostCard', () => {
  it('未加载显 —（诚实口径，不伪造 0）', () => {
    const w = mount(LlmCostCard, { props: { ...base, loaded: false } });
    expect(w.get('[data-testid="cost-today"]').text()).toBe('—');
    expect(w.get('[data-testid="cost-month"]').text()).toBe('—');
    expect(w.text()).not.toContain('$');
  });

  it('已加载按 formatUsdCompact 口径渲染今日/本月', () => {
    const w = mount(LlmCostCard, { props: base });
    expect(w.get('[data-testid="cost-today"]').text()).toBe('$1.23');
    expect(w.get('[data-testid="cost-month"]').text()).toBe('$45.00');
    expect(w.text()).toContain('@alpha');
    expect(w.text()).toContain('$0.900');
  });

  it('budgetState=none 不渲染预算徽标（无配置不假装安全）', () => {
    const w = mount(LlmCostCard, { props: { ...base, budgetState: 'none' as const } });
    expect(w.find('.wb-cost__budget').exists()).toBe(false);
  });

  it('预算三态 class 契约（ok/warn/over）', () => {
    for (const s of ['ok', 'warn', 'over'] as const) {
      const w = mount(LlmCostCard, { props: { ...base, budgetState: s } });
      expect(w.find(`.wb-cost__budget--${s}`).exists()).toBe(true);
    }
  });

  it('错误行 role=alert 且重试按钮 emit refresh', async () => {
    const w = mount(LlmCostCard, { props: { ...base, error: 'net down' } });
    const alert = w.get('[role="alert"]');
    expect(alert.text()).toContain('net down');
    await alert.get('button').trigger('click');
    expect(w.emitted('refresh')).toHaveLength(1);
  });

  it('labels prop 覆盖默认文案（i18n 注入路径）', () => {
    const w = mount(LlmCostCard, { props: { ...base, labels: { title: 'LLM COST' } } });
    expect(w.text()).toContain('LLM COST');
  });
});
