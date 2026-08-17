/**
 * BloomPipeline：G5 辉光后处理管线 v2（参数收敛降亮度 + 分辨率档位，设计 §V12.8-1 C-4）。
 *
 * - EffectComposer(RenderPass + UnrealBloomPass)，bloom 分辨率随画质档缩放（bloomScale）
 * - v2 收敛：strength 1.2→0.9 / radius 0.5→0.35 / threshold 0.28→0.55（只有高亮节点冒辉光）
 * - v3 可读性收敛：strength 0.9→0.55 / radius 0.35→0.30 / threshold 0.55→0.85
 *   （标签色亮度≈0.8 原超阈值导致每行文字套白晕；提阈后仅 emph×1.6 高亮节点冒光）
 */
import * as THREE from 'three';
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js';
import { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js';
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js';

export interface BloomOpts {
  strength?: number;
  radius?: number;
  threshold?: number;
  /** bloom 分辨率相对画布比例（默认 0.5；画质降档时调小）。 */
  resolutionScale?: number;
}

export const BLOOM_DEFAULTS = { strength: 0.55, radius: 0.3, threshold: 0.85, resolutionScale: 0.5 } as const;

export class BloomPipeline {
  readonly composer: EffectComposer;
  readonly bloomPass: UnrealBloomPass;
  private readonly renderPass: RenderPass;
  private resolutionScale: number;
  private width = 0;
  private height = 0;

  constructor(
    renderer: THREE.WebGLRenderer,
    scene: THREE.Scene,
    camera: THREE.Camera,
    width: number,
    height: number,
    opts: BloomOpts = {},
  ) {
    this.resolutionScale = opts.resolutionScale ?? BLOOM_DEFAULTS.resolutionScale;
    this.renderPass = new RenderPass(scene, camera);
    this.bloomPass = new UnrealBloomPass(
      new THREE.Vector2(Math.max(1, width * this.resolutionScale), Math.max(1, height * this.resolutionScale)),
      opts.strength ?? BLOOM_DEFAULTS.strength,
      opts.radius ?? BLOOM_DEFAULTS.radius,
      opts.threshold ?? BLOOM_DEFAULTS.threshold,
    );
    this.composer = new EffectComposer(renderer);
    this.composer.addPass(this.renderPass);
    this.composer.addPass(this.bloomPass);
    this.width = width;
    this.height = height;
  }

  setSize(width: number, height: number): void {
    this.width = width;
    this.height = height;
    this.composer.setSize(width, height);
    // composer.setSize 会用全分辨率回调各 pass，bloom 覆写回档位分辨率
    this.bloomPass.setSize(Math.max(1, width * this.resolutionScale), Math.max(1, height * this.resolutionScale));
  }

  /** 运行期调整 bloom 分辨率档（画质 governor 降档省 GPU）。 */
  setResolutionScale(scale: number): void {
    if (scale === this.resolutionScale) return;
    this.resolutionScale = scale;
    if (this.width > 0) this.setSize(this.width, this.height);
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
