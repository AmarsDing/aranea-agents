/**
 * NodeLayer：G5 深空图谱节点层（InstancedMesh 移植 fast-graph，设计 §V12.8-1 C-1）。
 *
 * - 低模球 SphereGeometry(1,6,4) + MeshBasicMaterial 加法混合（热核发光感，交 bloom 出辉光）
 * - instanceColor + baseColors 缓存；高亮 lerp(white,0.5)，非邻居向深空底压暗（保留 8% 原色）
 * - 大小 = (base + √degree·scale) × 分级倍率（tiering）
 */
import * as THREE from 'three';

/** 非邻居压暗：底色 92% + 原色 8%（设计「lerp 0.08」）。 */
const DIM_KEEP = 0.08;
/** 深空底色（与不透明 clear 色一致）。 */
const BG_HEX = '#050810';

export class NodeLayer {
  readonly mesh: THREE.InstancedMesh;
  private readonly geometry: THREE.SphereGeometry;
  private readonly material: THREE.MeshBasicMaterial;
  private readonly dummy = new THREE.Object3D();
  private readonly sizes: Float32Array;
  private baseColors: Float32Array | null = null;
  private highlighted: Set<number> | null = null;
  private readonly white = new THREE.Color(0xffffff);
  private readonly bg = new THREE.Color(BG_HEX);
  private readonly tmp = new THREE.Color();

  constructor(count: number) {
    this.geometry = new THREE.SphereGeometry(1, 6, 4);
    this.material = new THREE.MeshBasicMaterial({
      blending: THREE.AdditiveBlending,
      transparent: true,
      depthWrite: false,
    });
    this.mesh = new THREE.InstancedMesh(this.geometry, this.material, count);
    this.mesh.instanceMatrix.setUsage(THREE.DynamicDrawUsage);
    this.mesh.instanceColor = new THREE.InstancedBufferAttribute(new Float32Array(count * 3), 3);
    this.sizes = new Float32Array(count).fill(1);
    this.mesh.frustumCulled = false; // 实例整体剔除失效，交给相机远裁剪
  }

  /** 基础色（3N RGB float，palette 注入）。 */
  setColors(colors: Float32Array): void {
    this.baseColors = colors.slice();
    const attr = this.mesh.instanceColor!;
    (attr.array as Float32Array).set(colors);
    attr.needsUpdate = true;
    this.highlighted = null;
  }

  /** 大小 = (base + √degree·scale) × sizeMult[i]（缺省 1）。 */
  setSizes(degree: Uint16Array, base: number, scale: number, sizeMult?: Float32Array): void {
    for (let i = 0; i < degree.length; i++) {
      this.sizes[i] = (base + Math.sqrt(degree[i]) * scale) * (sizeMult ? sizeMult[i] : 1);
    }
  }

  /** 高亮一跳邻居集：集内提亮 lerp(white,0.5)，集外压暗；null 全恢复。 */
  setHighlight(indices: Set<number> | null): void {
    const base = this.baseColors;
    const attr = this.mesh.instanceColor;
    if (!base || !attr) return;
    const arr = attr.array as Float32Array;
    const c = this.tmp;
    const n = base.length / 3;

    if (indices === null || indices.size === 0) {
      for (let i = 0; i < n; i++) {
        arr[i * 3] = base[i * 3];
        arr[i * 3 + 1] = base[i * 3 + 1];
        arr[i * 3 + 2] = base[i * 3 + 2];
      }
      this.highlighted = null;
      attr.needsUpdate = true;
      return;
    }

    for (let i = 0; i < n; i++) {
      if (indices.has(i)) {
        c.setRGB(base[i * 3], base[i * 3 + 1], base[i * 3 + 2]).lerp(this.white, 0.5);
      } else {
        c.copy(this.bg);
        c.r += base[i * 3] * DIM_KEEP;
        c.g += base[i * 3 + 1] * DIM_KEEP;
        c.b += base[i * 3 + 2] * DIM_KEEP;
      }
      arr[i * 3] = c.r;
      arr[i * 3 + 1] = c.g;
      arr[i * 3 + 2] = c.b;
    }
    this.highlighted = indices;
    attr.needsUpdate = true;
  }

  get highlightedSet(): Set<number> | null {
    return this.highlighted;
  }

  nodeSize(i: number): number {
    return this.sizes[i];
  }

  updatePositions(positions: Float32Array): void {
    const d = this.dummy;
    const n = this.mesh.count;
    for (let i = 0; i < n; i++) {
      d.position.set(positions[i * 3], positions[i * 3 + 1], positions[i * 3 + 2]);
      const s = this.sizes[i];
      d.scale.set(s, s, s);
      d.updateMatrix();
      this.mesh.setMatrixAt(i, d.matrix);
    }
    this.mesh.instanceMatrix.needsUpdate = true;
  }

  dispose(): void {
    this.geometry.dispose();
    this.material.dispose();
    this.mesh.dispose();
  }
}
