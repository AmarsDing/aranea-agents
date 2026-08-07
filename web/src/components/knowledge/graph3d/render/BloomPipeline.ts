/**
 * BloomPipeline：G5 辉光后处理管线（jarvis 参数融合 orrery，设计 §V12.8-1 C-4）。
 *
 * - EffectComposer(RenderPass + UnrealBloomPass)，bloom 半分辨率（w/2×h/2，性能）
 * - strength≈1.2 / radius=0.5 / threshold=0.28（星云·核雾压在阈值下防糊屏）
 * - setBloomEnabled(false) 整 pass 禁用（性能兜底开关）
 */
import * as THREE from 'three';
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js';
import { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js';
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js';

export interface BloomOpts {
  strength?: number;
  radius?: number;
  threshold?: number;
}

export const BLOOM_DEFAULTS = { strength: 1.2, radius: 0.5, threshold: 0.28 } as const;

export class BloomPipeline {
  readonly composer: EffectComposer;
  readonly bloomPass: UnrealBloomPass;
  private readonly renderPass: RenderPass;

  constructor(
    renderer: THREE.WebGLRenderer,
    scene: THREE.Scene,
    camera: THREE.Camera,
    width: number,
    height: number,
    opts: BloomOpts = {},
  ) {
    this.renderPass = new RenderPass(scene, camera);
    this.bloomPass = new UnrealBloomPass(
      new THREE.Vector2(Math.max(1, width / 2), Math.max(1, height / 2)),
      opts.strength ?? BLOOM_DEFAULTS.strength,
      opts.radius ?? BLOOM_DEFAULTS.radius,
      opts.threshold ?? BLOOM_DEFAULTS.threshold,
    );
    this.composer = new EffectComposer(renderer);
    this.composer.addPass(this.renderPass);
    this.composer.addPass(this.bloomPass);
  }

  setSize(width: number, height: number): void {
    this.composer.setSize(width, height);
    // composer.setSize 会用全分辨率回调各 pass，bloom 覆写回半分辨率
    this.bloomPass.setSize(Math.max(1, width / 2), Math.max(1, height / 2));
  }

  /** bloom 开关：关=整 pass enabled=false（不是 strength=0，避免白跑后处理）。 */
  setBloomEnabled(on: boolean): void {
    this.bloomPass.enabled = on;
  }

  render(dt?: number): void {
    this.composer.render(dt);
  }

  dispose(): void {
    this.composer.dispose();
    this.bloomPass.dispose();
  }
}
