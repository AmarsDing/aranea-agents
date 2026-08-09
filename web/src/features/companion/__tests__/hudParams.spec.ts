import { describe, expect, it } from 'vitest';

import { clampAmplitude, hudParamsFor, TINT_ORB_A, TINT_ORB_B, TINT_RED_A, TINT_RED_B } from '../hud/hudParams';

// V7（TwinSprite 光球复刻）参数规格：uAmp/环速/配色常量与 TwinSprite SpriteOrb 一致。
describe('hudParamsFor — 顶点噪声振幅（TwinSprite uAmp = 0.12 + level×0.38）', () => {
  it('全状态基值 0.12、电平增益 0.38', () => {
    for (const s of ['idle', 'listening', 'thinking', 'speaking', 'interrupted', 'error'] as const) {
      const p = hudParamsFor(s, 0);
      expect(p.ampBase).toBeCloseTo(0.12, 5);
      expect(p.ampGain).toBeCloseTo(0.38, 5);
    }
  });
});

describe('hudParamsFor — 状态差异化（thinking：高速噪声 + 环速 ×3 + 核心收缩）', () => {
  it('thinking 噪声流速 ×3、环转速 ×3、核心收缩 0.85×', () => {
    const p = hudParamsFor('thinking', 0);
    expect(p.noiseSpeedFactor).toBe(3);
    expect(p.ringSpeedFactor).toBe(3);
    expect(p.orbScale).toBeCloseTo(0.85, 5);
  });

  it('其余状态默认流速/转速 ×1、缩放 1.0', () => {
    for (const s of ['idle', 'listening', 'speaking', 'interrupted', 'error'] as const) {
      const p = hudParamsFor(s, 0);
      expect(p.noiseSpeedFactor).toBe(1);
      expect(p.ringSpeedFactor).toBe(1);
      expect(p.orbScale).toBe(1);
    }
  });

  it('震动增益：仅 speaking，其余状态无震动', () => {
    expect(hudParamsFor('speaking', 0).shakeGain).toBe(1);
    for (const s of ['idle', 'listening', 'thinking', 'interrupted', 'error'] as const) {
      expect(hudParamsFor(s, 0).shakeGain).toBe(0);
    }
  });
});

describe('hudParamsFor — 颜色 tint（TwinSprite 深蓝→青；红仅警示）', () => {
  it('idle/listening/thinking/speaking = TwinSprite 光球配色 #123a6e → #4dd8e8', () => {
    for (const s of ['idle', 'listening', 'thinking', 'speaking'] as const) {
      const p = hudParamsFor(s, 0);
      expect(p.tintA).toBe(TINT_ORB_A);
      expect(p.tintB).toBe(TINT_ORB_B);
    }
  });

  it('interrupted/error = 红系', () => {
    expect(hudParamsFor('interrupted', 0).tintA).toBe(TINT_RED_A);
    expect(hudParamsFor('interrupted', 0).tintB).toBe(TINT_RED_B);
    expect(hudParamsFor('error', 0).tintA).toBe(TINT_RED_A);
    expect(hudParamsFor('error', 0).tintB).toBe(TINT_RED_B);
  });
});

describe('hudParamsFor — 启动过场 boot（待机微光 0.35 → 满功率 1）', () => {
  it('boot=0 待机：intensity 0.35', () => {
    expect(hudParamsFor('idle', 0, 0).intensity).toBeCloseTo(0.35, 5);
  });

  it('boot=1 满功率：intensity 1（与默认值一致）', () => {
    expect(hudParamsFor('idle', 0, 1).intensity).toBeCloseTo(1, 5);
    expect(hudParamsFor('idle', 0).intensity).toBeCloseTo(1, 5);
  });

  it('boot=0.5 线性过渡 0.675', () => {
    expect(hudParamsFor('idle', 0, 0.5).intensity).toBeCloseTo(0.675, 5);
  });

  it('boot 越界被钳制到 [0,1]', () => {
    expect(hudParamsFor('idle', 0, -1).intensity).toBeCloseTo(0.35, 5);
    expect(hudParamsFor('idle', 0, 2).intensity).toBeCloseTo(1, 5);
  });
});

describe('clampAmplitude', () => {
  it('钳制到 [0, 1]', () => {
    expect(clampAmplitude(0.4)).toBe(0.4);
    expect(clampAmplitude(-0.2)).toBe(0);
    expect(clampAmplitude(1.8)).toBe(1);
    expect(clampAmplitude(Number.NaN)).toBe(0);
  });
});
