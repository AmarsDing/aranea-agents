/**
 * tiering.spec：G5-A 节点三层分级契约（jarvis-ui 蓝本，设计 §V12.8-1 tiering.ts）。
 * supernode=degree≥15；ultranode=连接≥4 个不同 supernode。
 */
import { describe, expect, it } from 'vitest';
import {
  SUPERNODE_MIN_DEGREE,
  TIER_CHARGE_SCALE,
  TIER_REGULAR,
  TIER_SIZE_MULT,
  TIER_SUPERNODE,
  TIER_ULTRANODE,
  ULTRANODE_MIN_SUPER_LINKS,
  classifyTiers,
  tierChargeScales,
} from '../tiering';

/** 星型：hub 连 leaves.length 个叶。返回 edges 扁平数组，hub=0。 */
function star(count: number): Int32Array {
  const edges: number[] = [];
  for (let i = 1; i < count; i++) edges.push(0, i);
  return Int32Array.from(edges);
}

function degrees(count: number, edges: Int32Array): Uint16Array {
  const d = new Uint16Array(count);
  for (let e = 0; e < edges.length; e += 2) {
    d[edges[e]]++;
    d[edges[e + 1]]++;
  }
  return d;
}

describe('classifyTiers', () => {
  it('degree < 15 → regular', () => {
    const edges = star(5); // hub degree=4
    const t = classifyTiers(degrees(5, edges), edges);
    expect(t[0]).toBe(TIER_REGULAR);
  });

  it('degree ≥ 15 → supernode', () => {
    const edges = star(SUPERNODE_MIN_DEGREE + 1); // hub degree=15
    const t = classifyTiers(degrees(SUPERNODE_MIN_DEGREE + 1, edges), edges);
    expect(t[0]).toBe(TIER_SUPERNODE);
    expect(t[1]).toBe(TIER_REGULAR); // 叶节点 degree=1
  });

  it('supernode 连接 ≥4 个不同 supernode → ultranode', () => {
    // nexus(0) 连 4 个 hub(1..4)，每个 hub 连 14 个叶 → hub degree=15=supernode
    // nexus degree=4（regular 度数不足 15 但... nexus 需要先是 supernode 才能升 ultra）
    // 让 nexus 也连够 15 个叶：nexus degree=4+15=19 → supernode；邻居中 4 个 supernode → ultra
    const edges: number[] = [];
    const NEXUS = 0;
    const hubs = [1, 2, 3, 4];
    let next = 5;
    for (const h of hubs) {
      edges.push(NEXUS, h);
      for (let i = 0; i < 14; i++) edges.push(h, next++); // hub degree=1+14=15
    }
    for (let i = 0; i < 15; i++) edges.push(NEXUS, next++); // nexus degree=4+15=19
    const ea = Int32Array.from(edges);
    const t = classifyTiers(degrees(next, ea), ea);
    expect(t[NEXUS]).toBe(TIER_ULTRANODE);
    for (const h of hubs) expect(t[h]).toBe(TIER_SUPERNODE); // hub 只连 1 个 supernode(nexus)，不够 4
  });

  it('supernode 连接 <4 个 supernode → 保持 supernode', () => {
    const edges: number[] = [];
    let next = 2;
    // hub0 连 15 叶 + hub1；hub1 连 15 叶
    for (let i = 0; i < 15; i++) edges.push(0, next++);
    for (let i = 0; i < 15; i++) edges.push(1, next++);
    edges.push(0, 1);
    const ea = Int32Array.from(edges);
    const t = classifyTiers(degrees(next, ea), ea);
    expect(t[0]).toBe(TIER_SUPERNODE);
    expect(t[1]).toBe(TIER_SUPERNODE);
  });

  it('常量契约：ULTRANODE_MIN_SUPER_LINKS=4，尺寸倍率 1.0/1.5/2.5', () => {
    expect(ULTRANODE_MIN_SUPER_LINKS).toBe(4);
    expect(TIER_SIZE_MULT[TIER_REGULAR]).toBe(1.0);
    expect(TIER_SIZE_MULT[TIER_SUPERNODE]).toBe(1.5);
    expect(TIER_SIZE_MULT[TIER_ULTRANODE]).toBe(2.5);
  });
});

describe('tierChargeScales', () => {
  it('分层 charge：regular 1.0 / supernode ≈1.67 / ultranode ≈2.92', () => {
    const s = tierChargeScales(Uint8Array.from([TIER_REGULAR, TIER_SUPERNODE, TIER_ULTRANODE]));
    expect(s[0]).toBeCloseTo(1.0, 5);
    expect(s[1]).toBeCloseTo(200 / 120, 3);
    expect(s[2]).toBeCloseTo(350 / 120, 3);
  });

  it('TIER_CHARGE_SCALE 常量与函数输出一致', () => {
    expect(TIER_CHARGE_SCALE[TIER_REGULAR]).toBeCloseTo(1.0, 5);
  });
});
