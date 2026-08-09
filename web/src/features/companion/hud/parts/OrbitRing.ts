/**
 * TwinSprite 轨道粒子环（M74 设计 §7.4 v3，V7 复刻 TwinSprite SpriteOrb buildRing）。
 *
 * 260 粒子单环，半径 1.55~1.80、y 抖动 ±0.06，倾斜 π×0.42；加法混合、
 * 尺寸 0.035、基础透明度 0.85。电平驱动：转速 0.002+level×0.02 rad/帧
 * （dt 归一化为 ×60/s）、xz 缩放 1+level×0.12——均为 TwinSprite 原值。
 */

import * as THREE from 'three';

import type { HudAudioFrame, HudPart } from './HudPart';
import type { HudParams } from '../hudParams';

/** TwinSprite 每帧（60fps）转速公式 → 每秒弧度。 */
const RING_BASE_SPEED = 0.002 * 60;
const RING_LEVEL_SPEED = 0.02 * 60;
const RING_LEVEL_SCALE = 0.12;
const RING_BASE_OPACITY = 0.85;

export class OrbitRing implements HudPart {
  private readonly points: THREE.Points;
  private readonly material: THREE.PointsMaterial;

  constructor(parent: THREE.Scene) {
    const COUNT = 260;
    const pos = new Float32Array(COUNT * 3);
    for (let i = 0; i < COUNT; i++) {
      const a = (i / COUNT) * Math.PI * 2;
      const r = 1.55 + Math.random() * 0.25;
      pos[i * 3] = Math.cos(a) * r;
      pos[i * 3 + 1] = (Math.random() - 0.5) * 0.12;
      pos[i * 3 + 2] = Math.sin(a) * r;
    }
    const geometry = new THREE.BufferGeometry();
    geometry.setAttribute('position', new THREE.BufferAttribute(pos, 3));
    this.material = new THREE.PointsMaterial({
      color: 0x4dd8e8,
      size: 0.035,
      transparent: true,
      opacity: RING_BASE_OPACITY,
      blending: THREE.AdditiveBlending,
      depthWrite: false,
    });
    this.points = new THREE.Points(geometry, this.material);
    this.points.rotation.x = Math.PI * 0.42;
    parent.add(this.points);
  }

  update(dt: number, _timeS: number, params: HudParams, audio: HudAudioFrame): void {
    this.points.rotation.y += (RING_BASE_SPEED + audio.level * RING_LEVEL_SPEED) * params.ringSpeedFactor * dt;
    const s = 1 + audio.level * RING_LEVEL_SCALE;
    this.points.scale.set(s, 1, s);
    this.material.opacity = RING_BASE_OPACITY * params.intensity;
  }

  setTint(_a: THREE.Color, b: THREE.Color): void {
    this.material.color.copy(b);
  }

  dispose(): void {
    this.points.geometry.dispose();
    this.material.dispose();
    this.points.removeFromParent();
  }
}
