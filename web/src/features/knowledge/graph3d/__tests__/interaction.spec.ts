/**
 * interaction.spec：G5-D 交互状态机契约（设计 §V12.8-1 D-1/D-3/D-4）。
 * - oneHop 一跳邻居 O(E) 扫描正确性
 * - GraphInteraction hover/selected 分离、hover 优先、同值去抖
 * - wheelZoomFactor 0.95^(-ΔY·0.01)；isClickMovement <5px
 */
import { describe, expect, it } from 'vitest';
import { GraphInteraction, isClickMovement, nHop, oneHop, wheelZoomFactor } from '../interaction';

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

describe('M4 聚焦模式', () => {
  // 图：0-1-2-3 链 + 0-4 支链
  const edges = new Int32Array([0, 1, 1, 2, 2, 3, 0, 4]);
  const edgeCount = 4;

  it('nHop(root, 1) = 一跳邻居（与 oneHop 一致）', () => {
    const { nodes } = nHop(edges, edgeCount, 0, 1);
    expect([...nodes].sort()).toEqual([0, 1, 4]);
  });

  it('nHop(root, 2) = 二跳邻居（含 2）', () => {
    const { nodes } = nHop(edges, edgeCount, 0, 2);
    expect([...nodes].sort()).toEqual([0, 1, 2, 4]);
  });

  it('nHop(root, 0) = 仅根节点', () => {
    const { nodes } = nHop(edges, edgeCount, 0, 0);
    expect([...nodes]).toEqual([0]);
  });

  it('nHop 边集 = 两端点都在节点集内的边', () => {
    const { edges: edgeSet } = nHop(edges, edgeCount, 0, 1);
    expect(edgeSet.has(0)).toBe(true); // 0-1
    expect(edgeSet.has(3)).toBe(true); // 0-4
    expect(edgeSet.has(1)).toBe(false); // 1-2 出圈
  });

  it('GraphInteraction 聚焦锁定：focus 后 hover 不覆盖；clearFocus 恢复 hover 驱动', () => {
    const gi = new GraphInteraction();
    gi.setFocus(0, 2);
    expect(gi.focused).toBe(0);
    expect(gi.focusHops).toBe(2);
    gi.setHover(1);
    expect(gi.focused).toBe(0); // hover 不覆盖锁定
    gi.clearFocus();
    expect(gi.focused).toBeNull();
  });
});
