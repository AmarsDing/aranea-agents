/**
 * 反应堆同心刻度仪表环 ×3（M74 设计 §7.4 v2，Jarvis 标志性元素）。
 *
 * 每环 = 细圆环（Torus）+ InstancedMesh 刻度线段（每 6 格一根长刻度）；
 * 三环异速正反转、分段呼吸透明度；listening 整体展开、thinking 转速 ×3。
 */

import * as THREE from 'three';

import type { HudAudioFrame, HudPart } from './HudPart';
import type { HudParams } from '../hudParams';

type RingSpec = {
  /** 半径。 */
  radius: number;
  /** 基础转速（rad/s，负值反转）。 */
  speed: number;
  /** 刻度数。 */
  ticks: number;
  /** 呼吸相位偏移。 */
  phase: number;
};

const RING_SPECS: RingSpec[] = [
  { radius: 0.78, speed: 0.35, ticks: 48, phase: 0 },
  { radius: 0.92, speed: -0.22, ticks: 72, phase: 2.1 },
  { radius: 1.06, speed: 0.13, ticks: 96, phase: 4.2 },
];

const MINOR_TICK = { w: 0.012, h: 0.045 };
const MAJOR_TICK = { w: 0.016, h: 0.085 };
const MAJOR_EVERY = 6;
const BASE_OPACITY = 0.55;

export class ReactorRings implements HudPart {
  private readonly group = new THREE.Group();
  private readonly rings: THREE.Group[] = [];
  private readonly materials: THREE.MeshBasicMaterial[] = [];
  private expandCurrent = 1;

  constructor(parent: THREE.Scene) {
    for (const spec of RING_SPECS) {
      const ring = this.buildRing(spec);
      this.rings.push(ring);
      this.group.add(ring);
    }
    parent.add(this.group);
  }

  private buildRing(spec: RingSpec): THREE.Group {
    const ring = new THREE.Group();

    const circleMat = new THREE.MeshBasicMaterial({
      color: '#22d3ee',
      transparent: true,
      opacity: 0.28,
      blending: THREE.AdditiveBlending,
      depthWrite: false,
    });
    const circle = new THREE.Mesh(new THREE.TorusGeometry(spec.radius, 0.004, 6, 128), circleMat);
    ring.add(circle);
    this.materials.push(circleMat);

    const tickMat = new THREE.MeshBasicMaterial({
      color: '#22d3ee',
      transparent: true,
      opacity: BASE_OPACITY,
      blending: THREE.AdditiveBlending,
      depthWrite: false,
    });
    const tickGeo = new THREE.BoxGeometry(1, 1, 0.008);
    const ticks = new THREE.InstancedMesh(tickGeo, tickMat, spec.ticks);
    const m = new THREE.Matrix4();
    const q = new THREE.Quaternion();
    const pos = new THREE.Vector3();
    const scl = new THREE.Vector3();
    const zAxis = new THREE.Vector3(0, 0, 1);
    for (let i = 0; i < spec.ticks; i += 1) {
      const angle = (i / spec.ticks) * Math.PI * 2;
      const major = i % MAJOR_EVERY === 0;
      const size = major ? MAJOR_TICK : MINOR_TICK;
      pos.set(Math.cos(angle) * spec.radius, Math.sin(angle) * spec.radius, 0);
      q.setFromAxisAngle(zAxis, angle);
      scl.set(size.w, size.h, 1);
      m.compose(pos, q, scl);
      ticks.setMatrixAt(i, m);
    }
    ticks.instanceMatrix.needsUpdate = true;
    ring.add(ticks);
    this.materials.push(tickMat);

    return ring;
  }

  update(dt: number, timeS: number, params: HudParams, _audio: HudAudioFrame): void {
    // 展开平滑（避免状态切换跳变）
    this.expandCurrent += (params.ringExpand - this.expandCurrent) * Math.min(1, dt * 5);
    this.group.scale.setScalar(this.expandCurrent);

    for (let i = 0; i < RING_SPECS.length; i += 1) {
      const spec = RING_SPECS[i];
      this.rings[i].rotation.z += dt * spec.speed * params.ringSpeedFactor;
      // 分段呼吸透明度
      const breath = 0.7 + 0.3 * Math.sin(timeS * 0.9 + spec.phase);
      // V5-T3 逐层点亮：待机保留 20% 幽影，boot 过场内→中→外逐环展开（含缩放弹入）
      const reveal = params.ringBoot[i] ?? 1;
      const lit = 0.2 + 0.8 * reveal;
      this.materials[i * 2].opacity = 0.28 * breath * lit;
      this.materials[i * 2 + 1].opacity = BASE_OPACITY * breath * lit;
      this.rings[i].scale.setScalar(0.9 + 0.1 * reveal);
    }
  }

  setTint(a: THREE.Color, _b: THREE.Color): void {
    for (const mat of this.materials) {
      mat.color.set(a);
    }
  }

  dispose(): void {
    for (const ring of this.rings) {
      for (const child of ring.children) {
        if (child instanceof THREE.Mesh) {
          child.geometry.dispose();
        }
      }
    }
    for (const mat of this.materials) {
      mat.dispose();
    }
    this.group.removeFromParent();
  }
}
