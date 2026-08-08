/**
 * 声浪涟漪池（M74 设计 §7.4 v2，自初版迁移）：复用 4 圈 Mesh 环形扩散波，
 * listening/speaking 周期发射；burst 时立即发射一圈满强度。Bloom 下更亮。
 */

import * as THREE from 'three';

import type { HudAudioFrame, HudPart } from './HudPart';
import type { HudParams } from '../hudParams';

const RIPPLE_COUNT = 4;
const RIPPLE_LIFE = 1.4; // 秒
const RIPPLE_MAX_RADIUS = 1.9;

type RippleSlot = {
  mesh: THREE.Mesh;
  material: THREE.MeshBasicMaterial;
  age: number;
  strength: number;
  active: boolean;
};

export class ShockwavePool implements HudPart {
  private readonly pool: RippleSlot[] = [];
  private emitClock = 0;
  private burstQueued = false;

  constructor(parent: THREE.Scene) {
    const geometry = new THREE.RingGeometry(0.97, 1, 96);
    for (let i = 0; i < RIPPLE_COUNT; i += 1) {
      const material = new THREE.MeshBasicMaterial({
        color: '#22d3ee',
        transparent: true,
        opacity: 0,
        side: THREE.DoubleSide,
        blending: THREE.AdditiveBlending,
        depthWrite: false,
      });
      const mesh = new THREE.Mesh(geometry, material);
      mesh.visible = false;
      parent.add(mesh);
      this.pool.push({ mesh, material, age: 0, strength: 0, active: false });
    }
  }

  /** 确认批准等爆发事件：立即发射一圈满强度涟漪。 */
  queueBurst(): void {
    this.burstQueued = true;
  }

  private fire(strength: number): void {
    const slot = this.pool.find((s) => !s.active);
    if (!slot) return;
    slot.active = true;
    slot.age = 0;
    slot.strength = strength;
  }

  update(dt: number, _timeS: number, params: HudParams, _audio: HudAudioFrame): void {
    if (this.burstQueued) {
      this.fire(1);
      this.burstQueued = false;
    }

    // 周期发射：涟漪增益 > 0 时每 0.9s 一圈
    if (params.rippleGain > 0) {
      this.emitClock += dt;
      if (this.emitClock >= 0.9) {
        this.emitClock = 0;
        this.fire(params.rippleGain * 0.6);
      }
    } else {
      this.emitClock = 0;
    }

    for (const slot of this.pool) {
      if (!slot.active) continue;
      slot.age += dt;
      const t = slot.age / RIPPLE_LIFE;
      if (t >= 1) {
        slot.active = false;
        slot.mesh.visible = false;
        continue;
      }
      slot.mesh.visible = true;
      const ease = 1 - (1 - t) * (1 - t); // easeOutQuad
      slot.mesh.scale.setScalar(0.8 + ease * (RIPPLE_MAX_RADIUS - 0.8));
      slot.material.opacity = slot.strength * (1 - t) * 0.5;
    }
  }

  setTint(a: THREE.Color, _b: THREE.Color): void {
    for (const slot of this.pool) {
      slot.material.color.set(a);
    }
  }

  dispose(): void {
    const geometry = this.pool[0]?.mesh.geometry;
    geometry?.dispose();
    for (const slot of this.pool) {
      slot.material.dispose();
      slot.mesh.removeFromParent();
    }
    this.pool.length = 0;
  }
}
