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
};

export function clampAmplitude(v: number): number {
  if (Number.isNaN(v)) return 0;
  return Math.min(1, Math.max(0, v));
}

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
  };
}
