// web/src/features/chat/composables/usePlanDAGLayout.ts

/** Minimal node contract for DAG layout: an ID and a list of dependency IDs. */
export interface LayoutableNode {
  ID: string;
  DependsOn: string[];
}

export interface DAGLayoutOptions {
  width: number;
  nodeWidth: number;
  nodeHeight: number;
  gapX: number;
  gapY: number;
}

export interface NodePosition {
  x: number;
  y: number;
}

/**
 * usePlanDAGLayout computes (x, y) positions for plan step nodes in a
 * top-down DAG layout using longest-path layering.
 *
 * Generic over LayoutableNode so both PlanStep[] and GraphNode[] can reuse
 * the same layout algorithm (2026-07-04 补齐：GraphStage 复用)。
 */
export function usePlanDAGLayout<T extends LayoutableNode>() {
  function layoutDAG(steps: T[], opts: DAGLayoutOptions): Map<string, NodePosition> {
    const positions = new Map<string, NodePosition>();
    if (steps.length === 0) return positions;

    // Build dependency graph
    const stepMap = new Map(steps.map((s) => [s.ID, s]));
    const layer = new Map<string, number>(); // step ID → layer (0 = root)

    // Compute layer = longest path from any root
    function getLayer(id: string): number {
      if (layer.has(id)) return layer.get(id)!;
      const s = stepMap.get(id);
      if (!s || s.DependsOn.length === 0) {
        layer.set(id, 0);
        return 0;
      }
      const maxDepLayer = Math.max(...s.DependsOn.map((d) => getLayer(d)));
      const l = maxDepLayer + 1;
      layer.set(id, l);
      return l;
    }
    steps.forEach((s) => getLayer(s.ID));

    // Group by layer
    const byLayer = new Map<number, string[]>();
    for (const [id, l] of layer) {
      if (!byLayer.has(l)) byLayer.set(l, []);
      byLayer.get(l)!.push(id);
    }

    // Position: y = layer * (nodeHeight + gapY), x = centered in layer
    const maxLayer = Math.max(...layer.values());
    for (let l = 0; l <= maxLayer; l++) {
      const ids = byLayer.get(l) || [];
      const layerWidth = ids.length * opts.nodeWidth + (ids.length - 1) * opts.gapX;
      const startX = (opts.width - layerWidth) / 2;
      ids.forEach((id, i) => {
        positions.set(id, {
          x: startX + i * (opts.nodeWidth + opts.gapX),
          y: l * (opts.nodeHeight + opts.gapY),
        });
      });
    }

    return positions;
  }

  return { layoutDAG };
}
