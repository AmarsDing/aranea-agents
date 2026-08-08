/**
 * HUD 状态参数纯函数（M74 设计 §7.4）。
 *
 * 三态场景的全部状态驱动参数在此计算，与 Three.js 解耦以便单测。
 * HudScene 每帧调用 `hudParamsFor(state, timeS, amplitude)` 取参应用。
 */

import type { VoiceState } from '../types';

export const TINT_CYAN = '#22d3ee';
export const TINT_GREEN = '#34d399';
export const TINT_AMBER = '#fbbf24';
export const TINT_RED = '#f87171';

/** idle 呼吸频率 0.5Hz（周期 2s），幅度 ±4%。 */
const BREATH_OMEGA = Math.PI; // 2π × 0.5Hz
const BREATH_AMPLITUDE = 0.04;

const THINKING_CORE_SCALE = 0.85;
const SPEAKING_SCALE_GAIN = 0.15; // 1.0 → 1.15
const LISTENING_OUTER_RING_SCALE = 1.2;
const THINKING_RING_SPEED = 3;

export type HudParams = {
  /** 能量核缩放。 */
  coreScale: number;
  /** 外粒子环展开倍数。 */
  outerRingScale: number;
  /** 粒子环转速因子。 */
  ringSpeedFactor: number;
  /** 频谱环可见性（仅 listening）。 */
  spectrumVisible: boolean;
  /** 渐变主色。 */
  tintA: string;
  /** 渐变副色（单色系时与 tintA 相同）。 */
  tintB: string;
  /** 声波震动增益 [0,1]（粒子环随振幅径向震动；V2-T5 HUD 增强）。 */
  vibrationGain: number;
  /** 全息弧线转速因子（Jarvis 弧线组）。 */
  arcSpeedFactor: number;
  /** 能量核顶点波动幅度。 */
  coreWobble: number;
  /** 声浪涟漪增益 [0,1]（0 = 不发射）。 */
  rippleGain: number;
};

export function clampAmplitude(v: number): number {
  if (Number.isNaN(v)) return 0;
  return Math.min(1, Math.max(0, v));
}

/** 声波震动增益：聆听/播报全开，思考微抖动，其余静止。 */
const VIBRATION_GAIN: Record<VoiceState, number> = {
  idle: 0,
  listening: 1,
  thinking: 0.25,
  speaking: 1,
  interrupted: 0,
  error: 0,
};

/** 全息弧线转速因子：思考最快，中断/错误近停滞。 */
const ARC_SPEED_FACTOR: Record<VoiceState, number> = {
  idle: 1,
  listening: 1.6,
  thinking: 2.8,
  speaking: 1.4,
  interrupted: 0.4,
  error: 0.4,
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

/**
 * @param state 当前语音状态（服务端镜像，同 VoiceState）
 * @param timeS 动画累计时间（秒），驱动呼吸等周期动画
 * @param amplitude 播放侧实时振幅 [0,1]（speaking 能量核脉动）
 */
export function hudParamsFor(state: VoiceState, timeS: number, amplitude: number): HudParams {
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

  const coolTint = state !== 'thinking' && state !== 'interrupted' && state !== 'error';

  return {
    coreScale,
    outerRingScale: state === 'listening' ? LISTENING_OUTER_RING_SCALE : 1,
    ringSpeedFactor: state === 'thinking' ? THINKING_RING_SPEED : 1,
    spectrumVisible: state === 'listening',
    tintA: coolTint ? TINT_CYAN : state === 'thinking' ? TINT_AMBER : TINT_RED,
    tintB: coolTint ? TINT_GREEN : state === 'thinking' ? TINT_AMBER : TINT_RED,
    vibrationGain: VIBRATION_GAIN[state],
    arcSpeedFactor: ARC_SPEED_FACTOR[state],
    coreWobble: CORE_WOBBLE[state],
    rippleGain: RIPPLE_GAIN[state],
  };
}
