import { describe, expect, it } from 'vitest';

import { clampAmplitude, hudParamsFor, TINT_AMBER, TINT_CYAN, TINT_GREEN, TINT_RED } from '../hud/hudParams';

describe('hudParamsFor — 能量核缩放（设计 §7.4）', () => {
  it('idle 呼吸：sin 0.5Hz ±4%', () => {
    // 0.5Hz → 周期 2s；t=0.5s 处 sin(π/2)=1 → 1.04
    expect(hudParamsFor('idle', 0.5, 0).coreScale).toBeCloseTo(1.04, 5);
    // t=1.5s 处 sin(3π/2)=-1 → 0.96
    expect(hudParamsFor('idle', 1.5, 0).coreScale).toBeCloseTo(0.96, 5);
    // t=0 / 2s 处回到 1.0
    expect(hudParamsFor('idle', 0, 0).coreScale).toBeCloseTo(1.0, 5);
  });

  it('thinking 收缩 0.85×', () => {
    expect(hudParamsFor('thinking', 3.3, 0).coreScale).toBeCloseTo(0.85, 5);
  });

  it('speaking 随振幅 1.0-1.15×', () => {
    expect(hudParamsFor('speaking', 0, 0).coreScale).toBeCloseTo(1.0, 5);
    expect(hudParamsFor('speaking', 0, 0.5).coreScale).toBeCloseTo(1.075, 5);
    expect(hudParamsFor('speaking', 0, 1).coreScale).toBeCloseTo(1.15, 5);
  });

  it('speaking 振幅越界被钳制（>1 不超过 1.15×）', () => {
    expect(hudParamsFor('speaking', 0, 2.7).coreScale).toBeCloseTo(1.15, 5);
    expect(hudParamsFor('speaking', 0, -1).coreScale).toBeCloseTo(1.0, 5);
  });
});

describe('hudParamsFor — 粒子环与频谱环', () => {
  it('listening 外环展开 1.2×，其余 1.0×', () => {
    expect(hudParamsFor('listening', 0, 0).outerRingScale).toBeCloseTo(1.2, 5);
    expect(hudParamsFor('idle', 0, 0).outerRingScale).toBeCloseTo(1.0, 5);
    expect(hudParamsFor('speaking', 0, 0).outerRingScale).toBeCloseTo(1.0, 5);
  });

  it('thinking 转速 ×3，其余 ×1', () => {
    expect(hudParamsFor('thinking', 0, 0).ringSpeedFactor).toBe(3);
    expect(hudParamsFor('listening', 0, 0).ringSpeedFactor).toBe(1);
  });

  it('频谱环仅 listening 可见', () => {
    expect(hudParamsFor('listening', 0, 0).spectrumVisible).toBe(true);
    for (const s of ['idle', 'thinking', 'speaking', 'interrupted', 'error'] as const) {
      expect(hudParamsFor(s, 0, 0).spectrumVisible).toBe(false);
    }
  });
});

describe('hudParamsFor — 颜色 tint', () => {
  it('idle/listening/speaking = 青蓝系渐变（cyan → green）', () => {
    for (const s of ['idle', 'listening', 'speaking'] as const) {
      const p = hudParamsFor(s, 0, 0);
      expect(p.tintA).toBe(TINT_CYAN);
      expect(p.tintB).toBe(TINT_GREEN);
    }
  });

  it('thinking = 琥珀', () => {
    const p = hudParamsFor('thinking', 0, 0);
    expect(p.tintA).toBe(TINT_AMBER);
    expect(p.tintB).toBe(TINT_AMBER);
  });

  it('interrupted/error = 红', () => {
    expect(hudParamsFor('interrupted', 0, 0).tintA).toBe(TINT_RED);
    expect(hudParamsFor('error', 0, 0).tintA).toBe(TINT_RED);
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

describe('hudParamsFor — 声波震动增益（V2-T5 HUD 增强）', () => {
  it('listening/speaking 震动增益 = 1（音频驱动全开）', () => {
    expect(hudParamsFor('listening', 0, 0).vibrationGain).toBe(1);
    expect(hudParamsFor('speaking', 0, 0).vibrationGain).toBe(1);
  });

  it('thinking 微抖动 0.25；idle/interrupted/error 无震动', () => {
    expect(hudParamsFor('thinking', 0, 0).vibrationGain).toBeCloseTo(0.25, 5);
    for (const s of ['idle', 'interrupted', 'error'] as const) {
      expect(hudParamsFor(s, 0, 0).vibrationGain).toBe(0);
    }
  });
});

describe('hudParamsFor — 全息弧线转速因子', () => {
  it('thinking 最快（2.8），listening 次之（1.6）', () => {
    expect(hudParamsFor('thinking', 0, 0).arcSpeedFactor).toBeCloseTo(2.8, 5);
    expect(hudParamsFor('listening', 0, 0).arcSpeedFactor).toBeCloseTo(1.6, 5);
  });

  it('idle/speaking 常速；interrupted/error 近停滞', () => {
    expect(hudParamsFor('idle', 0, 0).arcSpeedFactor).toBe(1);
    expect(hudParamsFor('speaking', 0, 0).arcSpeedFactor).toBeCloseTo(1.4, 5);
    for (const s of ['interrupted', 'error'] as const) {
      expect(hudParamsFor(s, 0, 0).arcSpeedFactor).toBeCloseTo(0.4, 5);
    }
  });
});

describe('hudParamsFor — 能量核顶点波动', () => {
  it('thinking 波动最剧（0.09），idle 最缓（0.03）', () => {
    expect(hudParamsFor('thinking', 0, 0).coreWobble).toBeCloseTo(0.09, 5);
    expect(hudParamsFor('idle', 0, 0).coreWobble).toBeCloseTo(0.03, 5);
  });

  it('listening/speaking 中等波动（音频叠加在场景侧）', () => {
    expect(hudParamsFor('listening', 0, 0).coreWobble).toBeCloseTo(0.05, 5);
    expect(hudParamsFor('speaking', 0, 0).coreWobble).toBeCloseTo(0.06, 5);
  });
});

describe('hudParamsFor — 声浪涟漪增益', () => {
  it('仅 speaking（1）/listening（0.5）发射涟漪', () => {
    expect(hudParamsFor('speaking', 0, 0).rippleGain).toBe(1);
    expect(hudParamsFor('listening', 0, 0).rippleGain).toBeCloseTo(0.5, 5);
    for (const s of ['idle', 'thinking', 'interrupted', 'error'] as const) {
      expect(hudParamsFor(s, 0, 0).rippleGain).toBe(0);
    }
  });
});
