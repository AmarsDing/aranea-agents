/**
 * interaction.spec：G5-D 交互状态机契约（设计 §V12.8-1 D-1/D-3/D-4）。
 * - oneHop 一跳邻居 O(E) 扫描正确性
 * - GraphInteraction hover/selected 分离、hover 优先、同值去抖
 * - wheelZoomFactor 0.95^(-ΔY·0.01)；isClickMovement <5px
 */
import { describe, expect, it } from 'vitest';
import { GraphInteraction, isClickMovement, oneHop, wheelZoomFactor } from '../interaction';

describe('oneHop 一跳邻居', () => {
  // 星形图：0 为中心，1/2/3 为叶；4 孤立
  const edges = new Int32Array([0, 1, 0, 2, 0, 3]);

  it('中心节点：邻居含自身+全部叶，边全中', () => {
    const r = oneHop(edges, 3, 0);
    expect([...r.nodes].sort()).toEqual([0, 1, 2, 3]);
    expect(r.edges.size).toBe(3);
  });

  it('叶节点：只命中一条边', () => {
    const r = oneHop(edges, 3, 2);
    expect([...r.nodes].sort()).toEqual([0, 2]);
    expect([...r.edges]).toEqual([1]);
  });

  it('孤立节点：仅自身、无边', () => {
    const r = oneHop(edges, 3, 4);
    expect([...r.nodes]).toEqual([4]);
    expect(r.edges.size).toBe(0);
  });

  it('无向命中：反向存储的边同样命中', () => {
    // 边存 (5,0) 反向，查 0 仍命中
    const rev = new Int32Array([5, 0]);
    const r = oneHop(rev, 1, 0);
    expect([...r.nodes].sort()).toEqual([0, 5]);
    expect([...r.edges]).toEqual([0]);
  });
});

describe('GraphInteraction 状态机', () => {
  it('初始全空', () => {
    const s = new GraphInteraction();
    expect(s.hover).toBeNull();
    expect(s.selected).toBeNull();
    expect(s.active).toBeNull();
  });

  it('hover 优先于 selected；hover 清除回落 selected', () => {
    const s = new GraphInteraction();
    s.setSelected(7);
    expect(s.active).toBe(7);
    s.setHover(3);
    expect(s.active).toBe(3); // hover 优先
    s.setHover(null);
    expect(s.active).toBe(7); // 回落选中锁定
  });

  it('setHover 同值去抖返回 false（防粒子相位重置）', () => {
    const s = new GraphInteraction();
    expect(s.setHover(3)).toBe(true);
    expect(s.setHover(3)).toBe(false);
    expect(s.setHover(null)).toBe(true);
    expect(s.setHover(null)).toBe(false);
  });

  it('setSelected 同值返回 false', () => {
    const s = new GraphInteraction();
    expect(s.setSelected(1)).toBe(true);
    expect(s.setSelected(1)).toBe(false);
    expect(s.setSelected(null)).toBe(true);
  });
});

describe('zoom/点击判别', () => {
  it('wheelZoomFactor：滚轮向下放大视野（factor>1 拉远）', () => {
    expect(wheelZoomFactor(100)).toBeCloseTo(1 / 0.95, 6);
    expect(wheelZoomFactor(-100)).toBeCloseTo(0.95, 6);
    expect(wheelZoomFactor(0)).toBe(1);
  });

  it('isClickMovement：位移 <5px 为点击', () => {
    expect(isClickMovement(0, 0)).toBe(true);
    expect(isClickMovement(3, 3)).toBe(true); // ~4.24px
    expect(isClickMovement(3, 4)).toBe(false); // 5px 整
    expect(isClickMovement(10, 0)).toBe(false);
  });
});
