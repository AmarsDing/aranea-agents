// localGraphLayout 纯函数单测（SP2 §SP2-8）：力导向布局确定性。
// （B4 后邻域裁剪移至服务端 ListDocumentNeighborhood，前端 bfsNeighborhood 已删除。）
import { describe, expect, it } from 'vitest';
import { layoutLocalGraph } from '../localGraphLayout';
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
