/**
 * particleMath.spec：G5-A 粒子流纯数学契约（fast-graph 1:1，设计 §V12.8-1 particleMath.ts）。
 */
import { describe, expect, it } from 'vitest';
import {
  PARTICLE_MAX,
  PARTICLE_SPEED,
  advancePhase,
  easeInOutQuad,
  particlePosition,
  particleTint,
  spreadPhases,
} from '../particleMath';

describe('spreadPhases', () => {
  it('相位均布：prog[i]=i/n，覆盖 [0,1)', () => {
    const p = spreadPhases(4);
    expect(Array.from(p)).toEqual([0, 0.25, 0.5, 0.75]);
  });

  it('n=1 时相位为 0', () => {
    expect(spreadPhases(1)[0]).toBe(0);
  });
});

describe('advancePhase', () => {
  it('按 SPEED 推进并对 1 回绕', () => {
    expect(advancePhase(0.9, 0.5)).toBeCloseTo((0.9 + 0.5 * PARTICLE_SPEED) % 1, 6);
    expect(advancePhase(0, 0)).toBe(0);
  });
});

describe('easeInOutQuad', () => {
  it('端点恒等：f(0)=0, f(1)=1，中点 0.5', () => {
    expect(easeInOutQuad(0)).toBe(0);
    expect(easeInOutQuad(1)).toBe(1);
    expect(easeInOutQuad(0.5)).toBe(0.5);
  });

  it('前半段减速上凸：f(0.25)=0.125；后半段对称', () => {
    expect(easeInOutQuad(0.25)).toBeCloseTo(0.125, 6);
    expect(easeInOutQuad(0.75)).toBeCloseTo(0.875, 6);
  });
});

describe('particleTint', () => {
  it('单色青：hue ∈ [0.50, 0.54]，sat=0.9，light ∈ [0.5, 0.64]', () => {
    for (const t of [0, 1.3, 7.7]) {
      for (const p of [0, 0.33, 0.99]) {
        const { h, s, l } = particleTint(t, p, 0);
        expect(h).toBeGreaterThanOrEqual(0.5 - 1e-6);
        expect(h).toBeLessThanOrEqual(0.54 + 1e-6);
        expect(s).toBe(0.9);
        expect(l).toBeGreaterThanOrEqual(0.5 - 1e-6);
        expect(l).toBeLessThanOrEqual(0.64 + 1e-6);
      }
    }
  });

  it('公式契约：hue=0.52+0.02·sin(w·0.31)，light=0.5+0.14·(0.5+0.5·sin w)，w=t·2.6-p·2π+i·0.12', () => {
    const w = 2 * 2.6 - 0.5 * 6.2831853 + 3 * 0.12;
    const { h, l } = particleTint(2, 0.5, 3);
    expect(h).toBeCloseTo(0.52 + 0.02 * Math.sin(w * 0.31), 6);
    expect(l).toBeCloseTo(0.5 + 0.14 * (0.5 + 0.5 * Math.sin(w)), 6);
  });

  it('亮度随相位呼吸（同刻不同相位亮度不同）', () => {
    const a = particleTint(1, 0, 0).l;
    const b = particleTint(1, 0.4, 0).l;
    expect(Math.abs(a - b)).toBeGreaterThan(0.01);
  });
});

describe('particlePosition', () => {
  it('ease=0 在源点，ease=1 在终点，中间线性插值', () => {
    const out = new Float32Array(3);
    particlePosition([1, 2, 3], [11, 22, 33], 0, out, 0);
    expect(Array.from(out)).toEqual([1, 2, 3]);
    particlePosition([1, 2, 3], [11, 22, 33], 1, out, 0);
    expect(Array.from(out)).toEqual([11, 22, 33]);
    particlePosition([0, 0, 0], [10, 20, 30], 0.5, out, 0);
    expect(Array.from(out)).toEqual([5, 10, 15]);
  });

  it('支持 offset 写入共享缓冲', () => {
    const out = new Float32Array(6);
    particlePosition([0, 0, 0], [2, 4, 6], 0.5, out, 3);
    expect(Array.from(out)).toEqual([0, 0, 0, 1, 2, 3]);
  });
});

describe('常量契约', () => {
  it('MAX=80，SPEED=0.45（fast-graph 原值）', () => {
    expect(PARTICLE_MAX).toBe(80);
    expect(PARTICLE_SPEED).toBe(0.45);
  });
});
