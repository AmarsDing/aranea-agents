import dagre from 'dagre';
import type { GraphDefinition, GraphLayoutMetadata } from '../types';
import { GRAPH_LAYOUT_METADATA_KEY, NODE_DEFAULT_WIDTH, NODE_DEFAULT_HEIGHT } from '../types';

export function readGraphLayout(graphDef: GraphDefinition): GraphLayoutMetadata {
  const raw = graphDef.metadata?.[GRAPH_LAYOUT_METADATA_KEY];
  if (!raw || typeof raw !== 'object') return {};
  const layout: GraphLayoutMetadata = {};
  for (const [nodeId, pos] of Object.entries(raw as Record<string, unknown>)) {
    if (!pos || typeof pos !== 'object') continue;
    const point = pos as { x?: unknown; y?: unknown };
    const x = Number(point.x);
    const y = Number(point.y);
    if (Number.isFinite(x) && Number.isFinite(y)) {
      layout[nodeId] = { x, y };
    }
  }
  return layout;
}

export function writeGraphNodePosition(
  graphDef: GraphDefinition,
  nodeId: string,
  position: { x: number; y: number },
): void {
  const layout = readGraphLayout(graphDef);
  layout[nodeId] = position;
  graphDef.metadata = {
    ...graphDef.metadata,
    [GRAPH_LAYOUT_METADATA_KEY]: layout,
  };
}

export function defaultNodePosition(index: number): { x: number; y: number } {
  const column = index % 4;
  const row = Math.floor(index / 4);
  return { x: 120 + column * 220, y: 100 + row * 140 };
}

export function hasSavedLayout(graphDef: GraphDefinition): boolean {
  const layout = readGraphLayout(graphDef);
  return graphDef.nodes.some((node) => Boolean(layout[node.id]));
}

export type NodeMoveInfo = {
  nodeId: string;
  oldPos: { x: number; y: number };
  newPos: { x: number; y: number };
};

export function applyAutoLayout(graphDef: GraphDefinition): NodeMoveInfo[] {
  if (graphDef.nodes.length === 0) return [];

  const oldLayout = readGraphLayout(graphDef);

  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({
    rankdir: 'LR',
    nodesep: 60,
    ranksep: 120,
    marginx: 40,
    marginy: 40,
  });

  for (const node of graphDef.nodes) {
    g.setNode(node.id, { width: NODE_DEFAULT_WIDTH, height: NODE_DEFAULT_HEIGHT });
  }

  const nodeIds = new Set(graphDef.nodes.map((n) => n.id));
  for (const edge of graphDef.edges) {
    if (nodeIds.has(edge.from) && nodeIds.has(edge.to) && edge.from !== edge.to) {
      g.setEdge(edge.from, edge.to);
    }
  }

  for (const ce of graphDef.conditionalEdges) {
    if (!nodeIds.has(ce.from)) continue;
    const targets = Object.values(ce.pathMap ?? {});
    for (const target of targets) {
      if (nodeIds.has(target) && ce.from !== target) {
        g.setEdge(ce.from, target);
      }
    }
  }

  dagre.layout(g);

  const moves: NodeMoveInfo[] = [];
  for (const node of graphDef.nodes) {
    const pos = g.node(node.id);
    if (pos) {
      const newPos = {
        x: pos.x - (pos.width ?? NODE_DEFAULT_WIDTH) / 2,
        y: pos.y - (pos.height ?? NODE_DEFAULT_HEIGHT) / 2,
      };
      const oldPos = oldLayout[node.id] ?? { x: 0, y: 0 };
      writeGraphNodePosition(graphDef, node.id, newPos);
      moves.push({ nodeId: node.id, oldPos, newPos });
    }
  }
  return moves;
}
