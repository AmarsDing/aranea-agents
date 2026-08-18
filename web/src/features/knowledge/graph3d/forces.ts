/**
 * forces：G5 深空图谱物理引擎（主线程/Worker 共用同一代码）。
 *
 * 移植 fast-graph PhysicsEngine（设计 §V12.8-1）：
 * - 5 力：BH 斥力(repulsion=30,theta=0.8) + 弹簧(0.05/30) + 簇凝聚(0.08)
 *         + 簇分离(100·count/d²) + 向心力(0.011)
 * - 显式 Euler damping=0.9；maxStep 位移钳制(≤linkDistance) 防 hub 发散
 * - alphaDecay=0.0228，alphaMin=0.005
 * - 扩展：可选 chargeScale（per-node 斥力倍率，tiering 分层 charge 注入点）
 */
import { Octree } from './octree';

export interface ForceParams {
  /** 斥力强度（>0）。 */
  repulsion: number;
  /** 弹簧强度。 */
  linkStrength: number;
  /** 弹簧理想距离（兼 maxStep 钳制值）。 */
  linkDistance: number;
  /** 向心力强度。 */
  gravity: number;
  /** 速度阻尼（0~1，每 tick 乘）。 */
  damping: number;
  /** Barnes-Hut 开张判据。 */
  theta: number;
  /** 簇凝聚强度（同组节点向组中心）。 */
  groupCohesion: number;
  /** 簇分离强度（组中心间 Coulomb 斥力）。 */
  groupSeparation: number;
  /** M2 星系盘：核心引力强度（0=关闭）。软化径向，形成致密核。 */
  coreGravity: number;
  /** M2 星系盘：盘压扁强度（0=关闭）。Y 轴单向向心，压向 XZ 盘面。 */
  discFlatten: number;
  /** M2 星系盘：螺旋切向力强度（0=关闭）。XZ 平面绕 Y 轴，径向包络中心弱边缘饱和。 */
  spiralSwirl: number;
  /** V13-B：tier 径向分层力强度（0=关闭）。按 tierTargetRadius 把节点拉向目标半径壳层。 */
  stratify: number;
}

export const FORCE_DEFAULTS: ForceParams = {
  repulsion: 30,
  // V13-A3 主簇紧凑化：弹簧更强/理想距更短/向心更强（原 0.05/30/0.011 hairball 偏散）
  linkStrength: 0.07,
  linkDistance: 24,
  gravity: 0.015,
  damping: 0.9,
  theta: 0.8,
  groupCohesion: 0.08,
  groupSeparation: 100,
  coreGravity: 0,
  discFlatten: 0,
  spiralSwirl: 0,
  // V13-B：默认开（仅在注入 tierTargetRadius 时生效，否则零作用）
  stratify: 0.02,
};

/** M2 星系盘布局预设（布局切换 = setParams(GALAXY_FORCE_PARAMS) + reheat）。 */
export const GALAXY_FORCE_PARAMS: Partial<ForceParams> = {
  coreGravity: 0.08,
  discFlatten: 0.12,
  spiralSwirl: 0.02,
  gravity: 0.004, // 默认向心减弱（核心引力接管）
  groupSeparation: 60, // 簇间更紧凑（盘内悬臂簇）
  stratify: 0, // 盘内由 coreGravity/压扁/旋臂接管，径向分层关闭
};

const ALPHA_DECAY = 0.0228;
const ALPHA_TARGET = 0;

export interface ForceEngineOpts {
  count: number;
  edges: Int32Array;
  positions: Float32Array;
  params: ForceParams;
  groupId?: Uint16Array;
  /** per-node 斥力倍率（分层 charge；缺省全 1）。 */
  chargeScale?: Float32Array;
  /** V13-B：per-node 目标半径壳层（stratify>0 时生效；<0 = 该节点不分层，如孤立/末梢）。 */
  tierTargetRadius?: Float32Array;
  /** V13-A1：初始即钉住的节点（孤立节点先冻在播种球壳上，收敛后统一停泊环）。 */
  pinnedInit?: Uint8Array;
}

export class ForceEngine {
  alpha = 1;
  readonly alphaMin = 0.005;

  private count: number;
  private edges: Int32Array;
  private pos: Float32Array;
  private vel: Float32Array;
  private params: ForceParams;
  private force: Float32Array;
  private pinned: Uint8Array;
  private scratch = new Float32Array(3);
  private tree: Octree;
  private chargeScale: Float32Array | null;
  private tierTargetRadius: Float32Array | null;

  private groupId: Uint16Array;
  private numGroups: number;
  private gSum: Float32Array;
  private gCount: Float32Array;
  private gCen: Float32Array;
  private gSep: Float32Array;

  constructor(opts: ForceEngineOpts) {
    this.count = opts.count;
    this.edges = opts.edges;
    this.pos = opts.positions;
    this.vel = new Float32Array(opts.count * 3);
    this.params = { ...opts.params };
    this.force = new Float32Array(opts.count * 3);
    this.pinned = new Uint8Array(opts.count);
    this.tree = new Octree(opts.count);
    this.chargeScale = opts.chargeScale ?? null;
    this.tierTargetRadius = opts.tierTargetRadius ?? null;
    if (opts.pinnedInit) this.pinned.set(opts.pinnedInit);

    this.groupId = opts.groupId ?? new Uint16Array(opts.count);
    let maxG = 0;
    for (let i = 0; i < opts.count; i++) if (this.groupId[i] > maxG) maxG = this.groupId[i];
    this.numGroups = maxG + 1;
    this.gSum = new Float32Array(this.numGroups * 3);
    this.gCount = new Float32Array(this.numGroups);
    this.gCen = new Float32Array(this.numGroups * 3);
    this.gSep = new Float32Array(this.numGroups * 3);
  }

  get positions(): Float32Array {
    return this.pos;
  }

  /** alpha < alphaMin 即收敛（ Worker 据此自停）。 */
  get settled(): boolean {
    return this.alpha < this.alphaMin;
  }

  setParams(p: Partial<ForceParams>): void {
    this.params = { ...this.params, ...p };
  }

  pin(i: number, x: number, y: number, z: number): void {
    this.pinned[i] = 1;
    this.pos[i * 3] = x;
    this.pos[i * 3 + 1] = y;
    this.pos[i * 3 + 2] = z;
    this.vel[i * 3] = 0;
    this.vel[i * 3 + 1] = 0;
    this.vel[i * 3 + 2] = 0;
    this.reheat();
  }

  unpin(i: number): void {
    this.pinned[i] = 0;
  }

  /** V13-A1 停泊：钉住并写坐标，但不 reheat（收敛后调用不重启物理，由调用方补一次位置广播）。 */
  park(i: number, x: number, y: number, z: number): void {
    this.pinned[i] = 1;
    this.pos[i * 3] = x;
    this.pos[i * 3 + 1] = y;
    this.pos[i * 3 + 2] = z;
    this.vel[i * 3] = 0;
    this.vel[i * 3 + 1] = 0;
    this.vel[i * 3 + 2] = 0;
  }

  reheat(): void {
    this.alpha = 1;
  }

  tick(): void {
    const { repulsion, linkStrength, linkDistance, gravity, damping, theta, groupCohesion, groupSeparation } =
      this.params;
    const f = this.force;
    f.fill(0);

    // 1) BH 斥力（可选 per-node chargeScale 倍率）
    this.tree.rebuild(this.pos, this.count);
    const s = this.scratch;
    const cs = this.chargeScale;
    for (let i = 0; i < this.count; i++) {
      this.tree.computeForce(i, theta, repulsion, s);
      const k = cs ? cs[i] : 1;
      f[i * 3] += s[0] * k;
      f[i * 3 + 1] += s[1] * k;
      f[i * 3 + 2] += s[2] * k;
    }

    // 2) 弹簧（边）
    for (let e = 0; e < this.edges.length; e += 2) {
      const a = this.edges[e];
      const b = this.edges[e + 1];
      const dx = this.pos[b * 3] - this.pos[a * 3];
      const dy = this.pos[b * 3 + 1] - this.pos[a * 3 + 1];
      const dz = this.pos[b * 3 + 2] - this.pos[a * 3 + 2];
      const d = Math.sqrt(dx * dx + dy * dy + dz * dz) || 1e-3;
      const k = (linkStrength * (d - linkDistance)) / d;
      const fx = dx * k;
      const fy = dy * k;
      const fz = dz * k;
      f[a * 3] += fx;
      f[a * 3 + 1] += fy;
      f[a * 3 + 2] += fz;
      f[b * 3] -= fx;
      f[b * 3 + 1] -= fy;
      f[b * 3 + 2] -= fz;
    }

    // 2.5) 簇力：同组凝聚，组间分离
    const doCohesion = groupCohesion > 0;
    const doSeparation = groupSeparation > 0 && this.numGroups > 1;
    if (doCohesion || doSeparation) {
      const G = this.numGroups;
      this.gSum.fill(0);
      this.gCount.fill(0);
      for (let i = 0; i < this.count; i++) {
        const g = this.groupId[i];
        this.gSum[g * 3] += this.pos[i * 3];
        this.gSum[g * 3 + 1] += this.pos[i * 3 + 1];
        this.gSum[g * 3 + 2] += this.pos[i * 3 + 2];
        this.gCount[g]++;
      }
      for (let g = 0; g < G; g++) {
        const c = this.gCount[g] || 1;
        this.gCen[g * 3] = this.gSum[g * 3] / c;
        this.gCen[g * 3 + 1] = this.gSum[g * 3 + 1] / c;
        this.gCen[g * 3 + 2] = this.gSum[g * 3 + 2] / c;
      }
      if (doSeparation) {
        this.gSep.fill(0);
        for (let g = 0; g < G; g++) {
          if (this.gCount[g] === 0) continue;
          for (let h = 0; h < G; h++) {
            if (h === g || this.gCount[h] === 0) continue;
            let dx = this.gCen[g * 3] - this.gCen[h * 3];
            let dy = this.gCen[g * 3 + 1] - this.gCen[h * 3 + 1];
            let dz = this.gCen[g * 3 + 2] - this.gCen[h * 3 + 2];
            let d2 = dx * dx + dy * dy + dz * dz;
            if (d2 < 1e-3) {
              dx = g - h;
              dy = 0;
              dz = 0;
              d2 = (g - h) * (g - h) || 1e-3;
            }
            const dd = Math.sqrt(d2);
            const fmag = (groupSeparation * this.gCount[h]) / d2;
            this.gSep[g * 3] += (dx / dd) * fmag;
            this.gSep[g * 3 + 1] += (dy / dd) * fmag;
            this.gSep[g * 3 + 2] += (dz / dd) * fmag;
          }
        }
      }
      for (let i = 0; i < this.count; i++) {
        const g = this.groupId[i];
        if (doCohesion) {
          f[i * 3] += (this.gCen[g * 3] - this.pos[i * 3]) * groupCohesion;
          f[i * 3 + 1] += (this.gCen[g * 3 + 1] - this.pos[i * 3 + 1]) * groupCohesion;
          f[i * 3 + 2] += (this.gCen[g * 3 + 2] - this.pos[i * 3 + 2]) * groupCohesion;
        }
        if (doSeparation) {
          f[i * 3] += this.gSep[g * 3];
          f[i * 3 + 1] += this.gSep[g * 3 + 1];
          f[i * 3 + 2] += this.gSep[g * 3 + 2];
        }
      }
    }

    // 3) 向心力 + 星系盘三力 + 显式 Euler 积分（maxStep 位移钳制防 hub 发散）
    const { coreGravity, discFlatten, spiralSwirl, stratify } = this.params;
    const galaxyMode = coreGravity > 0 || discFlatten > 0 || spiralSwirl > 0;
    const ttr = stratify > 0 ? this.tierTargetRadius : null;
    const maxStep = linkDistance;
    const maxStep2 = maxStep * maxStep;
    for (let i = 0; i < this.count; i++) {
      if (this.pinned[i]) continue;
      const ix = i * 3;
      const iy = ix + 1;
      const iz = ix + 2;
      f[ix] -= this.pos[ix] * gravity;
      f[iy] -= this.pos[iy] * gravity;
      f[iz] -= this.pos[iz] * gravity;

      // V13-B：tier 径向分层——沿径向拉向目标壳层半径（ultra 核/super 中环/regular 外环）
      if (ttr && ttr[i] >= 0) {
        const px = this.pos[ix];
        const py = this.pos[iy];
        const pz = this.pos[iz];
        const r = Math.sqrt(px * px + py * py + pz * pz) || 1e-3;
        const k = ((ttr[i] - r) * stratify) / r;
        f[ix] += px * k;
        f[iy] += py * k;
        f[iz] += pz * k;
      }

      if (galaxyMode) {
        const px = this.pos[ix];
        const py = this.pos[iy];
        const pz = this.pos[iz];
        if (coreGravity > 0) {
          // 软化径向引力：中心不过冲，远处弱于线性（致密核成形）
          const r = Math.sqrt(px * px + py * py + pz * pz) || 1e-3;
          const k = coreGravity / (1 + r * 0.02);
          f[ix] -= px * k;
          f[iy] -= py * k;
          f[iz] -= pz * k;
        }
        if (discFlatten > 0) {
          f[iy] -= py * discFlatten; // Y 轴压向 XZ 盘面
        }
        if (spiralSwirl > 0) {
          const rxz = Math.sqrt(px * px + pz * pz);
          if (rxz > 1e-3) {
            const envelope = rxz / (rxz + 40); // 中心弱、边缘饱和
            const s = (spiralSwirl * envelope) / rxz;
            f[ix] += -pz * s;
            f[iz] += px * s;
          }
        }
      }

      let vx = (this.vel[ix] + f[ix] * this.alpha) * damping;
      let vy = (this.vel[iy] + f[iy] * this.alpha) * damping;
      let vz = (this.vel[iz] + f[iz] * this.alpha) * damping;

      const sp2 = vx * vx + vy * vy + vz * vz;
      if (sp2 > maxStep2) {
        const scale = maxStep / Math.sqrt(sp2);
        vx *= scale;
        vy *= scale;
        vz *= scale;
      }

      this.vel[ix] = vx;
      this.vel[iy] = vy;
      this.vel[iz] = vz;
      this.pos[ix] += vx;
      this.pos[iy] += vy;
      this.pos[iz] += vz;
    }

    this.alpha += (ALPHA_TARGET - this.alpha) * ALPHA_DECAY;
  }
}
