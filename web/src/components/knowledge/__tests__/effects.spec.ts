// SP2 特效组件 smoke：挂载不炸 + 降级契约（reduced-motion）。
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { mount } from '@vue/test-utils';
import GlassPanel from '../effects/GlassPanel.vue';
import TiltCard from '../effects/TiltCard.vue';
import GlowButton from '../effects/GlowButton.vue';
import RingCarousel from '../effects/RingCarousel.vue';
import ParticleField from '../effects/ParticleField.vue';

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

const FOUR_ITEMS = [
  { key: 'a', title: '笔记 A' },
  { key: 'b', title: '笔记 B' },
  { key: 'c', title: '笔记 C' },
  { key: 'd', title: '笔记 D' },
];

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

  // V3：hover 倾斜 → 离开后弹簧回正（overshoot easing 类）
  it('TiltCard tilts on hover and springs back on leave', async () => {
    stubReduced(false);
    const w = mount(TiltCard, { slots: { default: '<p>x</p>' }, ...globalOpts });
    const el = w.find('.kb-tilt-card');
    // jsdom 无布局：stub 几何使鼠标归一化可用（中心 50,50 / 尺寸 100×100）
    (el.element as HTMLElement).getBoundingClientRect = () =>
      ({
        left: 0,
        top: 0,
        width: 100,
        height: 100,
        right: 100,
        bottom: 100,
        x: 0,
        y: 0,
        toJSON: () => ({}),
      }) as DOMRect;
    await el.trigger('mousemove', { clientX: 75, clientY: 25 }); // ry=(0.75-0.5)*2*8=4°
    expect(el.attributes('style')).toContain('rotateY(4.00deg)');
    expect(el.classes()).not.toContain('kb-tilt-card--spring');
    await el.trigger('mouseleave');
    expect(el.classes()).toContain('kb-tilt-card--spring');
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

  // V1：JS 驱动旋转——挂载后首帧同步写入 rotateY 与聚焦态（不等 rAF，便于断言）
  it('RingCarousel writes initial rotation and focus state synchronously', () => {
    stubReduced(false);
    const w = mount(RingCarousel, { props: { items: FOUR_ITEMS, autoplay: false }, ...globalOpts });
    expect(w.find('.kb-ring__ring').attributes('style')).toContain('rotateY(0.00deg)');
    const cards = w.findAll('.kb-ring__card');
    expect(cards).toHaveLength(4);
    // 0° 时首张卡正对正面：聚焦类 + focus 变量拉满 + scale 拉满
    expect(cards[0].classes()).toContain('kb-ring__card--focus');
    expect(cards[0].attributes('style')).toContain('--focus: 1');
    expect(cards[0].attributes('style')).toContain('scale(1.000)');
    // 对侧卡（180°）：完全失焦
    expect(cards[2].classes()).not.toContain('kb-ring__card--focus');
    expect(cards[2].attributes('style')).toContain('--focus: 0');
  });

  it('RingCarousel drag rotates the ring', async () => {
    stubReduced(false);
    const w = mount(RingCarousel, { props: { items: FOUR_ITEMS, autoplay: false }, ...globalOpts });
    const stage = w.find('.kb-ring__stage');
    await stage.trigger('pointerdown', { clientX: 100 });
    await stage.trigger('pointermove', { clientX: 160 }); // dx=60 × 0.35°/px = 21°
    expect(w.find('.kb-ring__ring').attributes('style')).toContain('rotateY(21.00deg)');
    await stage.trigger('pointerup');
  });

  it('RingCarousel suppresses click after a real drag', async () => {
    stubReduced(false);
    const w = mount(RingCarousel, { props: { items: FOUR_ITEMS, autoplay: false }, ...globalOpts });
    const stage = w.find('.kb-ring__stage');
    await stage.trigger('pointerdown', { clientX: 100 });
    await stage.trigger('pointermove', { clientX: 160 });
    await stage.trigger('pointerup');
    await w.find('.kb-ring__card').trigger('click');
    expect(w.emitted('select')).toBeUndefined();
  });

  it('RingCarousel wheel rotates the ring', async () => {
    stubReduced(false);
    const w = mount(RingCarousel, { props: { items: FOUR_ITEMS, autoplay: false }, ...globalOpts });
    await w.find('.kb-ring__stage').trigger('wheel', { deltaY: 100 }); // 100 × 0.12°/px = 12°
    expect(w.find('.kb-ring__ring').attributes('style')).toContain('rotateY(12.00deg)');
  });

  it('RingCarousel accepts custom radius/speed/card size props', () => {
    stubReduced(false);
    const w = mount(RingCarousel, {
      props: { items: FOUR_ITEMS, autoplay: false, radius: 300, speed: 30, cardWidth: 180, cardHeight: 200 },
      ...globalOpts,
    });
    expect(w.find('.kb-ring__card').attributes('style')).toContain('translateZ(300px)');
    const ring = w.find('.kb-ring__ring');
    expect(ring.attributes('style')).toContain('width: 180px');
    expect(ring.attributes('style')).toContain('height: 200px');
  });

  // V2：ParticleField 渲染契约——允许动效时挂 canvas，reduced-motion 时不渲染
  it('ParticleField renders canvas when motion allowed', () => {
    stubReduced(false);
    const w = mount(ParticleField, globalOpts);
    expect(w.find('canvas.kb-particle-field').exists()).toBe(true);
  });

  it('ParticleField renders nothing under reduced-motion', () => {
    stubReduced(true);
    const w = mount(ParticleField, globalOpts);
    expect(w.find('canvas.kb-particle-field').exists()).toBe(false);
  });
});
