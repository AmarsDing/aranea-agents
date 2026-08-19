/**
 * parking：V13-A1 孤立节点停泊环 + V13-B tier 目标壳层（纯 TS，零 Vue/three 依赖）。
 *
 * 背景：孤立节点（degree=0）不受弹簧约束，力导向后在播种球壳上随机散布，
 *       p90 半径被撑大 → 适应视角远、排布无规则（用户反馈）。
 * 对策：
 * - init 时 pinnedInit 冻结孤立节点（不参与物理积分）
 * - 收敛后 park 到主簇外围的规则圆环（XZ 平面等角间距，过密自动外扩同心环）
 * - tier 目标壳层半径表（stratify 力）：ultra 核 / super 中环 / regular 外环；
 *   孤立/末梢（degree≤1）= -1 不分层（末梢由弹簧挂在 hub 旁，强拉壳层会撕裂局部结构）
 */

/** tier → 目标半径占 outerRadius 比例（regular 外环 / super 中环 / ultra 核）。 */
export const TIER_RADIUS_RATIO: readonly number[] = [1.0, 0.55, 0.25];

/** 主簇外半径估计（stratify 壳层基准）：紧凑力学（linkStrength 0.07/dist 24/gravity 0.015）
 *  平衡下实测 p90 ≈ 6·cbrt(N)（123 连通节点实测 29），取 6.5 留少量裕量。
 *  注意：旧 14·cbrt 是 hairball 力参（0.05/30/0.011）标定，紧凑化后高估近 3 倍会稀释分层。 */
export function estimateOuterRadius(count: number): number {
  return Math.max(30, Math.cbrt(count) * 6.5);
}

/** 孤立节点（degree=0）索引收集。 */
export function isolatedIndices(degree: Uint16Array): Uint32Array {
  const out: number[] = [];
  for (let i = 0; i < degree.length; i++) if (degree[i] === 0) out.push(i);
  return Uint32Array.from(out);
}

/** init 冻结掩码（pinnedInit）：孤立节点 1，其余 0。无孤立节点时返回 null（零开销语义）。 */
export function buildPinnedInit(degree: Uint16Array): Uint8Array | null {
  let has = false;
  const out = new Uint8Array(degree.length);
  for (let i = 0; i < degree.length; i++) {
    if (degree[i] === 0) {
      out[i] = 1;
      has = true;
    }
  }
  return has ? out : null;
}

/**
 * V13-B tier 目标壳层半径表（注入 ForceEngine.tierTargetRadius / stratify 力）。
 * degree≤1（孤立/末梢）→ -1 不分层。
 */
export function buildTierTargetRadii(tiers: Uint8Array, degree: Uint16Array, outerRadius: number): Float32Array {
  const out = new Float32Array(tiers.length);
  for (let i = 0; i < tiers.length; i++) {
    out[i] = degree[i] <= 1 ? -1 : outerRadius * TIER_RADIUS_RATIO[tiers[i]];
  }
  return out;
}

/** 停泊环节点弧长下限（孤立节点无标签，弧长可小；值越小单环容量越大、环带越窄越像「行星环」）。 */
export const PARK_ARC_MIN = 10;
/** 同心环径向间距（紧凑环带，形如「行星环」围绕主簇）。 */
export const PARK_RING_SPACING = 12;
/** 停泊环半径 = 主簇 p90 半径 × 此系数（明显在主簇外，又不远到撑爆视野）。 */
export const PARK_RING_FACTOR = 1.4;

/**
 * 停泊环坐标生成（确定性）：
 * - XZ 平面（y=0）规则圆环，等角间距
 * - 单环相邻弧长 < PARK_ARC_MIN 时外扩同心环（径向间距 PARK_RING_SPACING）
 * - 每环相位错开黄金角，避免径向轮辐对齐
 * 返回扁平 xyz（长度 count*3）。
 */
export function parkingRingPositions(count: number, radius: number): Float32Array {
  const out = new Float32Array(count * 3);
  if (count === 0) return out;
  const golden = Math.PI * (3 - Math.sqrt(5));
  let idx = 0;
  let ring = 0;
  while (idx < count) {
    const r = radius + ring * PARK_RING_SPACING;
    const capacity = Math.max(1, Math.floor((2 * Math.PI * r) / PARK_ARC_MIN));
    const take = Math.min(capacity, count - idx);
    const phase = ring * golden;
    for (let k = 0; k < take; k++) {
      const a = phase + (k / take) * 2 * Math.PI;
      out[(idx + k) * 3] = r * Math.cos(a);
      out[(idx + k) * 3 + 1] = 0;
      out[(idx + k) * 3 + 2] = r * Math.sin(a);
    }
    idx += take;
    ring++;
  }
  return out;
}

export interface ClusterStats {
  cx: number;
  cy: number;
  cz: number;
  /** p90 距离分位半径（≥radiusFloor）。 */
  radius: number;
  /** 纳入统计的节点数。 */
  included: number;
}

/**
 * 主簇统计：质心 + p90 半径（exclude[i]=1 的节点剔除；全剔除/无掩码回退全量）。
 * 用途：zoomToFit 视野计算与停泊环半径都只看主簇，孤立/停泊的远点不再撑大 p90。
 */
export function clusterStatsP90(
  positions: Float32Array,
  count: number,
  exclude?: Uint8Array | null,
  radiusFloor = 20,
): ClusterStats {
  let included = 0;
  if (exclude) {
    for (let i = 0; i < count; i++) if (!exclude[i]) included++;
  }
  const useAll = !exclude || included === 0;
  const n = useAll ? count : included;
  let cx = 0;
  let cy = 0;
  let cz = 0;
  for (let i = 0; i < count; i++) {
    if (!useAll && exclude![i]) continue;
    cx += positions[i * 3];
    cy += positions[i * 3 + 1];
    cz += positions[i * 3 + 2];
  }
  cx /= n;
  cy /= n;
  cz /= n;
  const dists = new Float32Array(n);
  let k = 0;
  for (let i = 0; i < count; i++) {
    if (!useAll && exclude![i]) continue;
    const dx = positions[i * 3] - cx;
    const dy = positions[i * 3 + 1] - cy;
    const dz = positions[i * 3 + 2] - cz;
    dists[k++] = Math.sqrt(dx * dx + dy * dy + dz * dz);
  }
  dists.sort();
  const radius = Math.max(dists[Math.min(n - 1, Math.floor(n * 0.9))], radiusFloor);
  return { cx, cy, cz, radius, included: n };
}
