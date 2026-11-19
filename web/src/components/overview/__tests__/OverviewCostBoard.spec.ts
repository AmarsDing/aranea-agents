import { mount } from '@vue/test-utils';
import { defineComponent } from 'vue';
import { createI18n } from 'vue-i18n';
import { describe, expect, it } from 'vitest';
import zh from '../../../i18n/locales/zh-CN';
import OverviewCostBoard from '../OverviewCostBoard.vue';
import { formatUsdCompact, formatUsdCompactFromUsd } from '../../../features/usage/moneyFormat';
import type { OvTerminal, OvTopBar } from '../ovTypes';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zh } });
// 切换按钮组：每个选项一个按钮，点了发 update:modelValue
const ToggleStub = defineComponent({
  props: { modelValue: { type: String, default: '' }, options: { type: Array, default: () => [] } },
  emits: ['update:modelValue'],
  template:
    '<div><button v-for="o in options" :key="o.value" :data-by="o.value" @click="$emit(\'update:modelValue\', o.value)">{{ o.label }}</button></div>',
});
const bar = (key: string, costUsd: number): OvTopBar => ({ key, label: key, costUsd, ratio: 1, unpriced: false });
const base = {
  costValueUsd: '$3.18',
  costDelta: null,
  budget: { configured: false, pct: 0, spentUsd: '', capUsd: '', forecastUsd: null, lowConfidence: false, tone: 'ok' },
  unpricedCount: 0,
  cache: { denied: true, samples: 0, p50: 0, low: false },
} as const;
const mountBoard = (props: Record<string, unknown>) =>
  mount(OverviewCostBoard, {
    props: { ...base, topBars: [bar('model-a', 3.18)], ...props },
    global: { plugins: [i18n], stubs: { 'q-icon': true, 'q-btn-toggle': ToggleStub, OverviewStatTile: true } },
  });

describe('金额格式：美元和微美元不混用', () => {
  it('概览页内部的美元值按美元显示，不再被当成微美元', () => {
    expect(formatUsdCompactFromUsd(3.18)).toBe('$3.18');
    expect(formatUsdCompactFromUsd(3.18)).toBe(formatUsdCompact(3_180_000));
    expect(formatUsdCompactFromUsd(null)).toBe('$0.0000');
  });
});

describe('OverviewCostBoard', () => {
  it('成本条默认按模型，可以切到按 Agent；金额按美元显示', async () => {
    const wrapper = mountBoard({ agentBars: [bar('选题编辑', 1.42), bar('评审岗', 1.18)] });
    expect(wrapper.findAll('.ov-topbars__row').map((r) => r.text())).toEqual(['01model-a$3.18']);
    await wrapper.get('[data-by="agent"]').trigger('click');
    expect(wrapper.findAll('.ov-topbars__row').map((r) => r.text())).toEqual(['01选题编辑$1.42', '02评审岗$1.18']);
  });

  it('切到没有数据的维度时写明没有花费', async () => {
    const wrapper = mountBoard({});
    await wrapper.get('[data-by="agent"]').trigger('click');
    expect(wrapper.text()).toContain('这个区间没有花费');
  });

  it('编码终端：有额度画进度条，额度未知写明并显示 Token，会话数照实显示', () => {
    const terminals: OvTerminal[] = [
      {
        agent: 'claude',
        sessions: 23,
        source: 'logs',
        windows: [
          { label: '5h', tokens: 1_200_000, percent: 92, resetsAt: '' },
          { label: '7d', tokens: 18_400_000, percent: 41, resetsAt: '' },
        ],
      },
      {
        agent: 'codebuddy',
        sessions: 6,
        source: 'logs',
        windows: [{ label: '7d', tokens: 15_324, percent: -1, resetsAt: '' }],
      },
    ];
    const wrapper = mountBoard({ terminals });
    const rows = wrapper.findAll('.ov-terms__row');
    expect(rows).toHaveLength(2);
    expect(rows[0].text()).toContain('92%');
    expect(rows[0].text()).toContain('23 个会话');
    expect(rows[0].findAll('[role="progressbar"]')).toHaveLength(2);
    expect(rows[0].find('.ov-terms__fill--danger').exists()).toBe(true);
    expect(rows[1].find('[role="progressbar"]').exists()).toBe(false);
    expect(rows[1].text()).toContain('额度未知');
    expect(rows[1].text()).toContain('1.53w');
  });

  it('终端用量取不到时如实写明；还没取回时不占位', () => {
    expect(mountBoard({ terminals: null }).text()).toContain('拿不到终端用量');
    expect(mountBoard({ terminals: [] }).text()).toContain('还没有终端的用量记录');
    expect(mountBoard({}).text()).not.toContain('编码终端');
  });
});
