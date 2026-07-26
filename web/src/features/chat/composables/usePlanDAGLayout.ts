// web/src/features/chat/composables/usePlanDAGLayout.ts

/** Minimal node contract for DAG layout: an ID and a list of dependency IDs. */
export interface LayoutableNode {
  ID: string;
  DependsOn: string[];
}

export interface DAGLayoutOptions {
  /** Max SVG width. Actual width is computed dynamically per layer. */
  width: number;
  nodeWidth: number;
  nodeHeight: number;
  gapX: number;
  gapY: number;
  /** Horizontal padding inside the SVG (left/right). Default 20. */
  padX?: number;
  /** Vertical padding inside the SVG (top/bottom). Only used in horizontal orientation. Default 12. */
  padY?: number;
  /**
   * Layout orientation. 'vertical' (default): layers flow top-down, width is
   * content-driven and capped at `width`. 'horizontal': layers flow left-to-right
   * (showcase DAG style), width/height are structural (uncapped) — caller should
   * wrap the canvas in an overflow-x container.
   */
  orientation?: 'vertical' | 'horizontal';
}

export interface NodePosition {
  x: number;
  y: number;
}

export interface DAGLayoutResult {
  positions: Map<string, NodePosition>;
  computedWidth: number;
  computedHeight: number;
  /** Node ID → layer index (0 = root layer, longest-path layering). */
  layers: Map<string, number>;
  /** Node ID → stable index within its own layer (follows input order). */
  orderInLayer: Map<string, number>;
}

/**
 * usePlanDAGLayout computes (x, y) positions for plan step nodes in a
 * top-down DAG layout using longest-path layering.
 *
 * Generic over LayoutableNode so both PlanStep[] and GraphNode[] can reuse
 * the same layout algorithm.
 *
 * Returns `{ positions, computedWidth }` where `computedWidth` is the actual
 * SVG width needed to fit the widest layer (capped at `opts.width`).
 * For linear chains (1 node per layer), the SVG narrows to fit a single node,
 * avoiding wasted horizontal space.
 */
export function usePlanDAGLayout<T extends LayoutableNode>() {
  function layoutDAG(steps: T[], opts: DAGLayoutOptions): DAGLayoutResult {
    const positions = new Map<string, NodePosition>();
    const padX = opts.padX ?? 20;
    const padY = opts.padY ?? 12;
    const horizontal = opts.orientation === 'horizontal';
    if (steps.length === 0)
      return {
        positions,
        computedWidth: padX * 2,
        computedHeight: padY * 2,
        layers: new Map(),
        orderInLayer: new Map(),
      };

    // Build dependency graph
    const stepMap = new Map(steps.map((s) => [s.ID, s]));
    const layer = new Map<string, number>(); // step ID → layer (0 = root)

    // Compute layer = longest path from any root
    function getLayer(id: string): number {
      if (layer.has(id)) return layer.get(id)!;
      const s = stepMap.get(id);
      if (!s || !s.DependsOn || s.DependsOn.length === 0) {
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

    // Stable index within each layer (follows input insertion order) — used
    // for staggered entrance animations (per-layer + per-node delays).
    const orderInLayer = new Map<string, number>();
    for (const ids of byLayer.values()) {
      ids.forEach((id, i) => orderInLayer.set(id, i));
    }

    const maxLayer = Math.max(...layer.values());

    if (horizontal) {
      // Horizontal: layer = column (left→right), nodes in a column stack
      // vertically and are centered against the tallest column.
      // Width/height are structural (not capped by opts.width).
      const columnHeight = (l: number) => {
        const count = (byLayer.get(l) || []).length;
        return count * opts.nodeHeight + Math.max(0, count - 1) * opts.gapY;
      };
      let maxColumnHeight = 0;
      for (let l = 0; l <= maxLayer; l++) {
        const h = columnHeight(l);
        if (h > maxColumnHeight) maxColumnHeight = h;
      }
      const computedWidth = padX * 2 + (maxLayer + 1) * opts.nodeWidth + maxLayer * opts.gapX;
      const computedHeight = padY * 2 + maxColumnHeight;
      for (let l = 0; l <= maxLayer; l++) {
        const ids = byLayer.get(l) || [];
        const startY = padY + (maxColumnHeight - columnHeight(l)) / 2;
        ids.forEach((id, i) => {
          positions.set(id, {
            x: padX + l * (opts.nodeWidth + opts.gapX),
            y: startY + i * (opts.nodeHeight + opts.gapY),
          });
        });
      }
      return { positions, computedWidth, computedHeight, layers: layer, orderInLayer };
    }

    // Compute the actual width needed: widest layer + padding.
    // For linear chains (1 node per layer), this narrows to nodeWidth + 2*padX.
    let maxLayerWidth = 0;
    for (let l = 0; l <= maxLayer; l++) {
      const ids = byLayer.get(l) || [];
      const layerWidth = ids.length * opts.nodeWidth + Math.max(0, ids.length - 1) * opts.gapX;
      if (layerWidth > maxLayerWidth) maxLayerWidth = layerWidth;
    }
    const computedWidth = Math.min(opts.width, maxLayerWidth + padX * 2);

    // Position: y = layer * (nodeHeight + gapY), x = centered in layer
    for (let l = 0; l <= maxLayer; l++) {
      const ids = byLayer.get(l) || [];
      const layerWidth = ids.length * opts.nodeWidth + Math.max(0, ids.length - 1) * opts.gapX;
      const startX = (computedWidth - layerWidth) / 2;
      ids.forEach((id, i) => {
        positions.set(id, {
          x: startX + i * (opts.nodeWidth + opts.gapX),
          y: l * (opts.nodeHeight + opts.gapY),
        });
      });
    }

    const computedHeight = (maxLayer + 1) * opts.nodeHeight + maxLayer * opts.gapY;
    return { positions, computedWidth, computedHeight, layers: layer, orderInLayer };
  }

  return { layoutDAG };
}
