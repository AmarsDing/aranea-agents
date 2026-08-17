/**
 * LabelLayer：G5 节点标签层（three-spritetext，设计 §V12.8-1 C-5）。
 *
 * - 候选池：按 degree 取 top-K（默认 80，与 HIGH 档一致），Sprite 常驻复用
 * - 双阈值可见性：相机距离 ≤ maxDistance 且 (degree ≥ minDegree 或 extraVisible 包含)
 * - 标签开关关时只显示 extraVisible（hover/选中节点）
 * - 位置：节点上方 y + nodeSize + 偏移
 */
import * as THREE from 'three';
import SpriteText from 'three-spritetext';

export interface LabelLayerOpts {
  names: string[];
  degree: Uint16Array;
  /** 候选池上限（按 degree 降序取 top-K）。 */
  maxLabels?: number;
  /** 标签文字颜色。 */
  color?: string;
  /** 标签字高（世界单位）。 */
  textHeight?: number;
}

export interface LabelVisibility {
  maxDistance: number;
  minDegree: number;
  /** 强制可见集（hover/选中）。 */
  extraVisible?: Set<number>;
  /** 节点半径查询（标签放在节点上方）。 */
  nodeSize?: (i: number) => number;
}

const LABEL_Y_OFFSET = 3;

/** 候选池：按 degree 降序取 top-K（纯函数，可单测）。 */
export function selectLabelCandidates(degree: Uint16Array, maxLabels: number): number[] {
  return Array.from({ length: degree.length }, (_, i) => i)
    .sort((a, b) => degree[b] - degree[a])
    .slice(0, maxLabels);
}

/** 动态度数下限（纯函数，G5-G 小图标签修复）：图最大度数低于基准时降档到最大度数，
 *  保证小图 hub 标签可见；全孤立图钳到 1（孤立节点不出标签）。 */
export function effectiveMinDegree(maxDegree: number, base: number): number {
  return Math.max(1, Math.min(base, maxDegree));
}

/** 可见性判定（纯函数）：forced（hover/选中）无条件显示（豁免距离/度数/开关——拉远超 maxDistance 时 hover 仍须出标签）；普通标签受 距离+度数+开关 三重阈值。 */
export function shouldShowLabel(
  dist: number,
  maxDistance: number,
  deg: number,
  minDegree: number,
  forced: boolean,
  labelsEnabled: boolean,
): boolean {
  if (forced) return true;
  return dist <= maxDistance && labelsEnabled && deg >= minDegree;
}

export class LabelLayer {
  readonly group = new THREE.Group();
  /** 候选节点索引（degree 降序 top-K）。 */
  readonly candidates: number[];
  private readonly sprites: SpriteText[] = [];
  private readonly degree: Uint16Array;
  private readonly tmp = new THREE.Vector3();
  private labelsEnabled = true;

  constructor(opts: LabelLayerOpts) {
    const { names, degree } = opts;
    this.degree = degree;
    const maxLabels = opts.maxLabels ?? 80;
    // v3 可读性：#9fdcff→#8fb9d9（原色亮度≈0.8 易冒辉光，压暗配合 bloom 提阈 0.85）
    const color = opts.color ?? '#8fb9d9';
    const textHeight = opts.textHeight ?? 4;

    this.candidates = selectLabelCandidates(degree, maxLabels);

    for (const i of this.candidates) {
      const sprite = new SpriteText(names[i], textHeight, color);
      sprite.visible = false;
      sprite.renderOrder = 3;
      this.sprites.push(sprite);
      this.group.add(sprite);
    }
  }

  /** 标签总开关：关时只显示 extraVisible。 */
  setLabelsEnabled(on: boolean): void {
    this.labelsEnabled = on;
  }

  /** 每帧按相机距离/度数双阈值刷新可见性与位置。 */
  update(positions: Float32Array, camera: THREE.Camera, vis: LabelVisibility): void {
    const camPos = camera.position;
    for (let k = 0; k < this.candidates.length; k++) {
      const i = this.candidates[k];
      const sprite = this.sprites[k];
      const x = positions[i * 3];
      const y = positions[i * 3 + 1];
      const z = positions[i * 3 + 2];
      this.tmp.set(x, y, z);
      const dist = this.tmp.distanceTo(camPos);
      const forced = vis.extraVisible?.has(i) ?? false;
      const show = shouldShowLabel(dist, vis.maxDistance, this.degree[i], vis.minDegree, forced, this.labelsEnabled);
      sprite.visible = show;
      if (show) {
        const r = vis.nodeSize ? vis.nodeSize(i) : 0;
        sprite.position.set(x, y + r + LABEL_Y_OFFSET, z);
      }
    }
  }

  dispose(): void {
    for (const s of this.sprites) {
      // SpriteText 无公开 dispose()：手动释放 canvas 纹理与材质（GC 路径）。
      s.material.map?.dispose();
      s.material.dispose();
      this.group.remove(s);
    }
    this.sprites.length = 0;
  }
}
