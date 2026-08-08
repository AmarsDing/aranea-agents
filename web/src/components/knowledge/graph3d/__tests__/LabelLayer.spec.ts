/**
 * LabelLayer 纯函数单测：候选池 top-K + 双阈值可见性（forced 豁免距离，G5-C v2 修复）。
 */
import { describe, expect, it } from 'vitest';
import { selectLabelCandidates, shouldShowLabel } from '../render/LabelLayer';

describe('selectLabelCandidates', () => {
  it('按 degree 降序取 top-K', () => {
    const degree = new Uint16Array([1, 9, 3, 7, 5]);
    expect(selectLabelCandidates(degree, 3)).toEqual([1, 3, 4]);
  });

  it('K 超过节点数时返回全部', () => {
    const degree = new Uint16Array([2, 8]);
    expect(selectLabelCandidates(degree, 200)).toEqual([1, 0]);
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

  it('forced（hover/选中）豁免距离阈值：适应视图后 hover 必须出标签', () => {
    // zoomToFit 后相机距离 ≈ fitDist，maxDistance = fitDist×0.85 → 距离必超阈值
    expect(shouldShowLabel(1000, 600, 0, 4, true, true)).toBe(true);
    expect(shouldShowLabel(1000, 600, 0, 4, true, false)).toBe(true);
  });
});
