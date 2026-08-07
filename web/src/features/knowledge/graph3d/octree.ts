/**
 * octree：G5 深空图谱 typed-array 八叉树（Barnes-Hut 质心近似）。
 *
 * 1:1 移植 fast-graph Octree（设计 §V12.8-1）：
 * - typed-array 池（Float32 8/cell + Int32 9/cell，容量 16N 倍增）
 * - 显式栈迭代（无递归），质心除法延迟到查询
 * - rebuild 复用池，热循环零分配
 */

// Float32Array 布局：0:cx 1:cy 2:cz 3:half 4:mass 5:comx 6:comy 7:comz
const FLOAT_STRIDE = 8;
// Int32Array 布局：0:body(-1=无) 1..8:children(-1=null)
const INT_STRIDE = 9;
// 节点数最坏 8*(N-1)+1 ≈ 8N，预留 16N
const CAPACITY_FACTOR = 16;

export class Octree {
  private floats: Float32Array;
  private ints: Int32Array;
  private capacity: number;
  private size = 0;

  private pos!: Float32Array;

  private stack: Int32Array;

  constructor(maxNodes: number) {
    this.capacity = Math.max(maxNodes * CAPACITY_FACTOR, 64);
    this.floats = new Float32Array(this.capacity * FLOAT_STRIDE);
    this.ints = new Int32Array(this.capacity * INT_STRIDE);
    this.stack = new Int32Array(this.capacity);
  }

  /** tick 开始调用：重置并把全部节点插入。 */
  rebuild(positions: Float32Array, count: number): void {
    this.pos = positions;
    this.size = 0;
    if (count === 0) return;

    let min = Infinity;
    let max = -Infinity;
    for (let i = 0; i < count * 3; i++) {
      const v = positions[i];
      if (v < min) min = v;
      if (v > max) max = v;
    }
    if (!isFinite(min)) {
      min = -1;
      max = 1;
    }
    const center = (min + max) / 2;
    const half = Math.max((max - min) / 2, 1e-3) + 1e-3;

    const root = this.allocCell(center, center, center, half);
    for (let i = 0; i < count; i++) this.insert(root, i);
  }

  private allocCell(cx: number, cy: number, cz: number, half: number): number {
    const idx = this.size;
    if (idx >= this.capacity) this.grow();
    this.size++;
    const fi = idx * FLOAT_STRIDE;
    this.floats[fi] = cx;
    this.floats[fi + 1] = cy;
    this.floats[fi + 2] = cz;
    this.floats[fi + 3] = half;
    this.floats[fi + 4] = 0;
    this.floats[fi + 5] = 0;
    this.floats[fi + 6] = 0;
    this.floats[fi + 7] = 0;
    const ii = idx * INT_STRIDE;
    this.ints[ii] = -1;
    for (let k = 1; k <= 8; k++) this.ints[ii + k] = -1;
    return idx;
  }

  private grow(): void {
    const newCap = this.capacity * 2;
    const nf = new Float32Array(newCap * FLOAT_STRIDE);
    nf.set(this.floats);
    const ni = new Int32Array(newCap * INT_STRIDE);
    ni.set(this.ints);
    const ns = new Int32Array(newCap);
    ns.set(this.stack);
    this.floats = nf;
    this.ints = ni;
    this.stack = ns;
    this.capacity = newCap;
  }

  private octant(cellIdx: number, x: number, y: number, z: number): number {
    const fi = cellIdx * FLOAT_STRIDE;
    return (x >= this.floats[fi] ? 1 : 0) | (y >= this.floats[fi + 1] ? 2 : 0) | (z >= this.floats[fi + 2] ? 4 : 0);
  }

  private childCell(parentIdx: number, oct: number): number {
    const fi = parentIdx * FLOAT_STRIDE;
    const h = this.floats[fi + 3] / 2;
    const cx = this.floats[fi] + (oct & 1 ? h : -h);
    const cy = this.floats[fi + 1] + (oct & 2 ? h : -h);
    const cz = this.floats[fi + 2] + (oct & 4 ? h : -h);
    return this.allocCell(cx, cy, cz, h);
  }

  private insert(cellIdx: number, body: number): void {
    const x = this.pos[body * 3];
    const y = this.pos[body * 3 + 1];
    const z = this.pos[body * 3 + 2];
    const fi = cellIdx * FLOAT_STRIDE;
    const ii = cellIdx * INT_STRIDE;

    this.floats[fi + 4] += 1;
    this.floats[fi + 5] += x;
    this.floats[fi + 6] += y;
    this.floats[fi + 7] += z;

    const currentBody = this.ints[ii];
    let hasChildren = false;
    for (let k = 1; k <= 8; k++) {
      if (this.ints[ii + k] !== -1) {
        hasChildren = true;
        break;
      }
    }

    if (currentBody === -1 && !hasChildren) {
      this.ints[ii] = body;
      return;
    }
    if (!hasChildren) {
      const existing = currentBody;
      this.ints[ii] = -1;
      this.placeInChild(cellIdx, existing);
    }
    if (this.floats[fi + 3] < 1e-4) return; // 防过细分
    this.placeInChild(cellIdx, body);
  }

  private placeInChild(cellIdx: number, body: number): void {
    const x = this.pos[body * 3];
    const y = this.pos[body * 3 + 1];
    const z = this.pos[body * 3 + 2];
    const oct = this.octant(cellIdx, x, y, z);
    const ii = cellIdx * INT_STRIDE;
    let childIdx = this.ints[ii + 1 + oct];
    if (childIdx === -1) {
      childIdx = this.childCell(cellIdx, oct);
      // grow() 后 this.ints 引用可能变化，重算 ii
      const ii2 = cellIdx * INT_STRIDE;
      this.ints[ii2 + 1 + oct] = childIdx;
    }
    this.insert(childIdx, body);
  }

  computeForce(i: number, theta: number, repulsion: number, out: Float32Array): void {
    out[0] = 0;
    out[1] = 0;
    out[2] = 0;
    if (this.size === 0) return;
    this.accumulate(0, i, theta, repulsion, out);
  }

  private accumulate(root: number, i: number, theta: number, repulsion: number, out: Float32Array): void {
    const px = this.pos[i * 3];
    const py = this.pos[i * 3 + 1];
    const pz = this.pos[i * 3 + 2];
    let top = 0;
    this.stack[top++] = root;

    while (top > 0) {
      const cellIdx = this.stack[--top];
      const fi = cellIdx * FLOAT_STRIDE;
      const ii = cellIdx * INT_STRIDE;

      const mass = this.floats[fi + 4];
      if (mass === 0) continue;

      const body = this.ints[ii];
      if (body === i && mass === 1) continue; // 自身

      const mx = this.floats[fi + 5] / mass;
      const my = this.floats[fi + 6] / mass;
      const mz = this.floats[fi + 7] / mass;
      let dx = px - mx;
      let dy = py - my;
      let dz = pz - mz;
      let dist2 = dx * dx + dy * dy + dz * dz;
      if (dist2 < 1e-6) {
        dx = 1e-3;
        dy = 0;
        dz = 0;
        dist2 = 1e-6;
      }
      const dist = Math.sqrt(dist2);

      const half = this.floats[fi + 3];
      let isLeaf = true;
      for (let k = 1; k <= 8; k++) {
        if (this.ints[ii + k] !== -1) {
          isLeaf = false;
          break;
        }
      }

      if (isLeaf || (half * 2) / dist < theta) {
        const f = (repulsion * mass) / dist2;
        out[0] += (dx / dist) * f;
        out[1] += (dy / dist) * f;
        out[2] += (dz / dist) * f;
      } else {
        for (let k = 1; k <= 8; k++) {
          const c = this.ints[ii + k];
          if (c !== -1) {
            if (top >= this.stack.length) {
              const ns = new Int32Array(this.stack.length * 2);
              ns.set(this.stack);
              this.stack = ns;
            }
            this.stack[top++] = c;
          }
        }
      }
    }
  }
}
