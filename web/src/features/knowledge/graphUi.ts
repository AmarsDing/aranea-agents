/**
 * graphUi：G4 3D 知识图谱纯函数（无组件/three.js 依赖，可单测）。
 *
 * - 边类型配色：explicit=primary 蓝 / entity=紫 / semantic=青（V12.7）
 * - 节点配色：doc_type 调色板哈希（稳定同色）
 * - 节点大小：连接度映射（sqrt 压缩长尾）
 * - 渲染裁剪：>2k 节点默认仅渲染有连接节点（「显示孤立节点」开关放开）
 * - 一跳邻居：hover 高亮集计算
 */
import type { CollectionGraphEdge, CollectionGraphNode } from './types';

/** 图谱边类型（chips 过滤项；与后端 link_type 一致）。 */
export const GRAPH_LINK_TYPES = ['explicit', 'entity', 'semantic'] as const;

/** 边类型配色（hex，供 three.js 材质）。 */
export function graphLinkColor(type: string): string {
  switch (type) {
    case 'explicit':
      return '#4c8dff'; // primary 蓝
    case 'entity':
      return '#a066d3'; // 紫
    case 'semantic':
      return '#3bbdbd'; // 青
    default:
      return '#8a93a6'; // 未知类型灰
  }
}

/** doc_type 调色板（稳定哈希取色，同类同色）。 */
const DOC_TYPE_PALETTE = [
  '#4c8dff', // 蓝
  '#3bbdbd', // 青
  '#a066d3', // 紫
  '#d3864a', // 橙
  '#c45b8a', // 品红
  '#7fb069', // 绿
  '#d3b04a', // 金
  '#5bc4e5', // 天青
] as const;

const DOC_TYPE_FALLBACK = '#8a93a6';

/** 节点配色：doc_type 稳定哈希 → 调色板；空类型灰。 */
export function graphDocTypeColor(docType: string): string {
  const key = docType.trim().toLowerCase();
  if (!key) return DOC_TYPE_FALLBACK;
  let h = 0;
  for (let i = 0; i < key.length; i++) {
    h = (h * 31 + key.charCodeAt(i)) | 0;
  }
  return DOC_TYPE_PALETTE[Math.abs(h) % DOC_TYPE_PALETTE.length];
}

/** 节点大小（three.js nodeVal 体积）：sqrt 压缩长尾，孤立节点保持可见下限。 */
export function graphNodeVal(degree: number): number {
  const d = Math.max(0, degree);
  return 1.5 + Math.sqrt(d) * 1.5;
}

/** 渲染规模阈值：超过则默认隐藏孤立节点（V12.7 规模条款）。 */
export const GRAPH_ISOLATED_HIDE_THRESHOLD = 2000;

export interface RenderGraph {
  nodes: CollectionGraphNode[];
  edges: CollectionGraphEdge[];
  /** 被隐藏的孤立节点数（提示条用）。 */
  hiddenIsolated: number;
}

/** 渲染裁剪：节点总数超阈值且未开「显示孤立节点」时，仅渲染有连接节点
 *  （边端点恒在节点集内——后端已剔除悬空边，裁剪后保持一致）。 */
export function buildRenderGraph(
  nodes: CollectionGraphNode[],
  edges: CollectionGraphEdge[],
  showIsolated: boolean,
): RenderGraph {
  if (showIsolated || nodes.length <= GRAPH_ISOLATED_HIDE_THRESHOLD) {
    return { nodes, edges, hiddenIsolated: 0 };
  }
  const connected = new Set<string>();
  for (const e of edges) {
    connected.add(e.source);
    connected.add(e.target);
  }
  const kept = nodes.filter((n) => connected.has(n.doc_id));
  const keptIds = new Set(kept.map((n) => n.doc_id));
  return {
    nodes: kept,
    edges: edges.filter((e) => keptIds.has(e.source) && keptIds.has(e.target)),
    hiddenIsolated: nodes.length - kept.length,
  };
}

/** 节点列表排序：连接度降序，同度按名称升序（稳定）。 */
export function sortedGraphNodes(nodes: CollectionGraphNode[]): CollectionGraphNode[] {
  return [...nodes].sort((a, b) => b.degree - a.degree || a.name.localeCompare(b.name));
}

/** 节点搜索过滤（名称/路径/doc_type 子串，大小写不敏感；空串 = 全部）。 */
export function filterGraphNodes(nodes: CollectionGraphNode[], query: string): CollectionGraphNode[] {
  const q = query.trim().toLowerCase();
  if (!q) return nodes;
  return nodes.filter(
    (n) =>
      n.name.toLowerCase().includes(q) || n.rel_path.toLowerCase().includes(q) || n.doc_type.toLowerCase().includes(q),
  );
}

/** 一跳邻居集（含自身）：hover 高亮/淡化其余用。 */
export function oneHopNeighborIds(nodeId: string, edges: CollectionGraphEdge[]): Set<string> {
  const out = new Set<string>([nodeId]);
  for (const e of edges) {
    if (e.source === nodeId) out.add(e.target);
    else if (e.target === nodeId) out.add(e.source);
  }
  return out;
}
