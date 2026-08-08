/**
 * Three.js 反应堆 HUD 场景组合器（M74 设计 §7.4 v2，V5 科幻重构）。
 *
 * 职责：renderer + EffectComposer（RenderPass/UnrealBloomPass/OutputPass）+
 * 帧循环 + 状态参数分发；视觉元素在 `hud/parts/*` 模块化部件中。
 *
 * 性能预算：NFR5 ≥40fps；HUD 不可见（document.hidden）时降帧至 15fps。
 * 音频数据源通过 provider 回调注入（拉取模型，场景每帧自取），
 * 场景不 import 任何 voice 模块，保持单向依赖。
 */

import * as THREE from 'three';
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js';
import { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js';
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js';
import { OutputPass } from 'three/examples/jsm/postprocessing/OutputPass.js';

import type { VoiceState } from '../types';
import { hudParamsFor } from './hudParams';
import type { HudAudioFrame, HudPart } from './parts/HudPart';
import { ReactorCore } from './parts/ReactorCore';
import { ReactorRings } from './parts/ReactorRings';
import { Starfield } from './parts/Starfield';
import { SpectrumRing } from './parts/SpectrumRing';
import { ShockwavePool } from './parts/ShockwavePool';
import { EnergyParticles } from './parts/EnergyParticles';

/** 场景配置（注入音频数据源，方便测试与解耦）。 */
export type HudSceneOptions = {
  /** 返回播放侧实时振幅 [0,1]（speaking 状态驱动能量核脉动）。 */
  getPlaybackLevel?: (() => number) | null;
  /** 返回采集侧实时振幅 [0,1]（listening 状态备用）。 */
  getMicLevel?: (() => number) | null;
  /** 返回采集侧 FFT 频谱（填充传入数组，listening 频谱环）。 */
  fillMicSpectrum?: ((bins: Uint8Array) => void) | null;
};

/**
 * 3D 形象渲染器抽象（设计 §3 D7）。
 * V5 实现：Three.js 反应堆 HUD；未来可平滑替换为 VRM 人形实现而不影响上层。
 */
export interface AvatarRenderer {
  setState(state: VoiceState): void;
  /** 语音模式开关：开启时触发 ~1.2s 启动过场（核心点亮 + 刻度环展开）。 */
  setVoiceMode(on: boolean): void;
  /** 触发一次能量脉冲（如确认批准）；涟漪爆发 + Bloom 提亮。 */
  burst(): void;
  resize(width: number, height: number): void;
  dispose(): void;
}

const FLASH_SECONDS = 0.3;
const FLASH_BOOST = 0.5;
const BURST_SECONDS = 0.6;
const BURST_BOOST = 0.8;
const BOOT_SECONDS = 1.2;
const IDLE_FPS = 15;
const IDLE_FRAME_MS = 1000 / IDLE_FPS;

export class HudScene implements AvatarRenderer {
  private readonly renderer: THREE.WebGLRenderer;
  private readonly composer: EffectComposer;
  private readonly bloomPass: UnrealBloomPass;
  private readonly scene = new THREE.Scene();
  private readonly camera: THREE.PerspectiveCamera;
  private readonly clock = new THREE.Clock();
  private readonly parts: HudPart[] = [];
  private readonly ripples: ShockwavePool;
  private readonly options: HudSceneOptions;
  private readonly spectrumBins = new Uint8Array(128);

  private state: VoiceState = 'idle';
  private timeS = 0;
  private rafId: number | null = null;
  private lastVisibleFrameAt = 0;
  private level = 0;
  private flashTimer = 0;
  private burstTimer = 0;
  private bootProgress = 0;
  private booting = false;
  private disposed = false;

  private readonly onVisibilityChange = () => {
    if (!document.hidden) {
      // 回到前台时重置时钟，避免大 dt 跳变
      this.clock.getDelta();
    }
  };

  constructor(canvas: HTMLCanvasElement, options: HudSceneOptions = {}) {
    this.options = options;
    this.renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true });
    this.renderer.setClearColor(0x000000, 0);
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));

    this.camera = new THREE.PerspectiveCamera(50, 1, 0.1, 60);
    this.camera.position.set(0, 0, 3.6);

    // 场景部件（设计 §7.4 v2）
    this.parts.push(new ReactorCore(this.scene));
    this.parts.push(new ReactorRings(this.scene));
    this.parts.push(new Starfield(this.scene));
    this.parts.push(new SpectrumRing(this.scene));
    this.parts.push(new EnergyParticles(this.scene));
    this.ripples = new ShockwavePool(this.scene);
    this.parts.push(this.ripples);

    // 后处理管线：Bloom 为「炫酷感」核心（V5-T1）
    this.composer = new EffectComposer(this.renderer);
    this.composer.addPass(new RenderPass(this.scene, this.camera));
    this.bloomPass = new UnrealBloomPass(new THREE.Vector2(1, 1), 0.7, 0.55, 0.12);
    this.composer.addPass(this.bloomPass);
    this.composer.addPass(new OutputPass());

    const rect = canvas.getBoundingClientRect();
    this.resize(Math.max(rect.width, 1), Math.max(rect.height, 1));

    document.addEventListener('visibilitychange', this.onVisibilityChange);
    this.start();
  }

  setState(state: VoiceState): void {
    if (state === this.state) return;
    if (state === 'interrupted') {
      this.flashTimer = FLASH_SECONDS;
    }
    if (state === 'speaking') {
      // V5.1：开始播报即能量爆发——涟漪 + Bloom 提亮 + 粒子满功率（particleGain）
      this.burst();
    }
    this.state = state;
  }

  setVoiceMode(on: boolean): void {
    if (on) {
      // 开启：从当前进度推进到 1（重复开启可续推）
      this.booting = true;
    } else {
      // 关闭：立即回待机熄灭
      this.booting = false;
      this.bootProgress = 0;
    }
  }

  burst(): void {
    this.burstTimer = BURST_SECONDS;
    this.ripples.queueBurst();
  }

  resize(width: number, height: number): void {
    this.renderer.setSize(width, height, false);
    this.composer.setSize(width, height);
    this.camera.aspect = width / height;
    this.camera.updateProjectionMatrix();
  }

  private start(): void {
    if (this.rafId !== null) return;
    this.clock.start();
    const loop = () => {
      if (this.disposed) return;
      this.rafId = requestAnimationFrame(loop);
      if (document.hidden) {
        const now = performance.now();
        if (now - this.lastVisibleFrameAt < IDLE_FRAME_MS) return;
        this.lastVisibleFrameAt = now;
      }
      this.tick();
    };
    this.rafId = requestAnimationFrame(loop);
  }

  private tick(): void {
    const dt = Math.min(this.clock.getDelta(), 0.1);
    this.timeS += dt;

    // 启动过场推进（语音模式开启后 1.2s 内 0 → 1）
    if (this.booting && this.bootProgress < 1) {
      this.bootProgress = Math.min(1, this.bootProgress + dt / BOOT_SECONDS);
      if (this.bootProgress >= 1) {
        this.booting = false;
      }
    }

    // 音频采样（拉取模型）
    let target = 0;
    let spectrum: Uint8Array | null = null;
    if (this.state === 'speaking') {
      target = this.options.getPlaybackLevel?.() ?? 0;
    } else if (this.state === 'listening') {
      target = this.options.getMicLevel?.() ?? 0;
      if (this.options.fillMicSpectrum) {
        this.options.fillMicSpectrum(this.spectrumBins);
        spectrum = this.spectrumBins;
      }
    }
    this.level += (target - this.level) * Math.min(1, dt * 18);

    // 状态计时器
    this.flashTimer = Math.max(0, this.flashTimer - dt);
    this.burstTimer = Math.max(0, this.burstTimer - dt);

    const params = hudParamsFor(this.state, this.timeS, this.level, this.bootProgress);
    const colorA = new THREE.Color(params.tintA);
    const colorB = new THREE.Color(params.tintB);

    // Bloom：状态基础强度 + 打断红闪/爆发提亮
    this.bloomPass.strength =
      params.bloomIntensity +
      (this.flashTimer / FLASH_SECONDS) * FLASH_BOOST +
      (this.burstTimer / BURST_SECONDS) * BURST_BOOST;

    // 相机微视差（缓慢漂移增加空间感）
    this.camera.position.x = Math.sin(this.timeS * 0.3) * 0.07;
    this.camera.position.y = Math.cos(this.timeS * 0.23) * 0.05;
    // speaking 震动（V5.1）：播报振幅驱动的高频微抖，叠加在视差之上
    const shake = params.shakeGain * this.level * 0.045;
    if (shake > 0.0005) {
      this.camera.position.x += (Math.sin(this.timeS * 47.3) + Math.sin(this.timeS * 31.7)) * shake;
      this.camera.position.y += (Math.cos(this.timeS * 41.9) + Math.sin(this.timeS * 37.1)) * shake;
    }
    this.camera.lookAt(0, 0, 0);

    const audio: HudAudioFrame = { level: this.level, spectrum };
    for (const part of this.parts) {
      part.setTint(colorA, colorB);
      part.update(dt, this.timeS, params, audio);
    }

    this.composer.render();
  }

  dispose(): void {
    this.disposed = true;
    if (this.rafId !== null) {
      cancelAnimationFrame(this.rafId);
      this.rafId = null;
    }
    document.removeEventListener('visibilitychange', this.onVisibilityChange);
    for (const part of this.parts) {
      part.dispose();
    }
    this.bloomPass.dispose();
    this.composer.dispose();
    this.renderer.dispose();
  }
}
