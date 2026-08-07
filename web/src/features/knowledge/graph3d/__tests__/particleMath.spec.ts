/**
 * particleMath.spec：G5-A 粒子流纯数学契约（fast-graph 1:1，设计 §V12.8-1 particleMath.ts）。
 */
import { describe, expect, it } from 'vitest';
import {
  PARTICLE_MAX,
  PARTICLE_SPEED,
  advancePhase,
  easeInOutQuad,
  particleHsl,
  particlePosition,
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

describe('particleHsl', () => {
  it('时变彩虹：hue ∈ [0.18, 0.82]，sat=0.9，light=0.62', () => {
    for (const t of [0, 1.3, 7.7]) {
      for (const p of [0, 0.33, 0.99]) {
        const { h, s, l } = particleHsl(t, p, 0);
        expect(h).toBeGreaterThanOrEqual(0.18 - 1e-6);
        expect(h).toBeLessThanOrEqual(0.82 + 1e-6);
        expect(s).toBe(0.9);
        expect(l).toBe(0.62);
      }
    }
  });

  it('公式契约：hue=0.5+0.32·sin((t·0.6+p·2.2+i·0.12)·π)', () => {
    const { h } = particleHsl(2, 0.5, 3);
    expect(h).toBeCloseTo(0.5 + 0.32 * Math.sin((2 * 0.6 + 0.5 * 2.2 + 3 * 0.12) * Math.PI), 6);
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
