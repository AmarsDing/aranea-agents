// SP2 特效组件 smoke：挂载不炸 + 降级契约（reduced-motion）。
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { mount } from '@vue/test-utils';
import GlassPanel from '../effects/GlassPanel.vue';
import TiltCard from '../effects/TiltCard.vue';
import GlowButton from '../effects/GlowButton.vue';
import RingCarousel from '../effects/RingCarousel.vue';

function stubReduced(matches: boolean) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation(() => ({
      matches,
      media: '(prefers-reduced-motion: reduce)',
      addEventListener: () => {},
      removeEventListener: () => {},
    })),
  );
}

const globalOpts = {
  global: {
    stubs: { 'q-icon': { template: '<i />' } },
  },
};

describe('effects components', () => {
  beforeEach(() => vi.restoreAllMocks());
  afterEach(() => vi.unstubAllGlobals());

  it('GlassPanel renders title + slot', () => {
    stubReduced(false);
    const w = mount(GlassPanel, {
      props: { title: '反向链接', icon: 'link' },
      slots: { default: '<p class="inner">body</p>' },
      ...globalOpts,
    });
    expect(w.text()).toContain('反向链接');
    expect(w.find('.inner').exists()).toBe(true);
  });

  it('GlowButton emits click and renders label', async () => {
    stubReduced(false);
    const w = mount(GlowButton, { props: { label: '保存', icon: 'save' }, ...globalOpts });
    expect(w.text()).toContain('保存');
    await w.trigger('click');
    expect(w.emitted('click')).toHaveLength(1);
  });

  it('TiltCard stays static under reduced-motion', () => {
    stubReduced(true);
    const w = mount(TiltCard, { slots: { default: '<p>x</p>' }, ...globalOpts });
    const el = w.find('.kb-tilt-card');
    expect(el.attributes('style')).toContain('rotateX(0deg)');
  });

  it('RingCarousel degrades to flat list under reduced-motion', () => {
    stubReduced(true);
    const w = mount(RingCarousel, {
      props: {
        items: [
          { key: 'a', title: '笔记 A' },
          { key: 'b', title: '笔记 B' },
        ],
      },
      ...globalOpts,
    });
    expect(w.find('.kb-ring__flat-list').exists()).toBe(true);
    expect(w.text()).toContain('笔记 A');
    expect(w.text()).toContain('笔记 B');
  });

  it('RingCarousel renders 3D ring and emits select', async () => {
    stubReduced(false);
    const w = mount(RingCarousel, {
      props: { items: [{ key: 'a', title: '笔记 A' }] },
      ...globalOpts,
    });
    expect(w.find('.kb-ring__ring').exists()).toBe(true);
    await w.find('.kb-ring__card').trigger('click');
    expect(w.emitted('select')?.[0]?.[0]).toMatchObject({ key: 'a' });
  });
});
