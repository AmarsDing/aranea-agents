/**
 * HUD 状态参数纯函数（M74 设计 §7.4 v3，V7 TwinSprite 光球复刻）。
 *
 * 光球场景的全部状态驱动参数在此计算，与 Three.js 解耦以便单测。
 * HudScene 每帧调用 `hudParamsFor(state, level, boot)` 取参应用。
 *
 * v3 说明：视觉完全复刻 TwinSprite SpriteOrb——uAmp（0.12+level×0.38）、
 * 粒子环转速/缩放、配色 #123a6e→#4dd8e8 均为 TwinSprite 原值；状态差异仅
 * 映射为噪声流速/环转速/核心收缩/颜色/震动（设计 §7.4 v3 状态映射表）。
 */

import type { VoiceState } from '../types';

/** TwinSprite 光球配色（SpriteOrb.vue uColorA/uColorB 原值）。 */
export const TINT_ORB_A = '#123a6e';
export const TINT_ORB_B = '#4dd8e8';
/** 警示红系（仅 interrupted/error，V5.1 约束沿用）。 */
export const TINT_RED_A = '#4c0519';
export const TINT_RED_B = '#f87171';

/** TwinSprite uAmp 公式常量：uAmp = AMP_BASE + level × AMP_GAIN。 */
const AMP_BASE = 0.12;
const AMP_GAIN = 0.38;

/** thinking 差异化：噪声流速 ×3、环转速 ×3、核心收缩 0.85×（无颜色变化约束）。 */
const THINKING_SPEED = 3;
const THINKING_ORB_SCALE = 0.85;

/** 待机（boot=0）强度系数：0.35 微光 → boot=1 满功率。 */
const BOOT_INTENSITY_BASE = 0.35;

export type HudParams = {
  /** 顶点噪声振幅基值（uAmp = ampBase + level × ampGain）。 */
  ampBase: number;
  /** 顶点噪声振幅电平增益。 */
  ampGain: number;
  /** 噪声时间流速因子（thinking ×3，其余 ×1）。 */
  noiseSpeedFactor: number;
  /** 光球整体缩放（thinking 收缩 0.85×）。 */
  orbScale: number;
  /** 粒子环转速因子（thinking ×3，其余 ×1）。 */
  ringSpeedFactor: number;
  /** 渐变主色（深蓝侧）。 */
  tintA: string;
  /** 渐变副色（Fresnel 高光侧）。 */
  tintB: string;
  /** 点亮强度 [0.35,1]（boot 过场：待机微光 → 满功率）。 */
  intensity: number;
  /** 震动增益 [0,1]（仅 speaking，场景侧按电平抖动相机）。 */
  shakeGain: number;
};

export function clampAmplitude(v: number): number {
  if (Number.isNaN(v)) return 0;
  return Math.min(1, Math.max(0, v));
}

/**
 * @param state 当前语音状态（服务端镜像，同 VoiceState）
 * @param _amplitude 实时电平 [0,1]（保留入参位；电平在场景侧与 uAmp 公式组合，
 *                   纯函数仅暴露基值/增益常量，便于测试锁定 TwinSprite 原值）
 * @param boot 启动过场进度 [0,1]（0=待机微光，1=满功率；场景侧 1.2s 推进）
 */
export function hudParamsFor(state: VoiceState, _amplitude: number, boot = 1): HudParams {
  const bootClamped = clampAmplitude(boot);
  const thinking = state === 'thinking';
  const alert = state === 'interrupted' || state === 'error';

  return {
    ampBase: AMP_BASE,
    ampGain: AMP_GAIN,
    noiseSpeedFactor: thinking ? THINKING_SPEED : 1,
    orbScale: thinking ? THINKING_ORB_SCALE : 1,
    ringSpeedFactor: thinking ? THINKING_SPEED : 1,
    tintA: alert ? TINT_RED_A : TINT_ORB_A,
    tintB: alert ? TINT_RED_B : TINT_ORB_B,
    intensity: BOOT_INTENSITY_BASE + (1 - BOOT_INTENSITY_BASE) * bootClamped,
    shakeGain: state === 'speaking' ? 1 : 0,
  };
}
