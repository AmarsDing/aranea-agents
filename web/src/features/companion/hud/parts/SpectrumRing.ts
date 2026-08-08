/**
 * 声浪频谱环（M74 设计 §7.4 v2，自初版迁移）：128 柱 InstancedMesh，
 * listening 态可见，实时麦克风 FFT 驱动，Bloom 下发光。
 */

import * as THREE from 'three';

import type { HudAudioFrame, HudPart } from './HudPart';
import type { HudParams } from '../hudParams';

const BARS = 128;
const RADIUS = 1.15;

export class SpectrumRing implements HudPart {
  private readonly mesh: THREE.InstancedMesh;
  private readonly material: THREE.MeshBasicMaterial;
  private readonly tempMatrix = new THREE.Matrix4();
  private readonly tempQuat = new THREE.Quaternion();
  private readonly tempPos = new THREE.Vector3();
  private readonly tempScale = new THREE.Vector3();
  private readonly zAxis = new THREE.Vector3(0, 0, 1);

  constructor(parent: THREE.Scene) {
    const geometry = new THREE.BoxGeometry(0.02, 1, 0.02);
    this.material = new THREE.MeshBasicMaterial({
      color: '#22d3ee',
      transparent: true,
      opacity: 0.65,
      blending: THREE.AdditiveBlending,
      depthWrite: false,
    });
    this.mesh = new THREE.InstancedMesh(geometry, this.material, BARS);
    this.mesh.visible = false;
    parent.add(this.mesh);
  }

  update(_dt: number, _timeS: number, params: HudParams, audio: HudAudioFrame): void {
    this.mesh.visible = params.spectrumVisible;
    if (!params.spectrumVisible || !audio.spectrum) return;

    const bins = audio.spectrum;
    for (let i = 0; i < BARS; i += 1) {
      const bin = bins[Math.floor((i / BARS) * bins.length * 0.7)] ?? 0;
      const h = 0.06 + (bin / 255) * 0.4;
      const angle = (i / BARS) * Math.PI * 2;
      this.tempPos.set(Math.cos(angle) * RADIUS, Math.sin(angle) * RADIUS, 0);
      this.tempQuat.setFromAxisAngle(this.zAxis, angle);
      this.tempScale.set(1, h, 1);
      this.tempMatrix.compose(this.tempPos, this.tempQuat, this.tempScale);
      this.mesh.setMatrixAt(i, this.tempMatrix);
    }
    this.mesh.instanceMatrix.needsUpdate = true;
  }

  setTint(a: THREE.Color, _b: THREE.Color): void {
    this.material.color.set(a);
  }

  dispose(): void {
    this.mesh.geometry.dispose();
    this.material.dispose();
    this.mesh.removeFromParent();
  }
}
