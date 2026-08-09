/**
 * HUD 场景部件统一接口（M74 设计 §7.4 v3，V7 TwinSprite 光球复刻）。
 *
 * HudScene 组合器每帧采集状态参数与音频快照，分发给各部件；
 * 部件内部自行做平滑/衰减，帧间不分配新对象。
 */

import type * as THREE from 'three';

import type { HudParams } from '../hudParams';

/** 每帧音频快照（组合器统一采集后分发）。 */
export type HudAudioFrame = {
  /** 平滑电平 [0,1]（speaking 取播放侧振幅，listening 取麦克风侧电平）。 */
  level: number;
};

export interface HudPart {
  update(dt: number, timeS: number, params: HudParams, audio: HudAudioFrame): void;
  setTint(a: THREE.Color, b: THREE.Color): void;
  dispose(): void;
}
