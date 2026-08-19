/**
 * parking.spec：V13-A1 停泊环 + V13-B tier 壳层 + 主簇统计契约。
 */
import { describe, expect, it } from 'vitest';
import {
  buildPinnedInit,
  buildTierTargetRadii,
  clusterStatsP90,
  estimateOuterRadius,
  isolatedIndices,
  PARK_ARC_MIN,
  PARK_RING_SPACING,
  parkingRingPositions,
  TIER_RADIUS_RATIO,
} from '../parking';
import { TIER_REGULAR, TIER_SUPERNODE, TIER_ULTRANODE } from '../tiering';

describe('isolatedIndices / buildPinnedInit', () => {
  it('degree=0 节点被收集，其余跳过', () => {
    const degree = Uint16Array.from([0, 2, 0, 1, 0]);
    expect(Array.from(isolatedIndices(degree))).toEqual([0, 2, 4]);
    const mask = buildPinnedInit(degree);
    expect(mask).not.toBeNull();
    expect(Array.from(mask!)).toEqual([1, 0, 1, 0, 1]);
  });

  it('无孤立节点时 pinnedInit 返回 null', () => {
    expect(buildPinnedInit(Uint16Array.from([1, 2, 3]))).toBeNull();
  });
});

describe('buildTierTargetRadii', () => {
  it('tier 比例映射 + degree≤1 不分层（-1）', () => {
    const tiers = Uint8Array.from([TIER_ULTRANODE, TIER_SUPERNODE, TIER_REGULAR, TIER_REGULAR, TIER_REGULAR]);
    const degree = Uint16Array.from([20, 16, 5, 1, 0]);
    const out = buildTierTargetRadii(tiers, degree, 100);
    expect(out[0]).toBeCloseTo(100 * TIER_RADIUS_RATIO[TIER_ULTRANODE]);
    expect(out[1]).toBeCloseTo(100 * TIER_RADIUS_RATIO[TIER_SUPERNODE]);
    expect(out[2]).toBeCloseTo(100 * TIER_RADIUS_RATIO[TIER_REGULAR]);
    expect(out[3]).toBe(-1); // 末梢
    expect(out[4]).toBe(-1); // 孤立
  });
});

describe('estimateOuterRadius', () => {
  it('随 cbrt(N) 增长且有下限', () => {
    expect(estimateOuterRadius(1)).toBe(30);
    expect(estimateOuterRadius(1000)).toBeCloseTo(65);
    expect(estimateOuterRadius(8000)).toBeCloseTo(130);
  });
});

describe('parkingRingPositions', () => {
  it('单环：所有点在 XZ 平面等半径、等角间距', () => {
    const count = 8;
    const radius = 100; // 容量 floor(628/10)=62 ≥ 8 → 单环
    const p = parkingRingPositions(count, radius);
    expect(p.length).toBe(count * 3);
    for (let i = 0; i < count; i++) {
      const r = Math.hypot(p[i * 3], p[i * 3 + 2]);
      expect(r).toBeCloseTo(radius, 4);
      expect(p[i * 3 + 1]).toBe(0);
    }
    // 等角间距：相邻点弦长一致
    const chord = (a: number, b: number) =>
      Math.hypot(p[a * 3] - p[b * 3], p[a * 3 + 1] - p[b * 3 + 1], p[a * 3 + 2] - p[b * 3 + 2]);
    const c0 = chord(0, 1);
    for (let i = 1; i < count - 1; i++) expect(chord(i, i + 1)).toBeCloseTo(c0, 4);
  });

  it('多环：过密时外扩同心环，弧长不低于 PARK_ARC_MIN', () => {
    const radius = 50; // 环0容量 floor(2π·50/10)=31，环1容量 floor(2π·62/10)=38
    const count = 50; // 环0 31 个（idx 0-30），环1 19 个（idx 31-49）
    const p = parkingRingPositions(count, radius);
    expect(p.length).toBe(count * 3);
    const ringOf = (i: number) => Math.round((Math.hypot(p[i * 3], p[i * 3 + 2]) - radius) / PARK_RING_SPACING) + 0; // +0 归一化 -0
    expect(ringOf(0)).toBe(0);
    expect(ringOf(30)).toBe(0);
    expect(ringOf(31)).toBe(1);
    expect(ringOf(49)).toBe(1);
    // 每环内相邻弧长 ≥ PARK_ARC_MIN（含闭环段）
    for (let ringStart = 0; ringStart < count; ) {
      const ring = ringOf(ringStart);
      let ringEnd = ringStart;
      while (ringEnd + 1 < count && ringOf(ringEnd + 1) === ring) ringEnd++;
      const ringCount = ringEnd - ringStart + 1;
      const r = radius + ring * PARK_RING_SPACING;
      const arc = (2 * Math.PI * r) / ringCount;
      expect(arc).toBeGreaterThanOrEqual(PARK_ARC_MIN);
      ringStart = ringEnd + 1;
    }
  });

  it('确定性：同参数两次生成完全一致；count=0 返回空', () => {
    const a = parkingRingPositions(30, 80);
    const b = parkingRingPositions(30, 80);
    expect(Array.from(a)).toEqual(Array.from(b));
    expect(parkingRingPositions(0, 80).length).toBe(0);
  });
});

describe('clusterStatsP90', () => {
  it('排除掩码节点后质心/半径只看主簇', () => {
    // 4 个主簇点在原点附近，1 个孤立点在 (1000,0,0)
    const positions = Float32Array.from([10, 0, 0, -10, 0, 0, 0, 10, 0, 0, -10, 0, 1000, 0, 0]);
    const exclude = Uint8Array.from([0, 0, 0, 0, 1]);
    const stats = clusterStatsP90(positions, 5, exclude, 0);
    expect(stats.included).toBe(4);
    expect(stats.cx).toBeCloseTo(0);
    expect(stats.cy).toBeCloseTo(0);
    expect(stats.cz).toBeCloseTo(0);
    expect(stats.radius).toBeCloseTo(10, 4);
  });

  it('无掩码/全排除时回退全量；半径下限生效', () => {
    const positions = Float32Array.from([1, 0, 0, -1, 0, 0]);
    const all = clusterStatsP90(positions, 2, null, 0);
    expect(all.included).toBe(2);
    expect(all.radius).toBeCloseTo(1, 4);
    const fallback = clusterStatsP90(positions, 2, Uint8Array.from([1, 1]), 0);
    expect(fallback.included).toBe(2);
    const floored = clusterStatsP90(positions, 2, null, 20);
    expect(floored.radius).toBe(20);
  });
});
