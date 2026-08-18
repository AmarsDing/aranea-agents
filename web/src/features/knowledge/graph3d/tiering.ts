/**
 * tiering：G5 深空图谱节点三层分级（jarvis-ui 蓝本，设计 §V12.8-1）。
 *
 * - supernode：degree ≥ 15（高度数 hub）
 * - ultranode：连接 ≥ 4 个不同 supernode（hub-of-hubs / nexus）
 * - 尺寸倍率 1.0/1.5/2.5；分层 charge -120/-200/-350 → 斥力倍率 1.0/1.67/2.92
 * - 邻接用 CSR（压缩稀疏行存储），构建期零逐节点数组分配
 */

export const TIER_REGULAR = 0;
export const TIER_SUPERNODE = 1;
export const TIER_ULTRANODE = 2;
export type NodeTier = typeof TIER_REGULAR | typeof TIER_SUPERNODE | typeof TIER_ULTRANODE;

export const SUPERNODE_MIN_DEGREE = 15;
export const ULTRANODE_MIN_SUPER_LINKS = 4;

/** 尺寸倍率（tier 索引）。 */
export const TIER_SIZE_MULT: readonly number[] = [1.0, 1.5, 2.5];
/** V13 标签候选层级权重（tier 索引）：常显标签优先给结构高层（ultra 4×/super 2×/regular 1×）。 */
export const TIER_LABEL_WEIGHT: readonly number[] = [1, 2, 4];
/** 分层 charge（jarvis-ui 原值，-120/-200/-350）→ 相对 regular 的斥力倍率。 */
export const TIER_CHARGE: readonly number[] = [-120, -200, -350];
export const TIER_CHARGE_SCALE: readonly number[] = TIER_CHARGE.map((c) => c / TIER_CHARGE[TIER_REGULAR]);

/** 三层分级：degree + supernode 邻居计数。 */
export function classifyTiers(degree: Uint16Array, edges: Int32Array): Uint8Array {
  const count = degree.length;
  const tiers = new Uint8Array(count);

  // 第一遍：supernode = degree ≥ 阈值
  const isSuper = new Uint8Array(count);
  for (let i = 0; i < count; i++) {
    if (degree[i] >= SUPERNODE_MIN_DEGREE) isSuper[i] = 1;
  }

  // CSR 邻接（无向）
  const offsets = new Uint32Array(count + 1);
  for (let e = 0; e < edges.length; e += 2) {
    offsets[edges[e] + 1]++;
    offsets[edges[e + 1] + 1]++;
  }
  for (let i = 0; i < count; i++) offsets[i + 1] += offsets[i];
  const neighbors = new Int32Array(offsets[count]);
  const cursor = new Uint32Array(count);
  for (let i = 0; i < count; i++) cursor[i] = offsets[i];
  for (let e = 0; e < edges.length; e += 2) {
    const a = edges[e];
    const b = edges[e + 1];
    neighbors[cursor[a]++] = b;
    neighbors[cursor[b]++] = a;
  }

  // 第二遍：supernode 的不同 supernode 邻居 ≥ 阈值 → ultranode
  for (let i = 0; i < count; i++) {
    if (!isSuper[i]) {
      tiers[i] = TIER_REGULAR;
      continue;
    }
    let superLinks = 0;
    // degree 可能 > 不同邻居数（模型已去重，邻居即不同），逐个计数
    const start = offsets[i];
    const end = offsets[i + 1];
    for (let j = start; j < end; j++) {
      if (isSuper[neighbors[j]]) superLinks++;
    }
    tiers[i] = superLinks >= ULTRANODE_MIN_SUPER_LINKS ? TIER_ULTRANODE : TIER_SUPERNODE;
  }
  return tiers;
}

/** tier → per-node 斥力倍率数组（注入 ForceEngine.chargeScale）。 */
export function tierChargeScales(tiers: Uint8Array): Float32Array {
  const out = new Float32Array(tiers.length);
  for (let i = 0; i < tiers.length; i++) out[i] = TIER_CHARGE_SCALE[tiers[i]];
  return out;
}
