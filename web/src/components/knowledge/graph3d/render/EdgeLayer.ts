/**
 * EdgeLayer：G5 渲染管线 v2 边层（直线 LineSegments + 位置纹理，每 tick 零 CPU）。
 *
 * - Obsidian 签名细直线：默认每边 2 顶点（segments=1 直线，密图视觉更净）；
 *   M2 可选 segmentsPerEdge=8 + uCurvature 贝塞尔（星系盘弧线沿盘面弯曲，力导向曲率 0 零回归）
 * - 顶点着色器按 aNodeA/aNodeB texelFetch 端点位置 + aT 插值（替代每 tick CPU 重算贝塞尔 + 字符串哈希）
 * - rest=低透明度细线（普通混合）；hover 关联边 = 提亮 + 流动光脉冲（uTime sin 沿边跑动，科幻数据流）
 * - 动态属性仅 aHi（per-edge 0/1），hover 变更时一次性写入
 */
import * as THREE from 'three';

/** rest 透明度 / 高亮透明度。 */
// UX 可读性：0.16→0.11（5121 边法线混合叠加成亮网淹没标签；hover 提亮通道不受影响）
export const EDGE_REST_ALPHA = 0.11;
// UX 聚焦：0.9→0.35（聚焦邻域 2 跳数百条高亮边法线混合叠穿 bloom 阈值成白幕；hover 单边仍 3× 于 rest 对比度）
export const EDGE_HOVER_ALPHA = 0.35;
/** rest 亮度系数 / 高亮亮度系数。 */
export const EDGE_REST_DIM = 0.45;
export const EDGE_HOVER_BOOST = 1.05;

const EDGE_VERTEX = `
  uniform sampler2D uPosTex;
  uniform float uTexW;
  uniform float uCurvature;
  uniform float uBundling;
  attribute float aNodeA;
  attribute float aNodeB;
  attribute float aT;
  attribute vec3 aColor;
  attribute float aHi;
  varying vec3 vColor;
  varying float vHi;
  varying float vT;
  void main() {
    int ia = int(aNodeA + 0.5);
    int ib = int(aNodeB + 0.5);
    vec3 pa = texelFetch(uPosTex, ivec2(ia % int(uTexW), ia / int(uTexW)), 0).xyz;
    vec3 pb = texelFetch(uPosTex, ivec2(ib % int(uTexW), ib / int(uTexW)), 0).xyz;
    vec3 wp = mix(pa, pb, aT);
    if (uBundling > 0.0001) {
      // 边捆绑：同向边捆成束（控制点向源-目标中点收缩，形成"花束"效果）
      vec3 mid = (pa + pb) * 0.5;
      vec3 dir = pb - pa;
      vec3 normal = normalize(vec3(-dir.z, 0.0, dir.x) + vec3(1e-4));
      // 捆绑强度：0=直线，1=完全弯曲到垂直法线
      vec3 ctrl = mid + normal * uBundling * length(dir) * 0.5;
      float t = aT;
      wp = mix(mix(pa, ctrl, t), mix(ctrl, pb, t), t);
    } else if (uCurvature > 0.0001) {
      // 二次贝塞尔：控制点 = 中点 + XZ 平面法向偏移（弧线沿盘面弯曲）
      vec3 mid = (pa + pb) * 0.5;
      vec3 dir = pb - pa;
      vec3 normal = normalize(vec3(-dir.z, 0.0, dir.x) + vec3(1e-4));
      vec3 ctrl = mid + normal * uCurvature * length(dir);
      float t = aT;
      wp = mix(mix(pa, ctrl, t), mix(ctrl, pb, t), t);
    }
    gl_Position = projectionMatrix * modelViewMatrix * vec4(wp, 1.0);
    vColor = aColor;
    vHi = aHi;
    vT = aT;
  }`;

const EDGE_FRAGMENT = `
  uniform float uTime;
  uniform float uRestAlpha;
  uniform float uHoverAlpha;
  uniform float uRestDim;
  uniform float uHoverBoost;
  uniform vec3 uHiColor;
  varying vec3 vColor;
  varying float vHi;
  varying float vT;
  void main() {
    float alpha = mix(uRestAlpha, uHoverAlpha, vHi);
    // V13：高亮链路统一亮青（与 rest 色脱钩——统一暗青灰的常态边也能亮青聚焦）
    vec3 col = mix(vColor * uRestDim, uHiColor * uHoverBoost, vHi);
    if (vHi > 0.5) {
      // 流动光脉冲：沿边跑动的数据流（hover 专属科幻感）；UX 收敛加色防白幕
      float pulse = 0.5 + 0.5 * sin(uTime * 7.0 - vT * 16.0);
      alpha *= 0.7 + 0.45 * pulse;
      col += vec3(0.10, 0.14, 0.16) * pulse;
    }
    gl_FragColor = vec4(col, alpha);
    #include <tonemapping_fragment>
    #include <colorspace_fragment>
  }`;

export class EdgeLayer {
  readonly object: THREE.LineSegments;
  private readonly geometry: THREE.BufferGeometry;
  private readonly material: THREE.ShaderMaterial;
  private readonly edgeCount: number;
  /** 每边顶点数（segmentsPerEdge × 2；LineSegments 逐对连线）。 */
  private readonly verticesPerEdge: number;
  private highlighted: Set<number> | null = null;

  constructor(edges: Int32Array, edgeColors: Float32Array, segmentsPerEdge = 1) {
    this.edgeCount = edges.length / 2;
    this.verticesPerEdge = segmentsPerEdge * 2;
    this.geometry = new THREE.BufferGeometry();
    const vCount = this.edgeCount * this.verticesPerEdge;
    // position 仅作顶点计数驱动（真实端点位置走 uPosTex）
    this.geometry.setAttribute('position', new THREE.BufferAttribute(new Float32Array(vCount * 3), 3));

    const nodeAAttr = new Float32Array(vCount);
    const nodeBAttr = new Float32Array(vCount);
    const tAttr = new Float32Array(vCount);
    const colorAttr = new Float32Array(vCount * 3);
    for (let e = 0; e < this.edgeCount; e++) {
      const na = edges[e * 2];
      const nb = edges[e * 2 + 1];
      const r = edgeColors[e * 3];
      const g = edgeColors[e * 3 + 1];
      const b = edgeColors[e * 3 + 2];
      for (let s = 0; s < segmentsPerEdge; s++) {
        const v0 = e * this.verticesPerEdge + s * 2;
        const v1 = v0 + 1;
        nodeAAttr[v0] = na;
        nodeAAttr[v1] = na;
        nodeBAttr[v0] = nb;
        nodeBAttr[v1] = nb;
        tAttr[v0] = s / segmentsPerEdge;
        tAttr[v1] = (s + 1) / segmentsPerEdge;
        colorAttr[v0 * 3] = r;
        colorAttr[v0 * 3 + 1] = g;
        colorAttr[v0 * 3 + 2] = b;
        colorAttr[v1 * 3] = r;
        colorAttr[v1 * 3 + 1] = g;
        colorAttr[v1 * 3 + 2] = b;
      }
    }
    this.geometry.setAttribute('aNodeA', new THREE.BufferAttribute(nodeAAttr, 1));
    this.geometry.setAttribute('aNodeB', new THREE.BufferAttribute(nodeBAttr, 1));
    this.geometry.setAttribute('aT', new THREE.BufferAttribute(tAttr, 1));
    this.geometry.setAttribute('aColor', new THREE.BufferAttribute(colorAttr, 3));
    const hi = new THREE.BufferAttribute(new Float32Array(vCount), 1);
    hi.setUsage(THREE.DynamicDrawUsage);
    this.geometry.setAttribute('aHi', hi);

    this.material = new THREE.ShaderMaterial({
      uniforms: {
        uPosTex: { value: null },
        uTexW: { value: 1 },
        uTime: { value: 0 },
        uCurvature: { value: 0 },
        uBundling: { value: 0 },
        uRestAlpha: { value: EDGE_REST_ALPHA },
        uHoverAlpha: { value: EDGE_HOVER_ALPHA },
        uRestDim: { value: EDGE_REST_DIM },
        uHoverBoost: { value: EDGE_HOVER_BOOST },
        uHiColor: { value: new THREE.Color('#39e6ff') },
      },
      vertexShader: EDGE_VERTEX,
      fragmentShader: EDGE_FRAGMENT,
      transparent: true,
      depthWrite: false,
      depthTest: false,
      blending: THREE.NormalBlending,
    });
    this.object = new THREE.LineSegments(this.geometry, this.material);
    this.object.frustumCulled = false;
  }

  /** 绑定位置纹理（Canvas 持有 PositionTexture）。 */
  setPositionTexture(texture: THREE.DataTexture, width: number): void {
    this.material.uniforms.uPosTex.value = texture;
    this.material.uniforms.uTexW.value = width;
  }

  /** 流动脉冲时钟（仅高亮活跃期需要推进）。 */
  setTime(t: number): void {
    this.material.uniforms.uTime.value = t;
  }

  /** M2：弧线弯曲系数（0=直线，力导向；>0 星系盘）。 */
  setCurvature(v: number): void {
    this.material.uniforms.uCurvature.value = v;
  }

  /** Phase 2：边捆绑强度（0=直线，0.3=轻度捆绑成束；与 curvature 互斥，bundling 优先）。 */
  setBundling(v: number): void {
    this.material.uniforms.uBundling.value = v;
  }

  /** V13：边颜色热切换（着色模式 unified/typed，不重建层）。colors = 每边 RGB（edgeCount×3）。 */
  setColors(colors: Float32Array): void {
    if (colors.length !== this.edgeCount * 3) {
      throw new Error(`EdgeLayer.setColors: 期望 ${this.edgeCount * 3}，实得 ${colors.length}`);
    }
    const attr = this.geometry.getAttribute('aColor') as THREE.BufferAttribute;
    const arr = attr.array as Float32Array;
    for (let e = 0; e < this.edgeCount; e++) {
      const r = colors[e * 3];
      const g = colors[e * 3 + 1];
      const b = colors[e * 3 + 2];
      for (let v = 0; v < this.verticesPerEdge; v++) {
        const base = (e * this.verticesPerEdge + v) * 3;
        arr[base] = r;
        arr[base + 1] = g;
        arr[base + 2] = b;
      }
    }
    attr.needsUpdate = true;
  }

  /** hover 关联边 aHi=1，其余回 0；null 全 0。 */
  setHighlight(edgeIndices: Set<number> | null): void {
    this.highlighted = edgeIndices;
    const attr = this.geometry.getAttribute('aHi') as THREE.BufferAttribute;
    const arr = attr.array as Float32Array;
    arr.fill(0);
    if (edgeIndices) {
      for (const e of edgeIndices) {
        if (e >= 0 && e < this.edgeCount) {
          const base = e * this.verticesPerEdge;
          for (let v = 0; v < this.verticesPerEdge; v++) arr[base + v] = 1;
        }
      }
    }
    attr.needsUpdate = true;
  }

  get highlightedEdges(): Set<number> | null {
    return this.highlighted;
  }

  dispose(): void {
    this.geometry.dispose();
    this.material.dispose();
  }
}
