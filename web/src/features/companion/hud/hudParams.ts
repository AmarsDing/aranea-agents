/**
 * HUD 状态参数纯函数（M74 设计 §7.4 v2，V5 反应堆科幻重构）。
 *
 * 反应堆场景的全部状态驱动参数在此计算，与 Three.js 解耦以便单测。
 * HudScene 每帧调用 `hudParamsFor(state, timeS, amplitude, boot)` 取参应用。
 */

import type { VoiceState } from '../types';

export const TINT_CYAN = '#22d3ee';
export const TINT_GREEN = '#34d399';
export const TINT_RED = '#f87171';

/** idle 呼吸频率 0.5Hz（周期 2s），幅度 ±4%。 */
const BREATH_OMEGA = Math.PI; // 2π × 0.5Hz
const BREATH_AMPLITUDE = 0.04;

const THINKING_CORE_SCALE = 0.85;
const SPEAKING_SCALE_GAIN = 0.15; // 1.0 → 1.15
const LISTENING_RING_EXPAND = 1.2;
const THINKING_RING_SPEED = 3;

/** speaking Bloom 振幅增益（基础值之上叠加 clamp(振幅)×增益）。 */
const SPEAKING_BLOOM_GAIN = 0.4;

/** 待机（boot=0）衰减系数：核心 0.75×、Bloom 0.35×、刻度环 0.8×，boot=1 恢复全额。 */
const BOOT_CORE_BASE = 0.75;
const BOOT_BLOOM_BASE = 0.35;
const BOOT_RING_BASE = 0.8;

/** 刻度环逐层点亮（V5-T3）：环 i 从 boot=i×0.25 开始，经 0.5 进度铺满。 */
const RING_BOOT_STAGGER = 0.25;
const RING_BOOT_SPAN = 0.5;

export type HudParams = {
  /** 能量核缩放。 */
  coreScale: number;
  /** 刻度环展开倍数（listening 1.2×，受 boot 收缩）。 */
  ringExpand: number;
  /** 刻度环逐层点亮进度 [0,1]×3（V5-T3：内→中→外交错展开）。 */
  ringBoot: [number, number, number];
  /** 刻度环转速因子。 */
  ringSpeedFactor: number;
  /** 频谱环可见性（仅 listening）。 */
  spectrumVisible: boolean;
  /** Bloom 辉光强度（UnrealBloomPass.strength）。 */
  bloomIntensity: number;
  /** 渐变主色。 */
  tintA: string;
  /** 渐变副色（单色系时与 tintA 相同）。 */
  tintB: string;
  /** 能量核顶点波动幅度。 */
  coreWobble: number;
  /** 声浪涟漪增益 [0,1]（0 = 不发射）。 */
  rippleGain: number;
  /** 能量粒子增益 [0,1]（0 = 粒子熄灭；speaking 满功率发射）。 */
  particleGain: number;
  /** 震动增益 [0,1]（仅 speaking，场景侧按振幅抖动相机/核心）。 */
  shakeGain: number;
};

export function clampAmplitude(v: number): number {
  if (Number.isNaN(v)) return 0;
  return Math.min(1, Math.max(0, v));
}

/** 各状态 Bloom 基础强度（V5-T1：speaking 另加振幅增益）。 */
const BLOOM_INTENSITY: Record<VoiceState, number> = {
  idle: 0.7,
  listening: 1.0,
  thinking: 0.9,
  speaking: 1.1,
  interrupted: 1.3,
  error: 0.8,
};

/** 能量核顶点波动幅度：思考最剧，idle 最缓（listening/speaking 由场景侧叠加振幅）。 */
const CORE_WOBBLE: Record<VoiceState, number> = {
  idle: 0.03,
  listening: 0.05,
  thinking: 0.09,
  speaking: 0.06,
  interrupted: 0.02,
  error: 0.02,
};

/** 声浪涟漪增益：仅播报/聆听发射。 */
const RIPPLE_GAIN: Record<VoiceState, number> = {
  idle: 0,
  listening: 0.5,
  thinking: 0,
  speaking: 1,
  interrupted: 0,
  error: 0,
};

/** 能量粒子增益（V5.1）：speaking 满功率，listening/thinking 维系，idle 微光。 */
const PARTICLE_GAIN: Record<VoiceState, number> = {
  idle: 0.15,
  listening: 0.4,
  thinking: 0.55,
  speaking: 1,
  interrupted: 0,
  error: 0,
};

/** 震动增益（V5.1）：仅 speaking 随播报振幅抖动。 */
const SHAKE_GAIN: Record<VoiceState, number> = {
  idle: 0,
  listening: 0,
  thinking: 0,
  speaking: 1,
  interrupted: 0,
  error: 0,
};

/**
 * @param state 当前语音状态（服务端镜像，同 VoiceState）
 * @param timeS 动画累计时间（秒），驱动呼吸等周期动画
 * @param amplitude 播放侧实时振幅 [0,1]（speaking 能量核脉动 / Bloom 增益）
 * @param boot 启动过场进度 [0,1]（V5-T3：0=待机熄灭，1=满功率；场景侧 1.2s 推进）
 */
export function hudParamsFor(state: VoiceState, timeS: number, amplitude: number, boot = 1): HudParams {
  const bootClamped = clampAmplitude(boot);
  const breath = 1 + BREATH_AMPLITUDE * Math.sin(BREATH_OMEGA * timeS);

  let coreScale: number;
  switch (state) {
    case 'thinking':
      coreScale = THINKING_CORE_SCALE;
      break;
    case 'speaking':
      coreScale = 1 + SPEAKING_SCALE_GAIN * clampAmplitude(amplitude);
      break;
    case 'idle':
    case 'listening':
      coreScale = breath;
      break;
    default:
      coreScale = 1;
      break;
  }

  // V5.1：仅 interrupted/error 变红警示；thinking 保持青蓝（用户反馈琥珀刺眼），
  // 靠转速 ×3 / 核心收缩 / 高波动区分。
  const coolTint = state !== 'interrupted' && state !== 'error';

  let bloomIntensity = BLOOM_INTENSITY[state];
  if (state === 'speaking') {
    bloomIntensity += SPEAKING_BLOOM_GAIN * clampAmplitude(amplitude);
  }

  const stateRingExpand = state === 'listening' ? LISTENING_RING_EXPAND : 1;

  return {
    coreScale: coreScale * (BOOT_CORE_BASE + (1 - BOOT_CORE_BASE) * bootClamped),
    ringExpand: stateRingExpand * (BOOT_RING_BASE + (1 - BOOT_RING_BASE) * bootClamped),
    ringBoot: [
      clampAmplitude((bootClamped - 0 * RING_BOOT_STAGGER) / RING_BOOT_SPAN),
      clampAmplitude((bootClamped - 1 * RING_BOOT_STAGGER) / RING_BOOT_SPAN),
      clampAmplitude((bootClamped - 2 * RING_BOOT_STAGGER) / RING_BOOT_SPAN),
    ],
    ringSpeedFactor: state === 'thinking' ? THINKING_RING_SPEED : 1,
    spectrumVisible: state === 'listening',
    bloomIntensity: bloomIntensity * (BOOT_BLOOM_BASE + (1 - BOOT_BLOOM_BASE) * bootClamped),
    tintA: coolTint ? TINT_CYAN : TINT_RED,
    tintB: coolTint ? TINT_GREEN : TINT_RED,
    coreWobble: CORE_WOBBLE[state],
    rippleGain: RIPPLE_GAIN[state],
    particleGain: PARTICLE_GAIN[state],
    shakeGain: SHAKE_GAIN[state],
  };
}
