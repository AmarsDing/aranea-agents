// web/src/features/chat/composables/__tests__/useGraphViewport.spec.ts
import { describe, it, expect } from 'vitest';
import { useGraphViewport, GRAPH_VIEWPORT_MIN_SCALE, GRAPH_VIEWPORT_MAX_SCALE } from '../useGraphViewport';

function mkWheel(deltaY: number, clientX = 100, clientY = 50): WheelEvent {
  return {
    deltaY,
    clientX,
    clientY,
    currentTarget: {
      getBoundingClientRect: () => ({ left: 0, top: 0, width: 800, height: 300 }),
    },
    preventDefault: () => {},
  } as unknown as WheelEvent;
}

function mkPointer(clientX: number, clientY: number): PointerEvent {
  return { clientX, clientY, button: 0 } as unknown as PointerEvent;
}

describe('useGraphViewport', () => {
  it('starts at scale=1 with zero translate', () => {
    const v = useGraphViewport();
    expect(v.scale.value).toBe(1);
    expect(v.tx.value).toBe(0);
    expect(v.ty.value).toBe(0);
    expect(v.transformStyle.value.transform).toBe('translate(0px, 0px) scale(1)');
  });

  it('zoomIn/zoomOut scale by step and clamp to [MIN, MAX]', () => {
    const v = useGraphViewport();
    for (let i = 0; i < 30; i++) v.zoomIn();
    expect(v.scale.value).toBe(GRAPH_VIEWPORT_MAX_SCALE);
    for (let i = 0; i < 60; i++) v.zoomOut();
    expect(v.scale.value).toBe(GRAPH_VIEWPORT_MIN_SCALE);
  });

  it('reset restores scale=1 and zero translate', () => {
    const v = useGraphViewport();
    v.zoomIn();
    v.onPanStart(mkPointer(10, 10));
    v.onPanMove(mkPointer(60, 40));
    v.onPanEnd();
    v.reset();
    expect(v.scale.value).toBe(1);
    expect(v.tx.value).toBe(0);
    expect(v.ty.value).toBe(0);
  });

  it('wheel zooms in on scroll up and out on scroll down', () => {
    const v = useGraphViewport();
    v.onWheel(mkWheel(-100));
    expect(v.scale.value).toBeGreaterThan(1);
    const afterIn = v.scale.value;
    v.onWheel(mkWheel(100));
    expect(v.scale.value).toBeLessThan(afterIn);
  });

  it('wheel keeps the cursor point stationary (zoom around cursor)', () => {
    const v = useGraphViewport();
    // 初始 scale=1, tx=ty=0：光标 (100,50) 对应内容坐标 (100,50)
    v.onWheel(mkWheel(-100, 100, 50));
    const s = v.scale.value;
    // 缩放后：content(100,50) 映射回视口 (100,50) → tx + 100*s = 100, ty + 50*s = 50
    expect(v.tx.value + 100 * s).toBeCloseTo(100, 5);
    expect(v.ty.value + 50 * s).toBeCloseTo(50, 5);
  });

  it('zoomFit shrinks content to fit viewport and centers it', () => {
    const v = useGraphViewport();
    // 内容 1600x600，视口 800x300 → fit scale = 0.5，居中
    v.zoomFit(1600, 600, 800, 300);
    expect(v.scale.value).toBeCloseTo(0.5, 5);
    expect(v.tx.value).toBeCloseTo(0, 5);
    expect(v.ty.value).toBeCloseTo(0, 5);
  });

  it('zoomFit never exceeds scale 1 for small content', () => {
    const v = useGraphViewport();
    v.zoomFit(400, 200, 800, 300);
    expect(v.scale.value).toBe(1);
  });

  it('pan moves translate and marks justPanned after threshold', () => {
    const v = useGraphViewport();
    v.onPanStart(mkPointer(10, 10));
    // 阈值内移动：不视为 pan
    v.onPanMove(mkPointer(12, 11));
    expect(v.justPanned.value).toBe(false);
    expect(v.tx.value).toBe(0);
    // 超过阈值
    v.onPanMove(mkPointer(60, 40));
    expect(v.justPanned.value).toBe(true);
    expect(v.tx.value).toBe(50);
    expect(v.ty.value).toBe(30);
    v.onPanEnd();
    expect(v.isPanning.value).toBe(false);
  });

  it('ignores non-left-button pan start', () => {
    const v = useGraphViewport();
    v.onPanStart({ clientX: 0, clientY: 0, button: 2 } as unknown as PointerEvent);
    v.onPanMove(mkPointer(50, 50));
    expect(v.tx.value).toBe(0);
  });
});
