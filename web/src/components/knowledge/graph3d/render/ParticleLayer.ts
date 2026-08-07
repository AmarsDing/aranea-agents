/**
 * ParticleLayer：G5 粒子流（fast-graph 1:1 复刻，设计 §V12.8-1 C-2/D-6）。
 *
 * - MAX=80 并发上限、SPEED=0.45/s（≈2.2s/边）
 * - hover 节点 → 每邻居一粒子，相位均布 prog[i]=i/n（连续流）
 * - easeInOutQuad 缓动；时变彩虹 hue=0.5+0.32·sin((t·0.6+p·2.2+i·0.12)·π)
 * - PointsMaterial size=8 + 64px 径向渐变 glowTexture + vertexColors + depthWrite:false
 * - 纯数学走 particleMath（可单测）；本层只做 three 缓冲写入
 */
import * as THREE from 'three';
import {
  PARTICLE_MAX,
  advancePhase,
  easeInOutQuad,
  particleHsl,
  particlePosition,
  spreadPhases,
} from '../../../../features/knowledge/graph3d/particleMath';
import { makeRadialTexture } from './BackdropLayer';

const tmpColor = new THREE.Color();

export class ParticleLayer {
  readonly points: THREE.Points;
  private readonly geometry: THREE.BufferGeometry;
  private readonly material: THREE.PointsMaterial;
  private readonly texture: THREE.Texture | null;
  private readonly positions = new Float32Array(PARTICLE_MAX * 3);
  private readonly colors = new Float32Array(PARTICLE_MAX * 3);
  private readonly src = new Int32Array(PARTICLE_MAX);
  private readonly dst = new Int32Array(PARTICLE_MAX);
  private prog = new Float32Array(PARTICLE_MAX);
  private count = 0;
  private time = 0;

  constructor() {
    this.texture = makeRadialTexture([
      [0, 'rgba(255,255,255,1)'],
      [0.25, 'rgba(255,255,255,0.85)'],
      [0.6, 'rgba(255,255,255,0.25)'],
      [1, 'rgba(255,255,255,0)'],
    ]);
    this.geometry = new THREE.BufferGeometry();
    this.geometry.setAttribute('position', new THREE.BufferAttribute(this.positions, 3));
    this.geometry.setAttribute('color', new THREE.BufferAttribute(this.colors, 3));
    this.geometry.setDrawRange(0, 0);
    this.material = new THREE.PointsMaterial({
      size: 8,
      map: this.texture,
      vertexColors: true,
      transparent: true,
      depthWrite: false,
      sizeAttenuation: true,
    });
    this.points = new THREE.Points(this.geometry, this.material);
    this.points.frustumCulled = false;
    this.points.renderOrder = 2;
  }

  /** 有活跃粒子（lazy-render 保持渲染循环的判定条件之一）。 */
  get active(): boolean {
    return this.count > 0;
  }

  /** hover 节点与邻居列表配置粒子流；hovered=null 清空。 */
  setSource(hovered: number | null, neighbors: number[]): void {
    if (hovered === null || neighbors.length === 0) {
      this.count = 0;
      this.geometry.setDrawRange(0, 0);
      return;
    }
    const n = Math.min(neighbors.length, PARTICLE_MAX);
    this.count = n;
    const phases = spreadPhases(n);
    for (let i = 0; i < n; i++) {
      this.src[i] = hovered;
      this.dst[i] = neighbors[i];
      this.prog[i] = phases[i];
    }
    this.geometry.setDrawRange(0, n);
  }

  /** 每帧推进（dt 秒）：位置 ease 插值 + 时变彩虹色。 */
  update(nodePositions: Float32Array, dt: number): void {
    if (this.count === 0) return;
    this.time += dt;
    const tmp = new Float32Array(3);
    for (let i = 0; i < this.count; i++) {
      this.prog[i] = advancePhase(this.prog[i], dt);
      const p = this.prog[i];
      const e = easeInOutQuad(p);
      const s = this.src[i] * 3;
      const d = this.dst[i] * 3;
      particlePosition(
        [nodePositions[s], nodePositions[s + 1], nodePositions[s + 2]],
        [nodePositions[d], nodePositions[d + 1], nodePositions[d + 2]],
        e,
        tmp,
        0,
      );
      this.positions[i * 3] = tmp[0];
      this.positions[i * 3 + 1] = tmp[1];
      this.positions[i * 3 + 2] = tmp[2];
      const { h, s: sat, l } = particleHsl(this.time, p, i);
      tmpColor.setHSL(h, sat, l);
      this.colors[i * 3] = tmpColor.r;
      this.colors[i * 3 + 1] = tmpColor.g;
      this.colors[i * 3 + 2] = tmpColor.b;
    }
    (this.geometry.getAttribute('position') as THREE.BufferAttribute).needsUpdate = true;
    (this.geometry.getAttribute('color') as THREE.BufferAttribute).needsUpdate = true;
  }

  dispose(): void {
    this.geometry.dispose();
    this.material.dispose();
    this.texture?.dispose();
  }
}
