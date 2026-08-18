/**
 * model：G5 深空图谱 SoA 图模型（纯 TS，零 Vue/three 依赖）。
 *
 * 移植 fast-graph GraphModel（设计 §V12.8-1）：
 * - SoA 布局：positions/velocities Float32Array(3N)、degree Uint16、groupId Uint16、edges Int32Array(2E)
 * - docId↔index 双射；edgeTypes 保留每边类型（渲染配色）
 * - 边去重（同对无序只留先见类型）+ 自环/悬空边剔除
 * - groupId 按 doc_type 分组（groups 排序保证分配确定性）
 * - 确定性播种 mulberry32 + 球内体采样 r=(cbrt(N)*20+1)·cbrt(rand)
 */

export interface GraphNodeInput {
  docId: string;
  name: string;
  relPath: string;
  docType: string;
}

export interface GraphEdgeInput {
  /** 出向文档 doc_id。 */
  source: string;
  /** 入向文档 doc_id。 */
  target: string;
  /** explicit | entity | semantic */
  type: string;
}

export interface GraphModel {
  count: number;
  edgeCount: number;
  /** index → doc_id。 */
  docIds: string[];
  docIdToIndex: Map<string, number>;
  positions: Float32Array;
  velocities: Float32Array;
  degree: Uint16Array;
  groupId: Uint16Array;
  /** 去重后边（扁平索引对，无向）。 */
  edges: Int32Array;
  /** 每边类型（与 edges 对齐，长度 E）。 */
  edgeTypes: string[];
  /** groupId → doc_type（排序，确定性）。 */
  groups: string[];
  /** 节点展示名（index 对齐）。 */
  names: string[];
  /** 节点 vault 相对路径（index 对齐）。 */
  relPaths: string[];
}

/** 无向边键（N < 2^26 安全）。 */
function edgeKey(a: number, b: number): number {
  const lo = Math.min(a, b);
  const hi = Math.max(a, b);
  return lo * 67108864 + hi;
}

export function buildGraphModel(nodes: GraphNodeInput[], edges: GraphEdgeInput[]): GraphModel {
  const docIdToIndex = new Map<string, number>();
  const docIds: string[] = [];
  const names: string[] = [];
  const relPaths: string[] = [];
  const docTypes: string[] = [];
  for (const n of nodes) {
    if (docIdToIndex.has(n.docId)) continue; // 防御重复 doc_id
    docIdToIndex.set(n.docId, docIds.length);
    docIds.push(n.docId);
    names.push(n.name);
    relPaths.push(n.relPath);
    docTypes.push(n.docType);
  }

  const count = docIds.length;
  const degree = new Uint16Array(count);
  const seen = new Set<number>();
  const edgeList: number[] = [];
  const edgeTypes: string[] = [];

  for (const e of edges) {
    const si = docIdToIndex.get(e.source);
    const ti = docIdToIndex.get(e.target);
    if (si === undefined || ti === undefined) continue; // 悬空边
    if (si === ti) continue; // 自环
    const key = edgeKey(si, ti);
    if (seen.has(key)) continue;
    seen.add(key);
    edgeList.push(si, ti);
    edgeTypes.push(e.type);
    degree[si]++;
    degree[ti]++;
  }

  // 分组：doc_type 排序后编号（确定性：同输入集合同 groupId 分配）。
  const groups = [...new Set(docTypes)].sort();
  const groupIndex = new Map<string, number>(groups.map((g, i) => [g, i]));
  const groupId = new Uint16Array(count);
  for (let i = 0; i < count; i++) groupId[i] = groupIndex.get(docTypes[i])!;

  return {
    count,
    edgeCount: edgeList.length / 2,
    docIds,
    docIdToIndex,
    positions: new Float32Array(count * 3),
    velocities: new Float32Array(count * 3),
    degree,
    groupId,
    edges: Int32Array.from(edgeList),
    edgeTypes,
    groups,
    names,
    relPaths,
  };
}

/** M5：按 doc_type 组过滤（隐藏组节点排除 + 边级联排除）。空集合零开销原样返回。 */
export function filterGraphByGroups(
  nodes: GraphNodeInput[],
  edges: GraphEdgeInput[],
  hiddenGroups: ReadonlySet<string>,
): { nodes: GraphNodeInput[]; edges: GraphEdgeInput[] } {
  if (hiddenGroups.size === 0) return { nodes, edges };
  const kept = nodes.filter((n) => !hiddenGroups.has(n.docType));
  const keptIds = new Set(kept.map((n) => n.docId));
  const keptEdges = edges.filter((e) => keptIds.has(e.source) && keptIds.has(e.target));
  return { nodes: kept, edges: keptEdges };
}

/** mulberry32 — 可播种确定性 PRNG。 */
export function mulberry32(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/** 黄金角 ≈ 2.39996 rad（斐波那契球面分布用）。 */
const GOLDEN_ANGLE = Math.PI * (3 - Math.sqrt(5));

/** 确定性播种：斐波那契球面分布（节点在球壳上有序均匀排列，像地球仪）。
 *  半径 cbrt(N)*20+1，力导向从此球壳演化，最终形成立体知识球。 */
export function seedPositions(model: GraphModel, _seed: number): void {
  const n = model.count;
  const radius = Math.cbrt(n) * 20 + 1;
  for (let i = 0; i < n; i++) {
    // y 在 [-1, 1] 均匀分布，避免两极聚集
    const y = 1 - (i / (n - 1 || 1)) * 2;
    const radiusAtY = Math.sqrt(Math.max(0, 1 - y * y));
    const theta = GOLDEN_ANGLE * i;
    model.positions[i * 3] = radius * radiusAtY * Math.cos(theta);
    model.positions[i * 3 + 1] = radius * y;
    model.positions[i * 3 + 2] = radius * radiusAtY * Math.sin(theta);
  }
}
