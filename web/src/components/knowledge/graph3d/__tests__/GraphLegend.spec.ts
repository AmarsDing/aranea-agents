// M5：GraphLegend 过滤图例——组行渲染 / 点击切换隐藏 / 悬停透镜事件。
import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import GraphLegend from '../GraphLegend.vue';

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  missing: (_l, k) => k,
  messages: { 'zh-CN': {} },
});

const globalOpts = {
  global: {
    plugins: [i18n],
    stubs: { 'q-icon': { template: '<i />' } },
  },
};

const groups = [
  { docType: 'note', color: '#4fc3f7', count: 12 },
  { docType: 'image', color: '#ba68c8', count: 3 },
];

describe('GraphLegend（M5）', () => {
  it('渲染组行：颜色点 + 组名 + 计数', () => {
    const w = mount(GraphLegend, { props: { groups, hiddenGroups: [] }, ...globalOpts });
    expect(w.text()).toContain('note');
    expect(w.text()).toContain('12');
    expect(w.text()).toContain('image');
  });

  it('点击组行 emit toggle-group', async () => {
    const w = mount(GraphLegend, { props: { groups, hiddenGroups: [] }, ...globalOpts });
    await w.find('[data-test="legend-row-note"]').trigger('click');
    expect(w.emitted('toggle-group')).toEqual([['note']]);
  });

  it('隐藏组行带隐藏样式类', () => {
    const w = mount(GraphLegend, { props: { groups, hiddenGroups: ['image'] }, ...globalOpts });
    expect(w.find('[data-test="legend-row-image"]').classes()).toContain('kg3d-legend__row--hidden');
  });

  it('悬停 emit lens-enter / lens-leave（透镜）', async () => {
    const w = mount(GraphLegend, { props: { groups, hiddenGroups: [] }, ...globalOpts });
    const row = w.find('[data-test="legend-row-note"]');
    await row.trigger('pointerenter');
    expect(w.emitted('lens-enter')).toEqual([['note']]);
    await row.trigger('pointerleave');
    expect(w.emitted('lens-leave')).toBeTruthy();
  });
});
