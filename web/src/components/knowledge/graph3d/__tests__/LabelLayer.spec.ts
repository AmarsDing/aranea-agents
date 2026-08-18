/**
 * LabelLayer 纯函数单测：候选池 top-K + 双阈值可见性（forced 豁免距离，G5-C v2 修复）
 * + 动态度数下限（G5-G 小图标签修复）。
 */
import { describe, expect, it } from 'vitest';
import {
  effectiveMinDegree,
  filterOverlappingLabels,
  scaleForPixels,
  selectLabelCandidates,
  shouldShowLabel,
  type NdcRect,
} from '../render/LabelLayer';

/** 造 NDC 矩形（中心+半宽半高）。 */
function rect(cx: number, cy: number, hw: number, hh: number): NdcRect {
  return { x0: cx - hw, y0: cy - hh, x1: cx + hw, y1: cy + hh };
}

describe('selectLabelCandidates', () => {
  it('按 degree 降序取 top-K', () => {
    const degree = new Uint16Array([1, 9, 3, 7, 5]);
    expect(selectLabelCandidates(degree, 3)).toEqual([1, 3, 4]);
  });

  it('K 超过节点数时返回全部', () => {
    const degree = new Uint16Array([2, 8]);
    expect(selectLabelCandidates(degree, 200)).toEqual([1, 0]);
  });

  it('V13 层级加权：低度 ultra 常显优先于高度 regular（结构层级标签 LOD）', () => {
    // 节点 0：degree 2 但 ultra（权重 4 → 8）；节点 1：degree 6 regular（权重 1 → 6）
    const degree = new Uint16Array([2, 6]);
    const tiers = new Uint8Array([2, 0]); // TIER_ULTRANODE / TIER_REGULAR
    expect(selectLabelCandidates(degree, 1, tiers)[0]).toBe(0);
  });

  it('V13 层级加权：不传 tiers 时保持 degree 语义（向后兼容）', () => {
    const degree = new Uint16Array([2, 6]);
    expect(selectLabelCandidates(degree, 1)[0]).toBe(1);
  });
});

describe('shouldShowLabel', () => {
  it('度数达标且距离内：开关开时显示', () => {
    expect(shouldShowLabel(100, 600, 8, 4, false, true)).toBe(true);
  });

  it('度数不足且非 forced：不显示', () => {
    expect(shouldShowLabel(100, 600, 2, 4, false, true)).toBe(false);
  });

  it('开关关时只显示 forced', () => {
    expect(shouldShowLabel(100, 600, 8, 4, false, false)).toBe(false);
    expect(shouldShowLabel(100, 600, 2, 4, true, false)).toBe(true);
  });

  it('距离超阈值：普通标签隐藏', () => {
    expect(shouldShowLabel(700, 600, 8, 4, false, true)).toBe(false);
  });

  it('forced（hover/选中）豁免距离阈值：拉远超阈值后 hover 必须出标签', () => {
    // maxDistance = fitDist + radius（G5-G 修复）；用户拉远或 hover 远侧节点时距离必超阈值
    expect(shouldShowLabel(1000, 600, 0, 4, true, true)).toBe(true);
    expect(shouldShowLabel(1000, 600, 0, 4, true, false)).toBe(true);
  });
});

describe('scaleForPixels（UX：像素目标真恒尺寸，近景不爆炸）', () => {
  const TAN_HALF = Math.tan((60 * Math.PI) / 360); // fov 60
  const VH = 849;
  const BASE = 3.2;
  const TARGET = 12.5;

  /** 由 k 反推屏幕像素字高：base·k·(vh/2)/(dist·tanHalf)。 */
  function screenPx(dist: number): number {
    const k = scaleForPixels(dist, TAN_HALF, VH, TARGET, BASE);
    return (BASE * k * (VH / 2)) / (dist * TAN_HALF);
  }

  it('屏幕字高跨距离严格恒定（近/中/远全量程 = targetPx）', () => {
    for (const dist of [5, 30, 100, 300, 800, 2000]) {
      expect(screenPx(dist)).toBeCloseTo(TARGET, 1);
    }
  });

  it('近景不再 1/dist 爆炸：dist=2 时仍是 targetPx（旧 min 钳制方案实测 60~90px）', () => {
    expect(screenPx(2)).toBeCloseTo(TARGET, 1);
    expect(screenPx(2)).toBeLessThan(20);
  });

  it('k 随 dist 单调递增（世界尺寸近小远大），且钳制在 [0.002, 16]', () => {
    const kNear = scaleForPixels(0.001, TAN_HALF, VH, TARGET, BASE);
    const kFar = scaleForPixels(1e6, TAN_HALF, VH, TARGET, BASE);
    expect(kNear).toBeGreaterThanOrEqual(0.002);
    expect(kFar).toBeLessThanOrEqual(16);
    expect(scaleForPixels(100, TAN_HALF, VH, TARGET, BASE)).toBeLessThan(
      scaleForPixels(300, TAN_HALF, VH, TARGET, BASE),
    );
  });

  it('viewportPx 减半（小窗）时 k 翻倍补偿，屏幕字高不变', () => {
    const kFull = scaleForPixels(200, TAN_HALF, VH, TARGET, BASE);
    const kHalf = scaleForPixels(200, TAN_HALF, VH / 2, TARGET, BASE);
    expect(kHalf / kFull).toBeCloseTo(2, 5);
  });
});

describe('filterOverlappingLabels（UX：屏幕空间防重叠，近景核心防叠字 soup）', () => {
  it('不重叠全部保留（输入顺序=度数降序）', () => {
    const keep = filterOverlappingLabels(
      [rect(-0.8, 0.8, 0.1, 0.05), rect(0, 0, 0.1, 0.05), rect(0.8, -0.8, 0.1, 0.05)],
      12,
    );
    expect(keep).toEqual([true, true, true]);
  });

  it('重叠时保留高度数（先来），隐藏后来者', () => {
    const keep = filterOverlappingLabels(
      [rect(0, 0, 0.2, 0.1), rect(0.05, 0.05, 0.2, 0.1), rect(0.9, 0.9, 0.05, 0.05)],
      12,
    );
    expect(keep).toEqual([true, false, true]);
  });

  it('超同屏上限截断；null（不可见项）跳过', () => {
    const rects = [rect(-0.9, 0.9, 0.05, 0.03), null, rect(0, 0.9, 0.05, 0.03), rect(0.9, 0.9, 0.05, 0.03)];
    expect(filterOverlappingLabels(rects, 2)).toEqual([true, false, true, false]);
  });

  it('贴边余量：间距小于 margin 视为重叠', () => {
    // 两矩形 x 间距 0.02 < 2×margin(0.015) → 隐藏后者
    const keep = filterOverlappingLabels([rect(0, 0, 0.1, 0.05), rect(0.21, 0, 0.1, 0.05)], 12);
    expect(keep).toEqual([true, false]);
  });
});

describe('effectiveMinDegree（G5-G：小图标签修复）', () => {
  it('大图维持基准值防拥挤', () => {
    expect(effectiveMinDegree(12, 4)).toBe(4);
    expect(effectiveMinDegree(4, 4)).toBe(4);
  });

  it('小图降档到最大度数：hub 标签在适应视图后可见', () => {
    expect(effectiveMinDegree(2, 4)).toBe(2);
    expect(effectiveMinDegree(1, 4)).toBe(1);
  });

  it('全孤立图（maxDegree 0）钳到 1：孤立节点不出标签', () => {
    expect(effectiveMinDegree(0, 4)).toBe(1);
  });
});
