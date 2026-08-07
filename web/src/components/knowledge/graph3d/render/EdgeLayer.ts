/**
 * EdgeLayer：G5 深空图谱边层（微弯 Bezier，设计 §V12.8-1 C-2）。
 *
 * - 每边 6 段 QuadraticBezier：控制点 = mid + 垂直基(cosθ·u+sinθ·v)·bow(0.3·len)，
 *   θ=hash01("a->b")·2π（稳定定向，双向/多边可区分）
 * - 单 LineSegments + vertexColors：rest=边类型色×0.32，hover 关联边=×0.9 瞬时换色
 * - 加法混合 + depthWrite:false（深空发光叠层）
 */
import * as THREE from 'three';

export const EDGE_SEGMENTS = 6;
export const EDGE_BOW = 0.3;
/** rest 亮度系数。 */
export const EDGE_REST_DIM = 0.32;
/** hover 关联边亮度系数。 */
export const EDGE_HOVER_BOOST = 0.9;

/** fnv-1a 字符串哈希 → [0,1)（边弯向稳定播种）。 */
export function hash01(key: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < key.length; i++) {
    h ^= key.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return (h >>> 0) / 4294967296;
}

const VERTS_PER_EDGE = EDGE_SEGMENTS * 2;

export class EdgeLayer {
  readonly object: THREE.LineSegments;
  private readonly geometry: THREE.BufferGeometry;
  private readonly material: THREE.LineBasicMaterial;
  private readonly edges: Int32Array;
  private readonly edgeCount: number;
  /** 每边基础色（3E，未乘系数）。 */
  private readonly baseColors: Float32Array;
  private highlighted: Set<number> | null = null;

  constructor(edges: Int32Array, edgeColors: Float32Array) {
    this.edges = edges;
    this.edgeCount = edges.length / 2;
    this.baseColors = edgeColors.slice();

    this.geometry = new THREE.BufferGeometry();
    const posAttr = new THREE.BufferAttribute(new Float32Array(this.edgeCount * VERTS_PER_EDGE * 3), 3);
    posAttr.setUsage(THREE.DynamicDrawUsage);
    this.geometry.setAttribute('position', posAttr);
    const colAttr = new THREE.BufferAttribute(new Float32Array(this.edgeCount * VERTS_PER_EDGE * 3), 3);
    this.geometry.setAttribute('color', colAttr);

    this.material = new THREE.LineBasicMaterial({
      vertexColors: true,
      blending: THREE.AdditiveBlending,
      transparent: true,
      depthWrite: false,
    });
    this.object = new THREE.LineSegments(this.geometry, this.material);
    this.object.frustumCulled = false;
    this.applyColors();
  }

  /** 全量重写顶点（布局 tick / 子图切换）。 */
  updatePositions(positions: Float32Array): void {
    for (let e = 0; e < this.edgeCount; e++) this.writeEdge(e, positions);
    (this.geometry.getAttribute('position') as THREE.BufferAttribute).needsUpdate = true;
  }

  /** 单边重写（拖拽优化：只重写关联边）。 */
  updateEdgesFor(edgeIndices: Iterable<number>, positions: Float32Array): void {
    for (const e of edgeIndices) this.writeEdge(e, positions);
    (this.geometry.getAttribute('position') as THREE.BufferAttribute).needsUpdate = true;
  }

  /** hover 关联边换色（×0.9），其余回 rest（×0.32）；null 全 rest。 */
  setHighlight(edgeIndices: Set<number> | null): void {
    this.highlighted = edgeIndices;
    this.applyColors();
  }

  get highlightedEdges(): Set<number> | null {
    return this.highlighted;
  }

  private applyColors(): void {
    const attr = this.geometry.getAttribute('color') as THREE.BufferAttribute;
    const arr = attr.array as Float32Array;
    for (let e = 0; e < this.edgeCount; e++) {
      const k = this.highlighted?.has(e) ? EDGE_HOVER_BOOST : EDGE_REST_DIM;
      const r = this.baseColors[e * 3] * k;
      const g = this.baseColors[e * 3 + 1] * k;
      const b = this.baseColors[e * 3 + 2] * k;
      const off = e * VERTS_PER_EDGE * 3;
      for (let v = 0; v < VERTS_PER_EDGE; v++) {
        arr[off + v * 3] = r;
        arr[off + v * 3 + 1] = g;
        arr[off + v * 3 + 2] = b;
      }
    }
    attr.needsUpdate = true;
  }

  private writeEdge(e: number, positions: Float32Array): void {
    const a = this.edges[e * 2];
    const b = this.edges[e * 2 + 1];
    const ax = positions[a * 3];
    const ay = positions[a * 3 + 1];
    const az = positions[a * 3 + 2];
    const bx = positions[b * 3];
    const by = positions[b * 3 + 1];
    const bz = positions[b * 3 + 2];

    let dx = bx - ax;
    let dy = by - ay;
    let dz = bz - az;
    const len = Math.sqrt(dx * dx + dy * dy + dz * dz);
    const attr = this.geometry.getAttribute('position') as THREE.BufferAttribute;
    const arr = attr.array as Float32Array;
    const off = e * VERTS_PER_EDGE * 3;

    if (len < 1e-6) {
      // 端点重合：退化为零长线段
      for (let v = 0; v < VERTS_PER_EDGE; v++) {
        arr[off + v * 3] = ax;
        arr[off + v * 3 + 1] = ay;
        arr[off + v * 3 + 2] = az;
      }
      return;
    }
    dx /= len;
    dy /= len;
    dz /= len;

    // 垂直于边的正交基（选与 dir 最不平行的世界轴叉积）
    const adx = Math.abs(dx);
    const ady = Math.abs(dy);
    const adz = Math.abs(dz);
    let rx = 0;
    let ry = 0;
    let rz = 0;
    if (adx <= ady && adx <= adz) rx = 1;
    else if (ady <= adz) ry = 1;
    else rz = 1;
    // u = normalize(dir × ref)
    let ux = dy * rz - dz * ry;
    let uy = dz * rx - dx * rz;
    let uz = dx * ry - dy * rx;
    const ul = Math.sqrt(ux * ux + uy * uy + uz * uz) || 1;
    ux /= ul;
    uy /= ul;
    uz /= ul;
    // v = dir × u
    const vx = dy * uz - dz * uy;
    const vy = dz * ux - dx * uz;
    const vz = dx * uy - dy * ux;

    const theta = hash01(`${a}->${b}`) * Math.PI * 2;
    const bow = EDGE_BOW * len;
    const ox = (ux * Math.cos(theta) + vx * Math.sin(theta)) * bow;
    const oy = (uy * Math.cos(theta) + vy * Math.sin(theta)) * bow;
    const oz = (uz * Math.cos(theta) + vz * Math.sin(theta)) * bow;

    const cx = (ax + bx) / 2 + ox;
    const cy = (ay + by) / 2 + oy;
    const cz = (az + bz) / 2 + oz;

    // QuadraticBezier 采样 SEGMENTS+1 个点，连 SEGMENTS 段
    let prevX = ax;
    let prevY = ay;
    let prevZ = az;
    for (let s = 1; s <= EDGE_SEGMENTS; s++) {
      const t = s / EDGE_SEGMENTS;
      const mt = 1 - t;
      const px = mt * mt * ax + 2 * mt * t * cx + t * t * bx;
      const py = mt * mt * ay + 2 * mt * t * cy + t * t * by;
      const pz = mt * mt * az + 2 * mt * t * cz + t * t * bz;
      const vo = off + (s - 1) * 6;
      arr[vo] = prevX;
      arr[vo + 1] = prevY;
      arr[vo + 2] = prevZ;
      arr[vo + 3] = px;
      arr[vo + 4] = py;
      arr[vo + 5] = pz;
      prevX = px;
      prevY = py;
      prevZ = pz;
    }
  }

  dispose(): void {
    this.geometry.dispose();
    this.material.dispose();
  }
}
