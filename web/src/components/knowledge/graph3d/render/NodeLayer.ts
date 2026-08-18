/**
 * NodeLayer：G5 渲染管线 v2 节点层（THREE.Points + 位置纹理，万级零 CPU/tick）。
 *
 * - 1 节点 = 1 顶点：顶点着色器 gl_VertexID → texelFetch 位置纹理（替代 InstancedMesh
 *   每 tick 矩阵重组 + 640KB 上传）；静态属性 aColor/aSize，动态属性 aEmph
 * - 柔光点（Obsidian 签名光点）：core+halo 径向衰减，普通混合（弃加法混合——重叠不烧白）
 * - 亮度收敛：rest 增益压在 bloom 阈值下，高亮 ×1.6 才冒辉光
 * - 高亮语义：一跳邻居 emph=1.6，其余压暗 0.15（向深空底淡出），null 全恢复 1.0
 */
import * as THREE from 'three';

/** 高亮/压暗/常态 emph 取值。 */
export const EMPH_DIM = 0.15;
export const EMPH_NORMAL = 1.0;
export const EMPH_HI = 1.6;
/** UX 亮度预算：高亮集规模超此阈值时 emph 按 √(预算/规模) 衰减（下限 EMPH_HI_FLOOR）。
 *  大 hub 一跳覆盖数百节点时，全部 ×1.6 冲过 bloom 阈值叠成白幕；
 *  衰减后高亮仅微亮于常态，聚焦对比由 dim 0.15 反衬承担。 */
export const EMPH_HI_BUDGET = 40;
export const EMPH_HI_FLOOR = 1.05;

/** 高亮 emph 规模自适应（纯函数，可单测）。 */
export function emphForCount(count: number): number {
  if (count <= EMPH_HI_BUDGET) return EMPH_HI;
  return Math.max(EMPH_HI_FLOOR, EMPH_HI * Math.sqrt(EMPH_HI_BUDGET / count));
}

const NODE_VERTEX = `
  uniform sampler2D uPosTex;
  uniform float uTexW;
  uniform float uPointScale;
  uniform float uRevealT;
  attribute float aSize;
  attribute vec3 aColor;
  attribute float aEmph;
  varying vec3 vColor;
  varying float vEmph;
  varying float vFade;
  void main() {
    int idx = gl_VertexID;
    vec3 wp = texelFetch(uPosTex, ivec2(idx % int(uTexW), idx / int(uTexW)), 0).xyz;
    vec4 mv = modelViewMatrix * vec4(wp, 1.0);
    gl_Position = projectionMatrix * mv;
    float px = aSize * uPointScale / max(-mv.z, 1.0);
    // UX 优化：上限 56→40（收敛满屏光斑，密集区不再糊成一片）
    gl_PointSize = clamp(px, 1.6, 40.0);
    gl_PointSize *= (0.2 + 0.8 * uRevealT); // M3 创世绽放：收拢 → 全尺寸
    vFade = clamp(px / 2.2, 0.35, 1.0); // 亚像素淡出防抖
    vFade *= uRevealT; // M3 创世绽放：透明 → 全显现
    vColor = aColor;
    vEmph = aEmph;
  }`;

const NODE_FRAGMENT = `
  varying vec3 vColor;
  varying float vEmph;
  varying float vFade;
  void main() {
    vec2 uv = gl_PointCoord * 2.0 - 1.0;
    float d2 = dot(uv, uv);
    if (d2 > 1.0) discard;
    float core = smoothstep(0.10, 0.0, d2);    // 亮核（半径 ~32%，收紧防粘连）
    float halo = 1.0 - smoothstep(0.0, 1.0, d2); // 外晕
    // UX 收敛：halo 平方衰减 + 权重 0.3→0.2——密集区数百节点 halo 法线混合叠加成白雾淹没标签
    float a = (core * 0.9 + halo * halo * 0.2) * vFade * min(vEmph, 1.0);
    vec3 col = vColor * (0.72 + 0.4 * core) * vEmph;
    gl_FragColor = vec4(col, a);
    #include <tonemapping_fragment>
    #include <colorspace_fragment>
  }`;

export class NodeLayer {
  readonly points: THREE.Points;
  private readonly geometry: THREE.BufferGeometry;
  private readonly material: THREE.ShaderMaterial;
  private readonly sizes: Float32Array;
  private readonly count: number;
  private highlighted: Set<number> | null = null;

  constructor(count: number) {
    this.count = count;
    this.geometry = new THREE.BufferGeometry();
    // position 仅作顶点计数驱动（真实位置走 uPosTex）；静态属性一次写入
    this.geometry.setAttribute('position', new THREE.BufferAttribute(new Float32Array(count * 3), 3));
    this.geometry.setAttribute('aColor', new THREE.BufferAttribute(new Float32Array(count * 3), 3));
    this.geometry.setAttribute('aSize', new THREE.BufferAttribute(new Float32Array(count).fill(1), 1));
    const emph = new THREE.BufferAttribute(new Float32Array(count).fill(EMPH_NORMAL), 1);
    emph.setUsage(THREE.DynamicDrawUsage);
    this.geometry.setAttribute('aEmph', emph);

    this.material = new THREE.ShaderMaterial({
      uniforms: {
        uPosTex: { value: null },
        uTexW: { value: 1 },
        uPointScale: { value: 540 },
        uRevealT: { value: 1 },
      },
      vertexShader: NODE_VERTEX,
      fragmentShader: NODE_FRAGMENT,
      transparent: true,
      depthWrite: false,
      depthTest: false,
      blending: THREE.NormalBlending,
    });
    this.points = new THREE.Points(this.geometry, this.material);
    this.points.frustumCulled = false;
    this.points.renderOrder = 1; // 分层：backdrop(-1) → 边(0) → 节点(1) → 粒子/瞄准具(2) → 标签(3)
    this.sizes = new Float32Array(count).fill(1);
  }

  /** 绑定位置纹理（Canvas 持有 PositionTexture）。 */
  setPositionTexture(texture: THREE.DataTexture, width: number): void {
    this.material.uniforms.uPosTex.value = texture;
    this.material.uniforms.uTexW.value = width;
  }

  /** 点像素缩放 = viewportHeightPx·0.5 / tan(fov/2)（resize 时调用）。 */
  setPointScale(scale: number): void {
    this.material.uniforms.uPointScale.value = scale;
  }

  /** M3 创世绽放：0=收拢于核心，1=完全显现（默认 1 无动画）。 */
  setRevealT(t: number): void {
    this.material.uniforms.uRevealT.value = t;
  }

  /** 基础色（3N RGB float，palette 注入）。 */
  setColors(colors: Float32Array): void {
    const attr = this.geometry.getAttribute('aColor') as THREE.BufferAttribute;
    (attr.array as Float32Array).set(colors);
    attr.needsUpdate = true;
  }

  /** 大小 = (base + √degree·scale) × sizeMult[i]（缺省 1）。 */
  setSizes(degree: Uint16Array, base: number, scale: number, sizeMult?: Float32Array): void {
    const attr = this.geometry.getAttribute('aSize') as THREE.BufferAttribute;
    const arr = attr.array as Float32Array;
    for (let i = 0; i < degree.length; i++) {
      this.sizes[i] = (base + Math.sqrt(degree[i]) * scale) * (sizeMult ? sizeMult[i] : 1);
      arr[i] = this.sizes[i];
    }
    attr.needsUpdate = true;
  }

  /** 高亮一跳邻居集：集内 1.6（冒辉光），集外 0.15（压暗）；null/空集全恢复 1.0。 */
  setHighlight(indices: Set<number> | null): void {
    const attr = this.geometry.getAttribute('aEmph') as THREE.BufferAttribute;
    const arr = attr.array as Float32Array;
    if (indices === null || indices.size === 0) {
      arr.fill(EMPH_NORMAL);
      this.highlighted = null;
    } else {
      arr.fill(EMPH_DIM);
      for (const i of indices) {
        if (i >= 0 && i < this.count) arr[i] = EMPH_HI;
      }
      this.highlighted = indices;
    }
    attr.needsUpdate = true;
  }

  get highlightedSet(): Set<number> | null {
    return this.highlighted;
  }

  nodeSize(i: number): number {
    return this.sizes[i];
  }

  /** 全量半径缓冲（Picker 拾取阈值用，只读视图）。 */
  get sizeData(): Float32Array {
    return this.sizes;
  }

  dispose(): void {
    this.geometry.dispose();
    this.material.dispose();
  }
}
