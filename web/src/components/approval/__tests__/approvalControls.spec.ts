import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import DecisionDock from '../DecisionDock.vue';
import EvidenceRail from '../EvidenceRail.vue';
import { i18n } from '../../../i18n';
import type { ApprovalItemRow } from '../../../features/approval/types';

function item(overrides: Partial<ApprovalItemRow> = {}): ApprovalItemRow {
  return {
    id: 'ap-1',
    kind: 'task_review',
    title: '验收任务 T-42',
    summary: '任务已完成，等待验收',
    requester: 'alice',
    risk_level: 'high',
    context: { task_id: 't-42', verdict: 'approve', risk_score: 82, note: 'diff 已复核' },
    context_raw: '',
    agent_key: 'writer',
    created_at: new Date(Date.now() - 30 * 60000).toISOString(),
    ...overrides,
  };
}

function mountDock(props: Record<string, unknown>) {
  return mount(DecisionDock, {
    props: {
      item: null,
      note: '',
      deciding: false,
      selectedCount: 0,
      batchRunning: false,
      batchDone: 0,
      batchTotal: 0,
      outcomes: [],
      ...props,
    },
    global: { plugins: [i18n] },
  });
}

describe('DecisionDock interaction contract', () => {
  it('shows the empty hint when nothing is selected', () => {
    const wrapper = mountDock({});
    expect(wrapper.find('.decision-dock__hint').text()).toContain('从左侧队列选择一条待办');
    expect(wrapper.find('.decision-dock__btn--primary').attributes('disabled')).toBeDefined();
  });

  it('single mode exposes approve / reject / archive / next and emits the action', async () => {
    const wrapper = mountDock({ item: item() });
    expect(wrapper.find('.decision-dock__title').text()).toBe('验收任务 T-42');

    const buttons = wrapper.findAll('.decision-dock__btn');
    expect(buttons.map((btn) => btn.text())).toEqual(['通过', '驳回', '归档', '下一条']);

    await wrapper.find('.decision-dock__btn--primary').trigger('click');
    expect(wrapper.emitted('decide')?.[0]).toEqual(['approve']);

    await wrapper.find('.decision-dock__btn--ghost').trigger('click');
    expect(wrapper.emitted('decide')?.[1]).toEqual(['archive']);

    await wrapper.findAll('.decision-dock__btn--ghost')[1].trigger('click');
    expect(wrapper.emitted('next')).toHaveLength(1);
  });

  it('emits note updates and appends reason templates', async () => {
    const wrapper = mountDock({ item: item() });
    const note = wrapper.find('#decision-dock-note');
    await note.setValue('材料缺失');
    expect(wrapper.emitted('update:note')?.[0]).toEqual(['材料缺失']);

    await wrapper.findAll('.decision-dock__chip')[0].trigger('click');
    expect(wrapper.emitted('update:note')?.at(-1)).toEqual(['证据不足，需补充材料']);
  });

  it('batch mode switches the actions to batch approve / reject and shows progress', () => {
    const wrapper = mountDock({ selectedCount: 3, batchRunning: true, batchDone: 1, batchTotal: 3 });
    expect(wrapper.find('.decision-dock__mode').text()).toBe('批量裁决');
    expect(wrapper.text()).toContain('已选 3 条');
    expect(wrapper.find('.decision-dock__btn--primary').text()).toBe('通过');
    expect(wrapper.find('.decision-dock__btn--danger').text()).toBe('驳回');
    expect(wrapper.find('.decision-dock__progress').text()).toContain('1/3');
    expect(wrapper.attributes('aria-busy')).toBe('true');
  });

  it('lists failed items and only offers retry for them', async () => {
    const wrapper = mountDock({
      outcomes: [
        { key: 'task_review:ap-1', title: '验收任务 T-42', status: 'failed', message: 'boom' },
        { key: 'task_review:ap-2', title: '验收任务 T-43', status: 'conflict' },
      ],
    });
    expect(wrapper.find('.decision-dock__failed-list').text()).toContain('boom');
    expect(wrapper.find('.decision-dock__conflicts').text()).toContain('验收任务 T-43');

    const retry = wrapper.findAll('.decision-dock__btn--ghost').at(-1);
    await retry?.trigger('click');
    expect(wrapper.emitted('retry-failed')).toHaveLength(1);
  });
});

describe('EvidenceRail evidence contract', () => {
  it('separates impact, machine verdict and request context', () => {
    const wrapper = mount(EvidenceRail, { props: { item: item(), backlog: 4 }, global: { plugins: [i18n] } });

    expect(wrapper.find('.evidence-rail__kind').text()).toBe('任务验收');
    expect(wrapper.find('.risk-meter__text').text()).toBe('高');
    expect(wrapper.text()).toContain('同类型积压 4 条');
    // 影响面：任务 t-42
    expect(wrapper.findAll('.evidence-rail__section')[0].text()).toContain('t-42');
    // 机器判定：verdict / risk_score
    const verdict = wrapper.findAll('.evidence-rail__section')[1].text();
    expect(verdict).toContain('verdict');
    expect(verdict).toContain('risk_score');
    // 请求上下文：note
    const context = wrapper.findAll('.evidence-rail__section')[2].text();
    expect(context).toContain('note');
    expect(context).not.toContain('verdict');
    expect(wrapper.find('.evidence-rail__foot').text()).toContain('alice');
  });

  it('falls back to raw context when JSON parsing failed', () => {
    const wrapper = mount(EvidenceRail, {
      props: { item: item({ context: null, context_raw: '{"broken":' }) },
      global: { plugins: [i18n] },
    });
    expect(wrapper.find('.evidence-rail__raw').text()).toBe('{"broken":');
  });

  it('shows an empty state when no item is selected', () => {
    const wrapper = mount(EvidenceRail, { props: { item: null }, global: { plugins: [i18n] } });
    expect(wrapper.find('.evidence-rail__empty').exists()).toBe(true);
  });
});
