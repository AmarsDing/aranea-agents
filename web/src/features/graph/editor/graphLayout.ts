import type { GraphDefinition, GraphLayoutMetadata } from "../types";
import { GRAPH_LAYOUT_METADATA_KEY } from "../types";

export function readGraphLayout(graphDef: GraphDefinition): GraphLayoutMetadata {
  const raw = graphDef.metadata?.[GRAPH_LAYOUT_METADATA_KEY];
  if (!raw || typeof raw !== "object") return {};
  const layout: GraphLayoutMetadata = {};
  for (const [nodeId, pos] of Object.entries(raw as Record<string, unknown>)) {
    if (!pos || typeof pos !== "object") continue;
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

/** Layered left-to-right layout for workflow graphs without saved positions. */
export function applyAutoLayout(graphDef: GraphDefinition): void {
  if (graphDef.nodes.length === 0) return;

  const nodeIds = new Set(graphDef.nodes.map((node) => node.id));
  const inDegree = new Map<string, number>();
  const adjacency = new Map<string, string[]>();

  for (const id of nodeIds) {
    inDegree.set(id, 0);
    adjacency.set(id, []);
  }

  for (const edge of graphDef.edges) {
    if (!nodeIds.has(edge.from) || !nodeIds.has(edge.to)) continue;
    adjacency.get(edge.from)?.push(edge.to);
    inDegree.set(edge.to, (inDegree.get(edge.to) ?? 0) + 1);
  }

  const entry =
    graphDef.entryPoint && nodeIds.has(graphDef.entryPoint)
      ? graphDef.entryPoint
      : graphDef.nodes.find((node) => (inDegree.get(node.id) ?? 0) === 0)?.id ?? graphDef.nodes[0]?.id;

  const layer = new Map<string, number>();
  if (entry) {
    const queue = [entry];
    layer.set(entry, 0);
    while (queue.length > 0) {
      const current = queue.shift()!;
      for (const next of adjacency.get(current) ?? []) {
        const nextLayer = (layer.get(current) ?? 0) + 1;
        const prior = layer.get(next);
        if (prior === undefined || prior < nextLayer) {
          layer.set(next, nextLayer);
          queue.push(next);
        }
      }
    }
  }

  for (const node of graphDef.nodes) {
    if (!layer.has(node.id)) {
      layer.set(node.id, 0);
    }
  }

  const byLayer = new Map<number, string[]>();
  for (const [id, depth] of layer.entries()) {
    const bucket = byLayer.get(depth) ?? [];
    bucket.push(id);
    byLayer.set(depth, bucket);
  }

  for (const [depth, ids] of byLayer.entries()) {
    ids.sort((a, b) => a.localeCompare(b));
    ids.forEach((id, index) => {
      writeGraphNodePosition(graphDef, id, {
        x: 80 + depth * 300,
        y: 80 + index * 180,
      });
    });
  }
}
