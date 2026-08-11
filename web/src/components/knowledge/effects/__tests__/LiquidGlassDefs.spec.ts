import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import LiquidGlassDefs from '../LiquidGlassDefs.vue';

describe('LiquidGlassDefs', () => {
  it('渲染两个单例 filter：kb-liquid-refract（光纹）与 kb-liquid-bg（背景真折射）', () => {
    const w = mount(LiquidGlassDefs);
    expect(w.find('#kb-liquid-refract').exists()).toBe(true);
    expect(w.find('#kb-liquid-bg').exists()).toBe(true);
  });

  it('kb-liquid-bg 含 feDisplacementMap（真折射核心）', () => {
    const w = mount(LiquidGlassDefs);
    const bg = w.find('#kb-liquid-bg');
    expect(bg.find('feDisplacementMap').exists()).toBe(true);
  });

  it('SVG 本体不可见且不占布局（width/height 0 + absolute）', () => {
    const w = mount(LiquidGlassDefs);
    const svg = w.find('svg');
    expect(svg.attributes('width')).toBe('0');
    expect(svg.attributes('height')).toBe('0');
    expect(svg.attributes('aria-hidden')).toBe('true');
  });
});
