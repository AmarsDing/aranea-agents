// 五态契约：数据集列表在失败时不得再渲染「暂无数据集」（2026-11-10 标准符合性核对第 3 条）。
import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import EvaluationDatasetList from '../EvaluationDatasetList.vue';
import type { EvalDataset } from '../../../features/evaluation/types';

const quasarStubs = {
  'q-card': { template: '<div><slot /></div>' },
  'q-card-section': { template: '<div><slot /></div>' },
  'q-separator': { template: '<hr />' },
  'q-list': { template: '<div><slot /></div>' },
  'q-item': { template: '<div><slot /></div>' },
  'q-item-section': { template: '<div><slot /></div>' },
  'q-item-label': { template: '<div><slot /></div>' },
};

const dataset = { id: 'ds-1', name: '基线集', case_count: 12 } as EvalDataset;

function mountList(props: Record<string, unknown>) {
  return mount(EvaluationDatasetList, {
    props: { datasets: [], selectedId: '', ...props },
    global: { stubs: quasarStubs },
  });
}

describe('EvaluationDatasetList 五态契约', () => {
  it('空数据且无错误时显示空态', () => {
    const w = mountList({});
    expect(w.text()).toContain('暂无数据集');
  });

  it('加载中不显示空态', () => {
    const w = mountList({ loading: true });
    expect(w.text()).not.toContain('暂无数据集');
  });

  it('失败时显示错误且抑制空态（不得同屏「暂无」）', () => {
    const w = mountList({ error: 'datasets 500' });
    const text = w.text();
    expect(text).toContain('datasets 500');
    expect(text).not.toContain('暂无数据集');
    expect(w.find('[role="alert"]').exists()).toBe(true);
  });

  it('有数据时正常渲染条目且无空态', () => {
    const w = mountList({ datasets: [dataset] });
    expect(w.text()).toContain('基线集');
    expect(w.text()).not.toContain('暂无数据集');
  });
});
