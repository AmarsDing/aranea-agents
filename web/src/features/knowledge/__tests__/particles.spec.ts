// V2 ParticleField 纯函数：星光闪烁 / 流星生命周期 / 视差偏移 / 分层种子（方案 §三-V2）。
import { describe, expect, it } from 'vitest';
import {
  createMeteor,
  METEOR_LIFE,
  meteorHead,
  meteorProgress,
  nextMeteorDelay,
  parallaxOffset,
  seedField,
  twinkleAlpha,
} from '../particles';

const seq = (v: number) => () => v; // 定值 rng

describe('twinkleAlpha（星光闪烁）', () => {
  it('透明度在 [0.25, 0.85] 区间内振荡', () => {
    for (let t = 0; t < 20000; t += 137) {
      for (const phase of [0, 1.3, 3.1, 5.9]) {
        const a = twinkleAlpha(phase, t);
        expect(a).toBeGreaterThanOrEqual(0.25);
        expect(a).toBeLessThanOrEqual(0.85);
      }
    }
  });

  it('随时间推进产生可见变化（非恒定）', () => {
    const values = new Set<number>();
    for (let t = 0; t < 4000; t += 100) values.add(twinkleAlpha(0, t).toFixed(3));
    expect(values.size).toBeGreaterThan(4);
  });
});

describe('nextMeteorDelay（流星间隔）', () => {
  it('落在 4~8s 区间', () => {
    expect(nextMeteorDelay(seq(0))).toBe(4000);
    expect(nextMeteorDelay(seq(1))).toBe(8000);
    expect(nextMeteorDelay(seq(0.5))).toBe(6000);
  });
});

describe('createMeteor / meteorProgress / meteorHead（流星生命周期）', () => {
  it('从屏上缘外出生，斜向坠入', () => {
    const m = createMeteor(800, 1000, seq(0.5));
    expect(m.y).toBeLessThan(0); // 上缘外
    expect(m.x).toBeGreaterThanOrEqual(0);
    expect(m.x).toBeLessThanOrEqual(800);
    expect(m.dy).toBeGreaterThan(0); // 向下
    expect(m.life).toBe(METEOR_LIFE);
  });

  it('方向归一化', () => {
    const m = createMeteor(800, 0, seq(0.25));
    expect(Math.hypot(m.dx, m.dy)).toBeCloseTo(1, 5);
  });

  it('进度：出生 0 → 中点 0.5 → 寿终 1 → 超寿 >1', () => {
    const m = createMeteor(800, 1000, seq(0.5));
    expect(meteorProgress(m, 1000)).toBe(0);
    expect(meteorProgress(m, 1000 + m.life / 2)).toBe(0.5);
    expect(meteorProgress(m, 1000 + m.life)).toBe(1);
    expect(meteorProgress(m, 1000 + m.life * 2)).toBeGreaterThan(1);
  });

  it('头部按进度沿方向位移', () => {
    const m = createMeteor(800, 0, seq(0.5));
    const mid = meteorHead(m, m.life / 2);
    expect(mid.x).toBeCloseTo(m.x + m.dx * m.distance * 0.5, 5);
    expect(mid.y).toBeCloseTo(m.y + m.dy * m.distance * 0.5, 5);
  });
});

describe('parallaxOffset（视差偏移）', () => {
  it('鼠标在屏心时无偏移', () => {
    expect(parallaxOffset(400, 300, 800, 600, 1)).toEqual({ x: 0, y: 0 });
  });

  it('偏移与深度成正比（远层小于近层）', () => {
    const near = parallaxOffset(800, 600, 800, 600, 1);
    const far = parallaxOffset(800, 600, 800, 600, 0.35);
    expect(Math.abs(near.x)).toBeGreaterThan(Math.abs(far.x));
    expect(Math.abs(far.x)).toBeGreaterThan(0);
  });

  it('鼠标越界（离开画布）时钳位不爆', () => {
    const o = parallaxOffset(-9999, -9999, 800, 600, 1);
    expect(Math.abs(o.x)).toBeLessThanOrEqual(20);
    expect(Math.abs(o.y)).toBeLessThanOrEqual(20);
  });
});

describe('seedField（视差双层种子）', () => {
  it('按预算生成，全部带 twinkle 相位与双层深度', () => {
    const ps = seedField(800, 600, 40);
    expect(ps).toHaveLength(40);
    for (const p of ps) {
      expect(p.phase).toBeGreaterThanOrEqual(0);
      expect(p.phase).toBeLessThan(Math.PI * 2);
      expect([0.35, 1]).toContain(p.depth);
      expect(p.x).toBeGreaterThanOrEqual(0);
      expect(p.x).toBeLessThanOrEqual(800);
      expect(p.y).toBeGreaterThanOrEqual(0);
      expect(p.y).toBeLessThanOrEqual(600);
    }
  });

  it('远层粒子更小更慢', () => {
    const ps = seedField(800, 600, 200);
    const far = ps.filter((p) => p.depth === 0.35);
    const near = ps.filter((p) => p.depth === 1);
    expect(far.length).toBeGreaterThan(0);
    expect(near.length).toBeGreaterThan(0);
    const farMaxR = Math.max(...far.map((p) => p.r));
    const nearMaxR = Math.max(...near.map((p) => p.r));
    expect(farMaxR).toBeLessThan(nearMaxR);
  });
});
