/**
 * 能量粒子群（M74 设计 §7.4 v2，V5.1 新增）：~220 粒子环绕能量核的轨道云。
 *
 * speaking 满功率：轨道加速、振幅把粒子外推、尺寸/亮度随播报能量脉动；
 * listening/thinking 维系发光，idle 微光漂浮，interrupted/error 熄灭。
 * 帧间零分配：全部粒子状态预分配，update 仅原地写 position 属性。
 */

import * as THREE from 'three';

import type { HudAudioFrame, HudPart } from './HudPart';
import type { HudParams } from '../hudParams';

const PARTICLE_COUNT = 220;
const SHELL_INNER = 0.78;
const SHELL_OUTER = 1.55;

export class EnergyParticles implements HudPart {
  private readonly points: THREE.Points;
  private readonly material: THREE.PointsMaterial;
  private readonly positions: Float32Array;
  private readonly colors: Float32Array;
  /** 粒子静止轨道参数（y 轴轨道）：高度分量 / 初始相位 / 轨道速度 / 径向波动相位 / 基础半径。 */
  private readonly us = new Float32Array(PARTICLE_COUNT);
  private readonly theta0 = new Float32Array(PARTICLE_COUNT);
  private readonly speeds = new Float32Array(PARTICLE_COUNT);
  private readonly phases = new Float32Array(PARTICLE_COUNT);
  private readonly baseRadii = new Float32Array(PARTICLE_COUNT);
  private spin = 0;
  private gainSmoothed = 0;

  constructor(parent: THREE.Scene) {
    this.positions = new Float32Array(PARTICLE_COUNT * 3);
    this.colors = new Float32Array(PARTICLE_COUNT * 3);
    for (let i = 0; i < PARTICLE_COUNT; i += 1) {
      this.us[i] = Math.random() * 2 - 1;
      this.theta0[i] = Math.random() * Math.PI * 2;
      this.speeds[i] = 0.4 + Math.random() * 0.8;
      this.phases[i] = Math.random() * Math.PI * 2;
      this.baseRadii[i] = SHELL_INNER + Math.random() * (SHELL_OUTER - SHELL_INNER);
      this.colors[i * 3] = 1;
      this.colors[i * 3 + 1] = 1;
      this.colors[i * 3 + 2] = 1;
    }
    const geometry = new THREE.BufferGeometry();
    geometry.setAttribute('position', new THREE.BufferAttribute(this.positions, 3));
    geometry.setAttribute('color', new THREE.BufferAttribute(this.colors, 3));

    this.material = new THREE.PointsMaterial({
      size: 0.04,
      vertexColors: true,
      transparent: true,
      opacity: 0,
      blending: THREE.AdditiveBlending,
      depthWrite: false,
      sizeAttenuation: true,
    });
    this.points = new THREE.Points(geometry, this.material);
    this.points.visible = false;
    parent.add(this.points);
  }

  update(dt: number, timeS: number, params: HudParams, audio: HudAudioFrame): void {
    // 增益平滑：状态切换（如 thinking→speaking）粒子渐亮而非突现
    this.gainSmoothed += (params.particleGain - this.gainSmoothed) * Math.min(1, dt * 8);
    const g = this.gainSmoothed;
    this.points.visible = g > 0.02;
    if (!this.points.visible) return;

    this.spin += dt * (0.25 + g * 1.1 + audio.level * 0.9 * g);
    const push = audio.level * 0.3 * g;
    for (let i = 0; i < PARTICLE_COUNT; i += 1) {
      const theta = this.theta0[i] + this.spin * this.speeds[i];
      const u = this.us[i];
      const ring = Math.sqrt(1 - u * u);
      const r =
        this.baseRadii[i] +
        Math.sin(timeS * 2.1 + this.phases[i]) * 0.06 +
        push * (0.5 + 0.5 * Math.sin(this.phases[i] * 3));
      this.positions[i * 3] = ring * Math.cos(theta) * r;
      this.positions[i * 3 + 1] = u * r;
      this.positions[i * 3 + 2] = ring * Math.sin(theta) * r;
    }
    this.points.geometry.attributes.position.needsUpdate = true;

    this.material.opacity = 0.15 + 0.85 * g;
    this.material.size = 0.028 + 0.05 * g + 0.05 * audio.level * g;
  }

  setTint(a: THREE.Color, b: THREE.Color): void {
    // 内层染主色、外层染副色，按基础半径插值（仅着色变化时重算）
    for (let i = 0; i < PARTICLE_COUNT; i += 1) {
      const t = (this.baseRadii[i] - SHELL_INNER) / (SHELL_OUTER - SHELL_INNER);
      this.colors[i * 3] = a.r + (b.r - a.r) * t;
      this.colors[i * 3 + 1] = a.g + (b.g - a.g) * t;
      this.colors[i * 3 + 2] = a.b + (b.b - a.b) * t;
    }
    this.points.geometry.attributes.color.needsUpdate = true;
  }

  dispose(): void {
    this.points.geometry.dispose();
    this.material.dispose();
    this.points.removeFromParent();
  }
}
