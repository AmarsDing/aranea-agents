/**
 * octree.spec：G5-A typed-array 八叉树契约（移植 fast-graph，设计 §V12.8-1 octree.ts）。
 */
import { describe, expect, it } from 'vitest';
import { Octree } from '../octree';

function forceOn(tree: Octree, i: number, theta: number, repulsion: number): [number, number, number] {
  const out = new Float32Array(3);
  tree.computeForce(i, theta, repulsion, out);
  return [out[0], out[1], out[2]];
}

describe('Octree', () => {
  it('空树：力为零', () => {
    const t = new Octree(4);
    t.rebuild(new Float32Array(0), 0);
    expect(forceOn(t, 0, 0.8, 30)).toEqual([0, 0, 0]);
  });

  it('两节点：斥力方向相反、沿连线远离', () => {
    // a 在原点，b 在 +x 方向 10 处
    const pos = new Float32Array([0, 0, 0, 10, 0, 0]);
    const t = new Octree(2);
    t.rebuild(pos, 2);
    const fa = forceOn(t, 0, 0.8, 30);
    const fb = forceOn(t, 1, 0.8, 30);
    expect(fa[0]).toBeLessThan(0); // a 被推向 -x
    expect(fb[0]).toBeGreaterThan(0); // b 被推向 +x
    expect(Math.abs(fa[1])).toBeLessThan(1e-6);
    expect(Math.abs(fb[1])).toBeLessThan(1e-6);
    // 牛顿第三定律（对称质量）
    expect(fa[0]).toBeCloseTo(-fb[0], 5);
    // 大小 ≈ repulsion / d² = 30/100 = 0.3
    expect(Math.abs(fb[0])).toBeCloseTo(0.3, 1);
  });

  it('远距簇近似：BH 与逐对直算误差 < 5%', () => {
    // 远簇 100 个节点挤在 (1000,0,0) 附近，对原点节点的力
    const n = 101;
    const pos = new Float32Array(n * 3);
    pos[0] = 0;
    pos[1] = 0;
    pos[2] = 0;
    for (let i = 1; i < n; i++) {
      pos[i * 3] = 1000 + (i % 5);
      pos[i * 3 + 1] = i % 7;
      pos[i * 3 + 2] = i % 3;
    }
    const tree = new Octree(n);
    tree.rebuild(pos, n);
    const approx = forceOn(tree, 0, 0.8, 30);

    // 逐对直算
    let ex = 0,
      ey = 0,
      ez = 0;
    for (let i = 1; i < n; i++) {
      const dx = pos[0] - pos[i * 3];
      const dy = pos[1] - pos[i * 3 + 1];
      const dz = pos[2] - pos[i * 3 + 2];
      const d2 = dx * dx + dy * dy + dz * dz;
      const d = Math.sqrt(d2);
      const f = 30 / d2;
      ex += (dx / d) * f;
      ey += (dy / d) * f;
      ez += (dz / d) * f;
    }
    const magA = Math.hypot(...approx);
    const magE = Math.hypot(ex, ey, ez);
    expect(Math.abs(magA - magE) / magE).toBeLessThan(0.05);
  });

  it('rebuild 可重复调用（池复用，结果一致）', () => {
    const pos = new Float32Array([0, 0, 0, 5, 5, 5, -3, 2, 1]);
    const t = new Octree(3);
    t.rebuild(pos, 3);
    const first = forceOn(t, 0, 0.8, 30);
    t.rebuild(pos, 3);
    expect(forceOn(t, 0, 0.8, 30)).toEqual(first);
  });

  it('大容量倍增不丢失：8k 节点插入后力有限且无 NaN', () => {
    const n = 8000;
    const pos = new Float32Array(n * 3);
    for (let i = 0; i < n; i++) {
      pos[i * 3] = (i % 97) * 10;
      pos[i * 3 + 1] = (i % 89) * 10;
      pos[i * 3 + 2] = (i % 83) * 10;
    }
    const t = new Octree(64); // 故意小初始容量触发 grow
    t.rebuild(pos, n);
    const f = forceOn(t, 0, 0.8, 30);
    expect(Number.isFinite(f[0])).toBe(true);
    expect(Number.isFinite(f[1])).toBe(true);
    expect(Number.isFinite(f[2])).toBe(true);
  });
});
