/**
 * HUD Three.js 场景（M74 设计 §7.4）。
 *
 * 场景元素：能量核（等离子 shader）/ 粒子环 ×2 / 线框壳 / 频谱环（InstancedMesh）。
 * 每帧从 `hudParamsFor` 取状态参数，颜色经 lerp 平滑过渡；interrupted 红闪 300ms。
 * HUD 不可见（document.hidden）时降帧至 15fps。
 *
 * 性能预算（NFR5）：draw call ≤5（core/shell/ring×2/spectrum instanced），三角形 <5k。
 *
 * 本文件依赖 WebGL，不可单测；状态→参数逻辑见 `hudParams.ts` 纯函数单测。
 */

import * as THREE from 'three';

import type { VoiceState } from '../types';
import { clampAmplitude, hudParamsFor } from './hudParams';

/** 预留 VRM 替换的渲染器接口（设计 §7.2）。 */
export type AvatarRenderer = {
  setState(state: VoiceState): void;
  /** speaking 播放侧实时振幅 [0,1]。 */
  setAmplitude(v: number): void;
  /** listening 采集侧 FFT 频谱（AnalyserNode.getByteFrequencyData）。 */
  setSpectrum(data: Uint8Array | null): void;
  resize(width: number, height: number): void;
  dispose(): void;
};

const SPECTRUM_BARS = 128;
const INNER_RING_COUNT = 600;
const OUTER_RING_COUNT = 900;
/** 不可见时降帧目标（设计 §7.4）。 */
const DEGRADED_FPS = 15;
/** interrupted 红闪时长（秒）。 */
const FLASH_SECONDS = 0.3;

const CORE_VERTEX = /* glsl */ `
varying vec3 vPos;
varying vec3 vNv;
void main() {
  vPos = position;
  vNv = normalize(normalMatrix * normal);
  gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
}
`;

const CORE_FRAGMENT = /* glsl */ `
uniform float uTime;
uniform vec3 uTintA;
uniform vec3 uTintB;
uniform float uIntensity;
varying vec3 vPos;
varying vec3 vNv;
void main() {
  float p = sin(vPos.x * 3.0 + uTime)
          + sin(vPos.y * 4.0 + uTime * 1.3)
          + sin(vPos.z * 5.0 + uTime * 0.7);
  float m = 0.5 + 0.5 * sin(p);
  vec3 base = mix(uTintA, uTintB, m);
  float fres = pow(1.0 - abs(vNv.z), 1.5);
  vec3 col = base * (uIntensity * (0.55 + 0.45 * m)) + base * fres * 0.6;
  gl_FragColor = vec4(col, 1.0);
}
`;

function makeRingPoints(count: number, radius: number): THREE.Points {
  const positions = new Float32Array(count * 3);
  for (let i = 0; i < count; i++) {
    const angle = (i / count) * Math.PI * 2;
    const jitter = (Math.random() - 0.5) * 0.12;
    positions[i * 3] = Math.cos(angle) * (radius + jitter);
    positions[i * 3 + 1] = (Math.random() - 0.5) * 0.08;
    positions[i * 3 + 2] = Math.sin(angle) * (radius + jitter);
  }
  const geo = new THREE.BufferGeometry();
  geo.setAttribute('position', new THREE.BufferAttribute(positions, 3));
  const mat = new THREE.PointsMaterial({
    color: 0xffffff,
    size: 0.035,
    transparent: true,
    opacity: 0.75,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  });
  return new THREE.Points(geo, mat);
}

export function createHudScene(canvas: HTMLCanvasElement): AvatarRenderer {
  const renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));

  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(45, 1, 0.1, 100);
  camera.position.set(0, 0, 6);

  // 能量核
  const coreUniforms = {
    uTime: { value: 0 },
    uTintA: { value: new THREE.Color('#22d3ee') },
    uTintB: { value: new THREE.Color('#34d399') },
    uIntensity: { value: 0.9 },
  };
  const core = new THREE.Mesh(
    new THREE.IcosahedronGeometry(1, 3),
    new THREE.ShaderMaterial({
      uniforms: coreUniforms,
      vertexShader: CORE_VERTEX,
      fragmentShader: CORE_FRAGMENT,
      transparent: true,
      blending: THREE.AdditiveBlending,
      depthWrite: false,
    }),
  );
  scene.add(core);

  // 线框壳（常态缓旋）
  const shell = new THREE.Mesh(
    new THREE.IcosahedronGeometry(1.45, 1),
    new THREE.MeshBasicMaterial({ wireframe: true, transparent: true, opacity: 0.14, color: 0x22d3ee }),
  );
  scene.add(shell);

  // 粒子环 ×2（反向旋转）
  const innerRing = makeRingPoints(INNER_RING_COUNT, 1.9);
  const outerRing = makeRingPoints(OUTER_RING_COUNT, 2.5);
  scene.add(innerRing);
  scene.add(outerRing);

  // 频谱环（128 柱 InstancedMesh，1 次 draw call）
  const spectrum = new THREE.InstancedMesh(
    new THREE.BoxGeometry(0.035, 1, 0.035),
    new THREE.MeshBasicMaterial({ transparent: true, opacity: 0.85, color: 0x22d3ee }),
    SPECTRUM_BARS,
  );
  spectrum.visible = false;
  spectrum.instanceMatrix.setUsage(THREE.DynamicDrawUsage);
  scene.add(spectrum);

  const dummy = new THREE.Object3D();
  const tintA = new THREE.Color('#22d3ee');
  const tintB = new THREE.Color('#34d399');
  const targetA = new THREE.Color();
  const targetB = new THREE.Color();

  let state: VoiceState = 'idle';
  let amplitude = 0;
  let amplitudeTarget = 0;
  let spectrumData: Uint8Array | null = null;
  let flashLeft = 0;
  let timeS = 0;
  let lastTs: number | null = null;
  let rafId = 0;
  let frameAcc = 0;
  let disposed = false;

  function applySpectrum(): void {
    if (!spectrum.visible) return;
    const step =
      spectrumData && spectrumData.length > 0 ? Math.max(1, Math.floor(spectrumData.length / SPECTRUM_BARS)) : 0;
    for (let i = 0; i < SPECTRUM_BARS; i++) {
      const v = spectrumData ? (spectrumData[i * step] ?? 0) / 255 : 0;
      const h = 0.08 + v * 1.1;
      const angle = (i / SPECTRUM_BARS) * Math.PI * 2;
      dummy.position.set(Math.cos(angle) * 2.1, 0, Math.sin(angle) * 2.1);
      dummy.scale.set(1, h, 1);
      dummy.rotation.set(0, -angle, 0);
      dummy.updateMatrix();
      spectrum.setMatrixAt(i, dummy.matrix);
    }
    spectrum.instanceMatrix.needsUpdate = true;
  }

  function frame(ts: number): void {
    if (disposed) return;
    rafId = requestAnimationFrame(frame);
    if (lastTs === null) lastTs = ts;
    const dt = Math.min(0.1, Math.max(0, (ts - lastTs) / 1000));
    lastTs = ts;

    // 不可见降帧：累计间隔不足 1/15s 时跳过渲染
    frameAcc += dt;
    const minInterval = document.hidden ? 1 / DEGRADED_FPS : 0;
    if (frameAcc < minInterval) return;
    const step = frameAcc;
    frameAcc = 0;
    timeS += step;

    amplitude += (clampAmplitude(amplitudeTarget) - amplitude) * Math.min(1, step * 12);
    if (flashLeft > 0) flashLeft = Math.max(0, flashLeft - step);

    const params = hudParamsFor(state, timeS, amplitude);

    // 颜色平滑过渡
    targetA.set(params.tintA);
    targetB.set(params.tintB);
    const lerpK = Math.min(1, step * 8);
    tintA.lerp(targetA, lerpK);
    tintB.lerp(targetB, lerpK);
    coreUniforms.uTintA.value.copy(tintA);
    coreUniforms.uTintB.value.copy(tintB);

    // 能量核：缩放 + 等离子时间 + 发光强度（interrupted 红闪 300ms）
    const flash = flashLeft > 0 ? flashLeft / FLASH_SECONDS : 0;
    core.scale.setScalar(params.coreScale * (1 + flash * 0.3));
    coreUniforms.uTime.value = timeS;
    coreUniforms.uIntensity.value = 0.9 + (state === 'speaking' ? amplitude * 0.4 : 0) + flash * 0.6;

    // 线框壳缓旋
    shell.rotation.y += step * 0.25;
    shell.rotation.x += step * 0.06;
    (shell.material as THREE.MeshBasicMaterial).color.copy(tintA);

    // 粒子环：反向旋转；listening 外环展开；thinking 转速 ×3
    const speed = params.ringSpeedFactor;
    innerRing.rotation.y += step * 0.4 * speed;
    outerRing.rotation.y -= step * 0.25 * speed;
    const outerScale = outerRing.scale.x + (params.outerRingScale - outerRing.scale.x) * lerpK;
    outerRing.scale.setScalar(outerScale);
    (innerRing.material as THREE.PointsMaterial).color.copy(tintA);
    (outerRing.material as THREE.PointsMaterial).color.copy(tintB);

    // 频谱环仅 listening
    spectrum.visible = params.spectrumVisible;
    (spectrum.material as THREE.MeshBasicMaterial).color.copy(tintA);
    applySpectrum();

    renderer.render(scene, camera);
  }

  rafId = requestAnimationFrame(frame);

  return {
    setState(next: VoiceState): void {
      if (next === 'interrupted' && state !== 'interrupted') {
        flashLeft = FLASH_SECONDS;
      }
      state = next;
    },

    setAmplitude(v: number): void {
      amplitudeTarget = v;
    },

    setSpectrum(data: Uint8Array | null): void {
      spectrumData = data;
    },

    resize(width: number, height: number): void {
      if (width <= 0 || height <= 0) return;
      renderer.setSize(width, height, false);
      camera.aspect = width / height;
      camera.updateProjectionMatrix();
    },

    dispose(): void {
      disposed = true;
      cancelAnimationFrame(rafId);
      core.geometry.dispose();
      core.material.dispose();
      shell.geometry.dispose();
      (shell.material as THREE.Material).dispose();
      innerRing.geometry.dispose();
      (innerRing.material as THREE.Material).dispose();
      outerRing.geometry.dispose();
      (outerRing.material as THREE.Material).dispose();
      spectrum.geometry.dispose();
      (spectrum.material as THREE.Material).dispose();
      renderer.dispose();
    },
  };
}
