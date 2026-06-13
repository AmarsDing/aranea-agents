/**
 * Kahn's algorithm for topological sort with layer assignment.
 *
 * Given a list of nodes with dependency edges, returns each node's
 * depth (layer index). Nodes with no dependencies are at depth 0;
 * a node's depth is `max(parent depth + 1)` across all its parents.
 *
 * Cycles are tolerated: unreachable nodes (still with in-degree > 0
 * after the BFS) are assigned depth 0.
 */
export interface TopoNode {
  id: string;
  dependsOn: string[];
}

export function kahnTopoLayers<T extends TopoNode>(nodes: T[]): Map<string, number> {
  const depths = new Map<string, number>();

  if (nodes.length === 0) return depths;

  // Build adjacency list (parent → children) and in-degree map
  const childrenOf = new Map<string, string[]>();
  const inDegree = new Map<string, number>();

  for (const node of nodes) {
    childrenOf.set(node.id, []);
    inDegree.set(node.id, node.dependsOn.length);
    for (const depId of node.dependsOn) {
      const children = childrenOf.get(depId);
      if (children) children.push(node.id);
    }
  }

  // Enqueue root nodes (in-degree 0)
  const queue: string[] = [];
  for (const [id, deg] of inDegree) {
    if (deg === 0) queue.push(id);
  }

  // Process in BFS order, assigning layers
  let visitedCount = 0;

  while (queue.length > 0) {
    const nodeId = queue.shift()!;
    const parentDepth = depths.get(nodeId) ?? 0;
    depths.set(nodeId, parentDepth); // Store root/processed node depth
    visitedCount++;

    for (const childId of childrenOf.get(nodeId) ?? []) {
      const childDepth = parentDepth + 1;
      const existing = depths.get(childId) ?? 0;
      depths.set(childId, Math.max(existing, childDepth));

      const deg = (inDegree.get(childId) ?? 1) - 1;
      inDegree.set(childId, deg);
      if (deg === 0) {
        queue.push(childId);
      }
    }
  }

  // Cycle detection: if not all nodes visited, assign remaining to depth 0
  if (visitedCount < nodes.length) {
    for (const node of nodes) {
      if (!depths.has(node.id)) {
        depths.set(node.id, 0);
      }
    }
  }

  return depths;
}
