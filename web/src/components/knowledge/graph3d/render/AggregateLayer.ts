/**
 * AggregateLayer：G5 渲染管线 v2 聚合层（超点渲染，远距离语义缩放）。
 *
 * - 按 doc_type 分组，同组节点 >= 3 时聚合为一个超点；<3 保持原样（仍由 NodeLayer 渲染）
 * - 超点位置 = 组内节点质心（每帧从位置纹理重算）；半径 ∝ √成员数；颜色 = 组色
 * - 远距离（LOD FAR/MID）显示超点 + 组标签；近距离（NEAR）隐藏超点，NodeLayer 接管
 * - 点击超点 → 触发 focus-group 事件（父组件过滤右侧面板 + 飞行相机到组中心）
 */
import * as THREE from 'three';
import type { GraphModel } from '../../../features/knowledge/graph3d/model';

/** 超点半径系数（√成员数 × 系数）。 */
export const AGG_SIZE_BASE = 4.0;
/** 超点最小/最大半径（像素）。 */
export const AGG_SIZE_MIN = 12;
export const AGG_SIZE_MAX = 48;
/** 组内节点数 < 此值不聚合。 */
export const AGG_MIN_GROUP_SIZE = 3;

export interface AggregateGroup {
  /** 组名（doc_type）。 */
  name: string;
  /** 组内节点索引（原 GraphModel index）。 */
  members: number[];
  /** 组色（RGB float）。 */
  color: [number, number, number];
  /** 超点半径（世界单位）。 */
  size: number;
  /** 质心（世界坐标，每帧更新）。 */
  centroid: THREE.Vector3;
}

/** 从 GraphModel 计算聚合分组（纯函数，可单测）。 */
export function computeAggregates(model: GraphModel, minSize = AGG_MIN_GROUP_SIZE): AggregateGroup[] {
  const byGroup = new Map<string, number[]>();
  for (let i = 0; i < model.count; i++) {
    const g = model.groups[model.groupId[i]];
    if (!byGroup.has(g)) byGroup.set(g, []);
    byGroup.get(g)!.push(i);
  }
  const result: AggregateGroup[] = [];
  for (const [name, members] of byGroup) {
    if (members.length < minSize) continue;
    result.push({
      name,
      members,
      color: [0.5, 0.5, 0.5], // 占位，由 palette 注入
      size: Math.max(AGG_SIZE_MIN, Math.min(AGG_SIZE_MAX, Math.sqrt(members.length) * AGG_SIZE_BASE)),
      centroid: new THREE.Vector3(),
    });
  }
  return result;
}

const AGG_VERTEX = `
  uniform float uPointScale;
  uniform float uRevealT;
  attribute vec3 aPosition;
  attribute float aSize;
  attribute vec3 aColor;
  attribute float aEmph;
  varying vec3 vColor;
  varying float vEmph;
  varying float vFade;
  void main() {
    vec4 mv = modelViewMatrix * vec4(aPosition, 1.0);
    gl_Position = projectionMatrix * mv;
    float px = aSize * uPointScale / max(-mv.z, 1.0);
    gl_PointSize = clamp(px, ${AGG_SIZE_MIN.toFixed(1)}, ${AGG_SIZE_MAX.toFixed(1)});
    gl_PointSize *= (0.2 + 0.8 * uRevealT);
    vFade = clamp(px / 2.2, 0.35, 1.0);
    vFade *= uRevealT;
    vColor = aColor;
    vEmph = aEmph;
  }`;

const AGG_FRAGMENT = `
  varying vec3 vColor;
  varying float vEmph;
  varying float vFade;
  void main() {
    vec2 uv = gl_PointCoord * 2.0 - 1.0;
    float d2 = dot(uv, uv);
    if (d2 > 1.0) discard;
    // 超点更朦胧：亮核弱、外晕强（与 NodeLayer 区分，表示"这是一团"）
    float core = smoothstep(0.06, 0.0, d2);
    float halo = 1.0 - smoothstep(0.0, 1.0, d2);
    float a = (core * 0.5 + halo * halo * 0.4) * vFade * min(vEmph, 1.0);
    vec3 col = vColor * (0.85 + 0.3 * core) * vEmph;
    gl_FragColor = vec4(col, a);
    #include <tonemapping_fragment>
    #include <colorspace_fragment>
  }`;

export class AggregateLayer {
  readonly points: THREE.Points;
  private readonly geometry: THREE.BufferGeometry;
  private readonly material: THREE.ShaderMaterial;
  private readonly groups: AggregateGroup[];
  private readonly positionsAttr: THREE.BufferAttribute;
  private readonly sizes: Float32Array;

  constructor(groups: AggregateGroup[]) {
    this.groups = groups;
    const n = groups.length;
    this.geometry = new THREE.BufferGeometry();
    this.geometry.setAttribute('position', new THREE.BufferAttribute(new Float32Array(n * 3), 3));
    this.positionsAttr = this.geometry.getAttribute('position') as THREE.BufferAttribute;
    this.positionsAttr.setUsage(THREE.DynamicDrawUsage);
    this.geometry.setAttribute('aColor', new THREE.BufferAttribute(new Float32Array(n * 3), 3));
    this.sizes = new Float32Array(n);
    this.geometry.setAttribute('aSize', new THREE.BufferAttribute(this.sizes, 1));
    const emph = new THREE.BufferAttribute(new Float32Array(n).fill(1), 1);
    emph.setUsage(THREE.DynamicDrawUsage);
    this.geometry.setAttribute('aEmph', emph);

    this.material = new THREE.ShaderMaterial({
      uniforms: {
        uPointScale: { value: 540 },
        uRevealT: { value: 1 },
      },
      vertexShader: AGG_VERTEX,
      fragmentShader: AGG_FRAGMENT,
      transparent: true,
      depthWrite: false,
      depthTest: false,
      blending: THREE.NormalBlending,
    });
    this.points = new THREE.Points(this.geometry, this.material);
    this.points.frustumCulled = false;
    this.points.renderOrder = 0; // 与边同层（节点之下，避免遮挡细节）
    this.points.visible = false; // 初始隐藏（LOD=NEAR 默认），由 applyLodVisibility 接管
  }

  /** 注入组色（palette 顺序与 groups 数组一致）。 */
  setColors(colors: Float32Array): void {
    const attr = this.geometry.getAttribute('aColor') as THREE.BufferAttribute;
    (attr.array as Float32Array).set(colors);
    attr.needsUpdate = true;
  }

  /** 每帧从物理位置重算质心（engine.positions 直读，零拷贝）。 */
  updateCentroids(positions: Float32Array): void {
    const n = this.groups.length;
    for (let g = 0; g < n; g++) {
      const grp = this.groups[g];
      let cx = 0, cy = 0, cz = 0;
      const m = grp.members;
      for (let i = 0; i < m.length; i++) {
        const idx = m[i] * 3;
        cx += positions[idx];
        cy += positions[idx + 1];
        cz += positions[idx + 2];
      }
      cx /= m.length;
      cy /= m.length;
      cz /= m.length;
      grp.centroid.set(cx, cy, cz);
      this.positionsAttr.setXYZ(g, cx, cy, cz);
    }
    this.positionsAttr.needsUpdate = true;
  }

  /** 点像素缩放（resize 时调用）。 */
  setPointScale(scale: number): void {
    this.material.uniforms.uPointScale.value = scale;
  }

  /** M3 创世绽放同步（与 NodeLayer 一致）。 */
  setRevealT(t: number): void {
    this.material.uniforms.uRevealT.value = t;
  }

  /** 高亮/压暗（与 NodeLayer 语义一致：hover/selected 组提亮，其余压暗）。 */
  setHighlight(groupIndices: Set<number> | null): void {
    const attr = this.geometry.getAttribute('aEmph') as THREE.BufferAttribute;
    const arr = attr.array as Float32Array;
    if (groupIndices === null || groupIndices.size === 0) {
      arr.fill(1.0);
    } else {
      arr.fill(0.15);
      for (const i of groupIndices) {
        if (i >= 0 && i < this.groups.length) arr[i] = 1.6;
      }
    }
    attr.needsUpdate = true;
  }

  /** 超点半径（Picker 拾取用）。 */
  groupSize(i: number): number {
    return this.groups[i]?.size ?? 0;
  }

  /** 超点世界坐标（标签定位用）。 */
  groupCentroid(i: number): THREE.Vector3 {
    return this.groups[i].centroid;
  }

  get groupCount(): number {
    return this.groups.length;
  }

  groupName(i: number): string {
    return this.groups[i]?.name ?? '';
  }

  dispose(): void {
    this.geometry.dispose();
    this.material.dispose();
  }
}
