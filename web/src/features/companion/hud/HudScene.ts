/**
 * HUD Three.js 场景（M74 设计 §7.4 + V2-T5 科幻增强）。
 *
 * 场景元素：能量核（等离子 shader + 顶点波动）/ 粒子环 ×2（声波震动）/ 线框壳 /
 * 频谱环（InstancedMesh）/ Jarvis 全息弧线 ×3 / 声浪涟漪池 ×4。
 * 每帧从 `hudParamsFor` 取状态参数，颜色经 lerp 平滑过渡；interrupted 红闪 300ms。
 * HUD 不可见（document.hidden）时降帧至 15fps。
 *
 * 性能预算（NFR5 as-built）：draw call ≤12（core/shell/ring×2/spectrum instanced/
 * arc×3/ripple 池×4），三角形 <8k；涟漪池复用、弧线共享几何，无每帧分配。
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
  /** 确认通过等外部触发的一次性能量脉冲（核闪光 + 立刻发射一圈涟漪）。 */
  burst(): void;
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
/** burst 能量脉冲时长（秒）。 */
const BURST_SECONDS = 0.6;
/** 声浪涟漪池大小与发射周期/生命周期（秒）。 */
const RIPPLE_POOL = 4;
const RIPPLE_PERIOD = 0.9;
const RIPPLE_LIFE = 1.3;
/** 粒子尺寸基准（震动时在此基础上放大）。 */
const POINT_SIZE_BASE = 0.035;

const CORE_VERTEX = /* glsl */ `
uniform float uTime;
uniform float uWobble;
varying vec3 vPos;
varying vec3 vNv;
void main() {
  vPos = position;
  vNv = normalize(normalMatrix * normal);
  float w = sin(position.x * 5.0 + uTime * 2.0)
          + sin(position.y * 6.0 + uTime * 1.7)
          + sin(position.z * 4.0 + uTime * 2.3);
  vec3 p = position + normal * (uWobble * w / 3.0);
  gl_Position = projectionMatrix * modelViewMatrix * vec4(p, 1.0);
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
    size: POINT_SIZE_BASE,
    transparent: true,
    opacity: 0.75,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  });
  return new THREE.Points(geo, mat);
}

/** Jarvis 全息弧线：XY 平面上的开口圆弧（Line），转速/方向由调用侧控制。 */
function makeArc(radius: number, thetaLength: number): THREE.Line {
  const points: THREE.Vector3[] = [];
  const segments = 48;
  for (let i = 0; i <= segments; i++) {
    const a = (i / segments) * thetaLength;
    points.push(new THREE.Vector3(Math.cos(a) * radius, Math.sin(a) * radius, 0));
  }
  const geo = new THREE.BufferGeometry().setFromPoints(points);
  const mat = new THREE.LineBasicMaterial({
    color: 0xffffff,
    transparent: true,
    opacity: 0.5,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  });
  return new THREE.Line(geo, mat);
}

/** 声浪涟漪：面向相机的细圆环（RingGeometry 复用，池化发射）。 */
function makeRipple(): THREE.Mesh {
  const geo = new THREE.RingGeometry(0.96, 1.0, 96);
  const mat = new THREE.MeshBasicMaterial({
    color: 0xffffff,
    transparent: true,
    opacity: 0,
    depthWrite: false,
    side: THREE.DoubleSide,
    blending: THREE.AdditiveBlending,
  });
  const mesh = new THREE.Mesh(geo, mat);
  mesh.visible = false;
  return mesh;
}

/** 弧线组配置：半径/弧长/倾角/基准转速（正负 = 方向）。 */
const ARC_SPECS: ReadonlyArray<{ radius: number; theta: number; tiltX: number; tiltY: number; speed: number }> = [
  { radius: 1.62, theta: Math.PI * 0.9, tiltX: 0.35, tiltY: 0.1, speed: 0.7 },
  { radius: 2.05, theta: Math.PI * 0.55, tiltX: -0.2, tiltY: 0.3, speed: -0.5 },
  { radius: 2.85, theta: Math.PI * 1.2, tiltX: 0.15, tiltY: -0.25, speed: 0.35 },
];

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
    uWobble: { value: 0.03 },
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

  // Jarvis 全息弧线组（V2-T5）：不同半径/弧长/倾角/转向
  const arcs = ARC_SPECS.map((spec) => {
    const arc = makeArc(spec.radius, spec.theta);
    arc.rotation.x = spec.tiltX;
    arc.rotation.y = spec.tiltY;
    scene.add(arc);
    return arc;
  });

  // 声浪涟漪池（V2-T5）：复用 4 个圆环，避免每帧分配
  const ripples: Array<{ mesh: THREE.Mesh; age: number; strength: number }> = [];
  for (let i = 0; i < RIPPLE_POOL; i++) {
    const mesh = makeRipple();
    scene.add(mesh);
    ripples.push({ mesh, age: -1, strength: 0 }); // age<0 = 空闲
  }
  let rippleTimer = 0;
  let burstLeft = 0;

  const dummy = new THREE.Object3D();
  const tintA = new THREE.Color('#22d3ee');
  const tintB = new THREE.Color('#34d399');
  const targetA = new THREE.Color();
  const targetB = new THREE.Color();

  let state: VoiceState = 'idle';
  let amplitude = 0;
  let amplitudeTarget = 0;
  let spectrumData: Uint8Array | null = null;
  let spectrumLevel = 0; // listening 采集侧平均电平 [0,1]（声波震动驱动源之一）
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
    let sum = 0;
    for (let i = 0; i < SPECTRUM_BARS; i++) {
      const v = spectrumData ? (spectrumData[i * step] ?? 0) / 255 : 0;
      sum += v;
      const h = 0.08 + v * 1.1;
      const angle = (i / SPECTRUM_BARS) * Math.PI * 2;
      dummy.position.set(Math.cos(angle) * 2.1, 0, Math.sin(angle) * 2.1);
      dummy.scale.set(1, h, 1);
      dummy.rotation.set(0, -angle, 0);
      dummy.updateMatrix();
      spectrum.setMatrixAt(i, dummy.matrix);
    }
    spectrum.instanceMatrix.needsUpdate = true;
    spectrumLevel = spectrumData ? sum / SPECTRUM_BARS : 0;
  }

  /** 发射一圈声浪涟漪（取空闲槽位；无空闲时抢占最老的一圈）。 */
  function emitRipple(strength: number): void {
    let slot = ripples.find((r) => r.age < 0);
    if (!slot) {
      slot = ripples[0];
      for (const r of ripples) {
        if (r.age > slot.age) slot = r;
      }
    }
    slot.age = 0;
    slot.strength = strength;
    slot.mesh.visible = true;
  }

  function updateRipples(step: number, gain: number, audioLevel: number): void {
    // 发射节拍：增益驱动；listening/speaking 且有声时更密
    if (gain > 0) {
      rippleTimer -= step * (0.6 + audioLevel);
      if (rippleTimer <= 0) {
        rippleTimer = RIPPLE_PERIOD;
        emitRipple(gain * (0.5 + 0.5 * audioLevel));
      }
    }
    for (const r of ripples) {
      if (r.age < 0) continue;
      r.age += step;
      const k = r.age / RIPPLE_LIFE;
      if (k >= 1) {
        r.age = -1;
        r.mesh.visible = false;
        continue;
      }
      const ease = 1 - (1 - k) * (1 - k); // easeOutQuad
      const radius = (1.15 + (3.4 - 1.15) * ease) * (0.7 + 0.5 * r.strength);
      r.mesh.scale.setScalar(radius);
      (r.mesh.material as THREE.MeshBasicMaterial).opacity = (1 - k) * (1 - k) * 0.55 * r.strength;
    }
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
    if (burstLeft > 0) burstLeft = Math.max(0, burstLeft - step);

    const params = hudParamsFor(state, timeS, amplitude);

    // 音频驱动电平：speaking 用播放振幅，listening 用采集频谱均值
    const audioLevel = state === 'speaking' ? amplitude : state === 'listening' ? spectrumLevel : 0;
    // 声波震动：增益 ×（基础微抖 0.15 + 音频 0.85），thinking 亦有微抖
    const vib = params.vibrationGain * (0.15 + 0.85 * audioLevel);

    // 颜色平滑过渡
    targetA.set(params.tintA);
    targetB.set(params.tintB);
    const lerpK = Math.min(1, step * 8);
    tintA.lerp(targetA, lerpK);
    tintB.lerp(targetB, lerpK);
    coreUniforms.uTintA.value.copy(tintA);
    coreUniforms.uTintB.value.copy(tintB);

    // 能量核：缩放 + 顶点波动 + 等离子时间 + 发光强度（interrupted 红闪 300ms，burst 脉冲）
    const flash = flashLeft > 0 ? flashLeft / FLASH_SECONDS : 0;
    const burstK = burstLeft > 0 ? burstLeft / BURST_SECONDS : 0;
    core.scale.setScalar(params.coreScale * (1 + flash * 0.3 + burstK * 0.25));
    coreUniforms.uTime.value = timeS;
    coreUniforms.uWobble.value = params.coreWobble * (1 + 2 * audioLevel) + burstK * 0.08;
    coreUniforms.uIntensity.value =
      0.9 + (state === 'speaking' ? amplitude * 0.4 : 0) + flash * 0.6 + burstK * 0.8;

    // 线框壳缓旋
    shell.rotation.y += step * 0.25;
    shell.rotation.x += step * 0.06;
    (shell.material as THREE.MeshBasicMaterial).color.copy(tintA);

    // 粒子环：反向旋转 + 声波震动（径向缩放脉动 + 粒子尺寸随音频放大）
    const speed = params.ringSpeedFactor;
    innerRing.rotation.y += step * 0.4 * speed;
    outerRing.rotation.y -= step * 0.25 * speed;
    const outerScale = outerRing.scale.x + (params.outerRingScale - outerRing.scale.x) * lerpK;
    const ringPulse = 1 + vib * (0.07 * Math.sin(timeS * 26) + 0.03 * Math.sin(timeS * 41));
    outerRing.scale.setScalar(outerScale * ringPulse);
    innerRing.scale.setScalar(ringPulse);
    (innerRing.material as THREE.PointsMaterial).size = POINT_SIZE_BASE * (1 + vib * 1.2);
    (outerRing.material as THREE.PointsMaterial).size = POINT_SIZE_BASE * (1 + vib * 0.8);
    (innerRing.material as THREE.PointsMaterial).color.copy(tintA);
    (outerRing.material as THREE.PointsMaterial).color.copy(tintB);

    // 频谱环仅 listening
    spectrum.visible = params.spectrumVisible;
    (spectrum.material as THREE.MeshBasicMaterial).color.copy(tintA);
    applySpectrum();

    // Jarvis 弧线组：状态转速因子 + 呼吸式透明度
    for (let i = 0; i < arcs.length; i++) {
      const arc = arcs[i];
      arc.rotation.z += step * ARC_SPECS[i].speed * params.arcSpeedFactor;
      const mat = arc.material as THREE.LineBasicMaterial;
      mat.color.copy(i % 2 === 0 ? tintA : tintB);
      mat.opacity = 0.3 + 0.25 * (0.5 + 0.5 * Math.sin(timeS * 1.4 + i * 2.1)) + burstK * 0.3;
    }

    // 声浪涟漪：speaking/listening 周期发射，burst 立即一圈满强度
    updateRipples(step, params.rippleGain, audioLevel);
    for (const r of ripples) {
      if (r.age >= 0) (r.mesh.material as THREE.MeshBasicMaterial).color.copy(tintA);
    }

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

    burst(): void {
      burstLeft = BURST_SECONDS;
      emitRipple(1);
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
      for (const arc of arcs) {
        arc.geometry.dispose();
        (arc.material as THREE.Material).dispose();
      }
      for (const r of ripples) {
        r.mesh.geometry.dispose();
        (r.mesh.material as THREE.Material).dispose();
      }
      renderer.dispose();
    },
  };
}
