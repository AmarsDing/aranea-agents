/**
 * 深空星野背景（M74 设计 §7.4 v2）：~800 粒子球壳分布，缓慢漂移。
 */

import * as THREE from 'three';

import type { HudAudioFrame, HudPart } from './HudPart';
import type { HudParams } from '../hudParams';

const STAR_COUNT = 800;
const SHELL_INNER = 3.2;
const SHELL_OUTER = 9;

export class Starfield implements HudPart {
  private readonly points: THREE.Points;
  private readonly material: THREE.PointsMaterial;

  constructor(parent: THREE.Scene) {
    const positions = new Float32Array(STAR_COUNT * 3);
    for (let i = 0; i < STAR_COUNT; i += 1) {
      // 均匀球面方向 × 壳层随机半径
      const u = Math.random() * 2 - 1;
      const theta = Math.random() * Math.PI * 2;
      const ring = Math.sqrt(1 - u * u);
      const r = SHELL_INNER + Math.random() * (SHELL_OUTER - SHELL_INNER);
      positions[i * 3] = ring * Math.cos(theta) * r;
      positions[i * 3 + 1] = ring * Math.sin(theta) * r;
      positions[i * 3 + 2] = u * r;
    }
    const geometry = new THREE.BufferGeometry();
    geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));

    this.material = new THREE.PointsMaterial({
      color: '#7dd3fc',
      size: 0.035,
      transparent: true,
      opacity: 0.5,
      blending: THREE.AdditiveBlending,
      depthWrite: false,
      sizeAttenuation: true,
    });
    this.points = new THREE.Points(geometry, this.material);
    parent.add(this.points);
  }

  update(dt: number, _timeS: number, params: HudParams, _audio: HudAudioFrame): void {
    this.points.rotation.y += dt * 0.008;
    this.points.rotation.x += dt * 0.003;
    // 待机时星野也随之黯淡，保持整体氛围一致
    this.material.opacity = 0.2 + 0.3 * Math.min(1, params.bloomIntensity);
  }

  setTint(_a: THREE.Color, _b: THREE.Color): void {
    // 星野保持中性青白，不随状态染色
  }

  dispose(): void {
    this.points.geometry.dispose();
    this.material.dispose();
    this.points.removeFromParent();
  }
}
