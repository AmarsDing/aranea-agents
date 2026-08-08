import { describe, expect, it } from 'vitest';

import { clampAmplitude, hudParamsFor, TINT_CYAN, TINT_GREEN, TINT_RED } from '../hud/hudParams';

describe('hudParamsFor — 能量核缩放（设计 §7.4 v2）', () => {
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

describe('hudParamsFor — 刻度环与频谱环', () => {
  it('listening 刻度环展开 1.2×，其余 1.0×', () => {
    expect(hudParamsFor('listening', 0, 0).ringExpand).toBeCloseTo(1.2, 5);
    expect(hudParamsFor('idle', 0, 0).ringExpand).toBeCloseTo(1.0, 5);
    expect(hudParamsFor('speaking', 0, 0).ringExpand).toBeCloseTo(1.0, 5);
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

describe('hudParamsFor — Bloom 辉光强度（V5-T1）', () => {
  it('listening 最强（1.0），speaking 基础 1.1 随振幅增益', () => {
    expect(hudParamsFor('listening', 0, 0).bloomIntensity).toBeCloseTo(1.0, 5);
    expect(hudParamsFor('speaking', 0, 0).bloomIntensity).toBeCloseTo(1.1, 5);
    expect(hudParamsFor('speaking', 0, 1).bloomIntensity).toBeCloseTo(1.5, 5);
  });

  it('interrupted 红闪提亮 1.3；idle 0.7 / thinking 0.9 / error 0.8', () => {
    expect(hudParamsFor('interrupted', 0, 0).bloomIntensity).toBeCloseTo(1.3, 5);
    expect(hudParamsFor('idle', 0, 0).bloomIntensity).toBeCloseTo(0.7, 5);
    expect(hudParamsFor('thinking', 0, 0).bloomIntensity).toBeCloseTo(0.9, 5);
    expect(hudParamsFor('error', 0, 0).bloomIntensity).toBeCloseTo(0.8, 5);
  });

  it('speaking 振幅越界时 bloom 增益被钳制', () => {
    expect(hudParamsFor('speaking', 0, 9).bloomIntensity).toBeCloseTo(1.5, 5);
  });
});

describe('hudParamsFor — 启动过场 boot（V5-T3）', () => {
  it('boot=0 待机：核心收缩 0.75×、bloom 压至 35%、刻度环收缩 0.8×', () => {
    const p = hudParamsFor('idle', 0, 0, 0);
    expect(p.coreScale).toBeCloseTo(0.75, 5);
    expect(p.bloomIntensity).toBeCloseTo(0.7 * 0.35, 5);
    expect(p.ringExpand).toBeCloseTo(0.8, 5);
  });

  it('boot=1 满功率：与默认值一致', () => {
    const def = hudParamsFor('listening', 0, 0);
    const full = hudParamsFor('listening', 0, 0, 1);
    expect(full.coreScale).toBeCloseTo(def.coreScale, 5);
    expect(full.bloomIntensity).toBeCloseTo(def.bloomIntensity, 5);
    expect(full.ringExpand).toBeCloseTo(def.ringExpand, 5);
  });

  it('boot=0.5 线性过渡（核心 0.875×、bloom 67.5%、环 0.9×）', () => {
    const p = hudParamsFor('idle', 0, 0, 0.5);
    expect(p.coreScale).toBeCloseTo(0.875, 5);
    expect(p.bloomIntensity).toBeCloseTo(0.7 * 0.675, 5);
    expect(p.ringExpand).toBeCloseTo(0.9, 5);
  });

  it('boot 越界被钳制到 [0,1]', () => {
    expect(hudParamsFor('idle', 0, 0, -1).coreScale).toBeCloseTo(0.75, 5);
    expect(hudParamsFor('idle', 0, 0, 2).coreScale).toBeCloseTo(1.0, 5);
  });
});

describe('hudParamsFor — 刻度环逐层展开 ringBoot（V5-T3）', () => {
  it('boot=0 待机：三环全部未点亮 [0,0,0]', () => {
    expect(hudParamsFor('idle', 0, 0, 0).ringBoot).toEqual([0, 0, 0]);
  });

  it('boot=1 满功率：三环全部点亮 [1,1,1]', () => {
    expect(hudParamsFor('listening', 0, 0, 1).ringBoot).toEqual([1, 1, 1]);
  });

  it('逐层交错：内环先亮（0→0.5），中环 0.25→0.75，外环 0.5→1.0', () => {
    // boot=0.25：内环半程，中外环未动
    expect(hudParamsFor('idle', 0, 0, 0.25).ringBoot).toEqual([0.5, 0, 0]);
    // boot=0.5：内环满，中环半程，外环未动
    expect(hudParamsFor('idle', 0, 0, 0.5).ringBoot).toEqual([1, 0.5, 0]);
    // boot=0.75：内中环满，外环半程
    expect(hudParamsFor('idle', 0, 0, 0.75).ringBoot).toEqual([1, 1, 0.5]);
  });

  it('boot 越界时 ringBoot 仍钳制在 [0,1]', () => {
    expect(hudParamsFor('idle', 0, 0, -1).ringBoot).toEqual([0, 0, 0]);
    expect(hudParamsFor('idle', 0, 0, 2).ringBoot).toEqual([1, 1, 1]);
  });
});

describe('hudParamsFor — 颜色 tint', () => {
  it('idle/listening/thinking/speaking = 青蓝系渐变（cyan → green）', () => {
    // V5.1：thinking 不再变琥珀（用户反馈黄色突兀），仅靠转速/收缩/波动区分
    for (const s of ['idle', 'listening', 'thinking', 'speaking'] as const) {
      const p = hudParamsFor(s, 0, 0);
      expect(p.tintA).toBe(TINT_CYAN);
      expect(p.tintB).toBe(TINT_GREEN);
    }
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

describe('hudParamsFor — 能量粒子与震动（V5.1）', () => {
  it('粒子增益：speaking 最强（1），idle 最弱，interrupted/error 熄灭', () => {
    expect(hudParamsFor('speaking', 0, 0).particleGain).toBe(1);
    expect(hudParamsFor('idle', 0, 0).particleGain).toBeCloseTo(0.15, 5);
    expect(hudParamsFor('listening', 0, 0).particleGain).toBeCloseTo(0.4, 5);
    expect(hudParamsFor('thinking', 0, 0).particleGain).toBeCloseTo(0.55, 5);
    expect(hudParamsFor('interrupted', 0, 0).particleGain).toBe(0);
    expect(hudParamsFor('error', 0, 0).particleGain).toBe(0);
  });

  it('震动增益：仅 speaking 随振幅震动，其余状态无震动', () => {
    expect(hudParamsFor('speaking', 0, 0.8).shakeGain).toBe(1);
    for (const s of ['idle', 'listening', 'thinking', 'interrupted', 'error'] as const) {
      expect(hudParamsFor(s, 0, 0.8).shakeGain).toBe(0);
    }
  });
});
