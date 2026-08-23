import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import GlassPanel from '../GlassPanel.vue';

describe('GlassPanel（SP3 标准玻璃）', () => {
  it('渲染标题 + 图标 + 插槽', () => {
    const w = mount(GlassPanel, {
      props: { title: '反向链接', icon: 'link' },
      slots: { default: '<p class="inner">body</p>' },
      global: { stubs: { 'q-icon': { template: '<i />' } } },
    });
    expect(w.text()).toContain('反向链接');
    expect(w.find('.inner').exists()).toBe(true);
  });

  it('strong 修饰类保留', () => {
    const w = mount(GlassPanel, { props: { strong: true } });
    expect(w.classes()).toContain('kb-glass--strong');
  });

  it('装饰层与特效修饰类已退役（无 sheen/highlight/glow/refract）', () => {
    const w = mount(GlassPanel, { props: { title: 'T' } });
    expect(w.find('.kb-glass-panel__sheen').exists()).toBe(false);
    expect(w.find('.kb-glass-panel__highlight').exists()).toBe(false);
    expect(w.classes()).not.toContain('kb-glass-panel--refract');
    expect(w.classes()).not.toContain('kb-glass-panel--glow');
  });
});
