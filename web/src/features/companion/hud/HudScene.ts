/**
 * Three.js 光球 HUD 场景组合器（M74 设计 §7.4 v3，V7 TwinSprite 光球复刻）。
 *
 * 职责：renderer + 帧循环 + 状态参数分发；视觉为 TwinSprite SpriteOrb 复刻——
 * 噪声置换发光球体（`parts/SpriteOrbCore`）+ 倾斜轨道粒子环（`parts/OrbitRing`），
 * 无后处理（TwinSprite 无 Bloom，加法混合 + CSS 光晕即成辉光）。
 *
 * 相机/驱动公式与 TwinSprite 一致：fov 45、z 5.2；电平指数平滑 0.2/帧
 * （dt 归一化 ×12/s）；uAmp = 0.12+level×0.38。
 *
 * 保留本产品增量：boot 点亮过场（1.2s）、speaking 相机微震、burst 能量脉冲、
 * HUD 不可见（document.hidden）降帧 15fps（NFR5 ≥40fps）。
 * 音频数据源通过 provider 回调注入（拉取模型，场景每帧自取），
 * 场景不 import 任何 voice 模块，保持单向依赖。
 */

import * as THREE from 'three';

import type { VoiceState } from '../types';
import { hudParamsFor } from './hudParams';
import type { HudAudioFrame, HudPart } from './parts/HudPart';
import { SpriteOrbCore } from './parts/SpriteOrbCore';
import { OrbitRing } from './parts/OrbitRing';

/** 场景配置（注入音频数据源，方便测试与解耦）。 */
export type HudSceneOptions = {
  /** 返回播放侧实时振幅 [0,1]（speaking 状态驱动光球震动）。 */
  getPlaybackLevel?: (() => number) | null;
  /** 返回采集侧实时电平 [0,1]（listening 状态驱动光球震动）。 */
  getMicLevel?: (() => number) | null;
};

/**
 * 3D 形象渲染器抽象（设计 §3 D7）。
 * V7 实现：TwinSprite 光球；未来可平滑替换为 VRM 人形实现而不影响上层。
 */
export interface AvatarRenderer {
  setState(state: VoiceState): void;
  /** 语音模式开关：开启时触发 ~1.2s 启动过场（光球由微光点亮至满功率）。 */
  setVoiceMode(on: boolean): void;
  /** 触发一次能量脉冲（如确认批准 / 进入播报）：电平瞬时冲高衰减。 */
  burst(): void;
  resize(width: number, height: number): void;
  dispose(): void;
}

const FLASH_SECONDS = 0.3;
const FLASH_BOOST = 0.5;
const BURST_SECONDS = 0.6;
const BURST_LEVEL_BOOST = 0.8;
const BOOT_SECONDS = 1.2;
const IDLE_FPS = 15;
const IDLE_FRAME_MS = 1000 / IDLE_FPS;
/** TwinSprite 电平平滑 0.2/帧（60fps）→ dt 归一化系数。 */
const LEVEL_SMOOTH_RATE = 12;

export class HudScene implements AvatarRenderer {
  private readonly renderer: THREE.WebGLRenderer;
  private readonly scene = new THREE.Scene();
  private readonly camera: THREE.PerspectiveCamera;
  private readonly clock = new THREE.Clock();
  private readonly parts: HudPart[] = [];
  private readonly options: HudSceneOptions;

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

    // TwinSprite 相机原值
    this.camera = new THREE.PerspectiveCamera(45, 1, 0.1, 50);
    this.camera.position.set(0, 0, 5.2);

    // 场景部件（设计 §7.4 v3）
    this.parts.push(new SpriteOrbCore(this.scene));
    this.parts.push(new OrbitRing(this.scene));

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
      // V5.1 沿用：开始播报即能量脉冲——电平冲高驱动振幅/粒子环加速
      this.burst();
    }
    this.state = state;
  }

  setVoiceMode(on: boolean): void {
    if (on) {
      // 开启：从当前进度推进到 1（重复开启可续推）
      this.booting = true;
    } else {
      // 关闭：立即回待机微光
      this.booting = false;
      this.bootProgress = 0;
    }
  }

  burst(): void {
    this.burstTimer = BURST_SECONDS;
  }

  resize(width: number, height: number): void {
    this.renderer.setSize(width, height, false);
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

    // 音频采样（拉取模型）：speaking 播放侧振幅 / listening 采集侧电平
    let target = 0;
    if (this.state === 'speaking') {
      target = this.options.getPlaybackLevel?.() ?? 0;
    } else if (this.state === 'listening') {
      target = this.options.getMicLevel?.() ?? 0;
    }
    this.level += (target - this.level) * Math.min(1, dt * LEVEL_SMOOTH_RATE);

    // 状态计时器
    this.flashTimer = Math.max(0, this.flashTimer - dt);
    this.burstTimer = Math.max(0, this.burstTimer - dt);

    const params = hudParamsFor(this.state, this.level, this.bootProgress);
    const colorA = new THREE.Color(params.tintA);
    const colorB = new THREE.Color(params.tintB);

    // burst 能量脉冲：电平瞬时冲高（衰减）→ uAmp/环速/环缩放同步冲高
    const burstLevel = (this.burstTimer / BURST_SECONDS) * BURST_LEVEL_BOOST;
    const effectiveLevel = Math.min(1, this.level + burstLevel);
    // 打断红闪：强度瞬时提亮
    const flashBoost = (this.flashTimer / FLASH_SECONDS) * FLASH_BOOST;
    if (flashBoost > 0) {
      params.intensity = Math.min(1.5, params.intensity + flashBoost);
    }

    // speaking 相机微震（V5.1 沿用）：播报电平驱动的高频微抖
    const shake = params.shakeGain * effectiveLevel * 0.045;
    if (shake > 0.0005) {
      this.camera.position.x = (Math.sin(this.timeS * 47.3) + Math.sin(this.timeS * 31.7)) * shake;
      this.camera.position.y = (Math.cos(this.timeS * 41.9) + Math.sin(this.timeS * 37.1)) * shake;
      this.camera.lookAt(0, 0, 0);
    } else {
      this.camera.position.set(0, 0, 5.2);
    }

    const audio: HudAudioFrame = { level: effectiveLevel };
    for (const part of this.parts) {
      part.setTint(colorA, colorB);
      part.update(dt, this.timeS, params, audio);
    }

    this.renderer.render(this.scene, this.camera);
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
    this.renderer.dispose();
  }
}
