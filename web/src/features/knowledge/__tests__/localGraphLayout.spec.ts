// localGraphLayout 纯函数单测（SP2 §SP2-8）：BFS 邻域裁剪 + 力导向布局确定性。
import { describe, expect, it } from 'vitest';
import { bfsNeighborhood, layoutLocalGraph } from '../localGraphLayout';
import type { CollectionGraphEdge, CollectionGraphNode } from '../types';

const N = (id: string): CollectionGraphNode => ({
  doc_id: id,
  name: id,
  rel_path: `${id}.md`,
  doc_type: 'note',
  degree: 0,
});
const E = (s: string, t: string): CollectionGraphEdge => ({ source: s, target: t, type: 'explicit' });

// 拓扑：a - b - c - d（链），a - e（旁支）
const nodes = ['a', 'b', 'c', 'd', 'e'].map(N);
const edges = [E('a', 'b'), E('b', 'c'), E('c', 'd'), E('a', 'e')];

describe('bfsNeighborhood', () => {
  it('hops=1 keeps direct neighbors only', () => {
    const { nodes: ns, edges: es } = bfsNeighborhood(nodes, edges, 'a', 1);
    expect(ns.map((x) => x.doc_id).sort()).toEqual(['a', 'b', 'e']);
    expect(es).toHaveLength(2);
  });

  it('hops=2 reaches c but not d', () => {
    const { nodes: ns } = bfsNeighborhood(nodes, edges, 'a', 2);
    expect(ns.map((x) => x.doc_id).sort()).toEqual(['a', 'b', 'c', 'e']);
  });

  it('treats edges as undirected (backlink reachability)', () => {
    const { nodes: ns } = bfsNeighborhood(nodes, edges, 'd', 1);
    expect(ns.map((x) => x.doc_id).sort()).toEqual(['c', 'd']);
  });

  it('empty root or zero hops returns empty', () => {
    expect(bfsNeighborhood(nodes, edges, '', 2).nodes).toEqual([]);
    expect(bfsNeighborhood(nodes, edges, 'a', 0).nodes).toEqual([]);
  });
});

describe('layoutLocalGraph', () => {
  it('positions all nodes inside bounds deterministically', () => {
    const a = layoutLocalGraph(nodes, edges, 300, 240);
    const b = layoutLocalGraph(nodes, edges, 300, 240);
    expect(a.size).toBe(5);
    for (const id of ['a', 'b', 'c', 'd', 'e']) {
      const p = a.get(id)!;
      expect(p.x).toBeGreaterThanOrEqual(16);
      expect(p.x).toBeLessThanOrEqual(284);
      expect(p.y).toBeGreaterThanOrEqual(16);
      expect(p.y).toBeLessThanOrEqual(224);
      expect(p).toEqual(b.get(id));
    }
  });

  it('empty graph returns empty map', () => {
    expect(layoutLocalGraph([], [], 100, 100).size).toBe(0);
  });
});
