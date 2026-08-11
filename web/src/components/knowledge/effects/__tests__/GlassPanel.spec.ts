import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import GlassPanel from '../GlassPanel.vue';

describe('GlassPanel（M1 refract）', () => {
  it('默认不带 refract 修饰类', () => {
    const w = mount(GlassPanel, { props: { title: 'T' } });
    expect(w.classes()).not.toContain('kb-glass-panel--refract');
  });

  it('refract prop → kb-glass-panel--refract 类', () => {
    const w = mount(GlassPanel, { props: { title: 'T', refract: true } });
    expect(w.classes()).toContain('kb-glass-panel--refract');
  });

  it('内联 filter-def 已迁移到 LiquidGlassDefs（防重复 id 回归）', () => {
    const w = mount(GlassPanel, { props: { title: 'T' } });
    expect(w.find('.kb-glass-panel__filter-def').exists()).toBe(false);
    expect(w.find('#kb-liquid-refract').exists()).toBe(false);
  });

  it('装饰层保留（sheen/highlight）', () => {
    const w = mount(GlassPanel, { props: { title: 'T', refract: true } });
    expect(w.find('.kb-glass-panel__sheen').exists()).toBe(true);
    expect(w.find('.kb-glass-panel__highlight').exists()).toBe(true);
  });
});
