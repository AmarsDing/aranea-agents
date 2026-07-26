import dagre from 'dagre';
import type { UnifiedGraphEdge, UnifiedGraphNode } from '../types';
import { memoryLayerColor } from '../panorama/layerMeta';

/** 节点外形尺寸（与 UnifiedGraphNode.vue 渲染盒一致）。 */
export const UNIFIED_NODE_WIDTH = 132;
export const UNIFIED_NODE_HEIGHT = 96;

/**
 * 边样式（设计 §10.4）：冲突/INHIBIT → 红色虚线 `#ef5350`；事实来源 → 虚线；
 * 其余（实体关系/实体-事实/事实链接）→ 实线，颜色随源节点层级色码。
 */
export function unifiedEdgeStyle(edge: UnifiedGraphEdge, sourceLayer: string): Record<string, string> {
  if (edge.type === 'fact_conflict' || edge.polarity === 'INHIBIT') {
    return { stroke: '#ef5350', strokeDasharray: '4 3', strokeWidth: '1.8' };
  }
  if (edge.type === 'fact_source') {
    return { stroke: memoryLayerColor(sourceLayer), strokeDasharray: '5 4', strokeWidth: '1.4' };
  }
  return { stroke: memoryLayerColor(sourceLayer), strokeWidth: '1.6' };
}

/**
 * dagre 分层布局（设计 §10.3：L4→L3→L2 自上而下）。
 * 跨层边（entity_fact / fact_source）天然把图分成 L4/L3/L2 三层；
 * 同层边（entity_relation / fact_link）由 dagre 在层内排布。
 */
export function layoutUnifiedMemoryGraph(
  nodes: UnifiedGraphNode[],
  edges: UnifiedGraphEdge[],
): Map<string, { x: number; y: number }> {
  const out = new Map<string, { x: number; y: number }>();
  if (!nodes.length) return out;

  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: 'TB', nodesep: 36, ranksep: 80, marginx: 24, marginy: 24 });

  for (const n of nodes) {
    g.setNode(n.id, { width: UNIFIED_NODE_WIDTH, height: UNIFIED_NODE_HEIGHT });
  }
  const ids = new Set(nodes.map((n) => n.id));
  for (const e of edges) {
    if (ids.has(e.source) && ids.has(e.target) && e.source !== e.target) {
      g.setEdge(e.source, e.target);
    }
  }

  dagre.layout(g);

  nodes.forEach((n, index) => {
    const p = g.node(n.id);
    if (p) {
      out.set(n.id, { x: p.x - UNIFIED_NODE_WIDTH / 2, y: p.y - UNIFIED_NODE_HEIGHT / 2 });
    } else {
      // dagre 未排位（孤立节点兜底）：按序网格铺开。
      out.set(n.id, { x: 40 + (index % 5) * (UNIFIED_NODE_WIDTH + 36), y: 40 });
    }
  });
  return out;
}
