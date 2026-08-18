// G4-F：3D 知识图谱纯函数测试（V12.7）。
import { describe, it, expect } from 'vitest';
import {
  graphLinkColor,
  graphDocTypeColor,
  graphNodeGroupKey,
  graphNodeVal,
  buildRenderGraph,
  sortedGraphNodes,
  filterGraphNodes,
  oneHopNeighborIds,
  bfsNeighborhoodIds,
  buildNeighborhoodGraph,
  GRAPH_LINK_TYPES,
  GRAPH_ISOLATED_HIDE_THRESHOLD,
} from '../graphUi';
import type { CollectionGraphEdge, CollectionGraphNode } from '../types';

function node(id: string, degree = 0, name = id, docType = 'note', relPath = `${id}.md`): CollectionGraphNode {
  return { doc_id: id, name, rel_path: relPath, doc_type: docType, degree };
}

function edge(source: string, target: string, type = 'explicit'): CollectionGraphEdge {
  return { source, target, type };
}

describe('graphLinkColor', () => {
  it('三类边按 V12.7 配色：explicit 蓝 / entity 紫 / semantic 青', () => {
    expect(graphLinkColor('explicit')).toBe('#4c8dff');
    expect(graphLinkColor('entity')).toBe('#a066d3');
    expect(graphLinkColor('semantic')).toBe('#3bbdbd');
  });

  it('未知类型灰色兜底', () => {
    expect(graphLinkColor('unknown')).toBe('#8a93a6');
    expect(graphLinkColor('')).toBe('#8a93a6');
  });
});

describe('graphDocTypeColor', () => {
  it('空类型灰色兜底', () => {
    expect(graphDocTypeColor('')).toBe('#8a93a6');
    expect(graphDocTypeColor('  ')).toBe('#8a93a6');
  });

  it('同类型稳定同色（大小写不敏感）', () => {
    expect(graphDocTypeColor('note')).toBe(graphDocTypeColor('note'));
    expect(graphDocTypeColor('Note')).toBe(graphDocTypeColor('note'));
  });
});

describe('graphNodeGroupKey（UX：doc_type 空回退顶级目录分组）', () => {
  it('doc_type 非空优先（去空白）', () => {
    expect(graphNodeGroupKey('note', 'entries/a.md')).toBe('note');
    expect(graphNodeGroupKey('  report ', 'diary/b.md')).toBe('report');
  });

  it('doc_type 空 → rel_path 顶级目录', () => {
    expect(graphNodeGroupKey('', 'entries/机房.md')).toBe('entries');
    expect(graphNodeGroupKey('  ', 'diary/2026-08-18.md')).toBe('diary');
    expect(graphNodeGroupKey('', 'inbox/writeback/a.md')).toBe('inbox');
  });

  it('doc_type 空且根目录文件 → 空（未分类灰）', () => {
    expect(graphNodeGroupKey('', 'README.md')).toBe('');
    expect(graphNodeGroupKey('', '')).toBe('');
  });
});

describe('graphNodeVal', () => {
  it('孤立节点（度 0）保持可见下限', () => {
    expect(graphNodeVal(0)).toBeCloseTo(1.5);
  });

  it('度越大体积越大，sqrt 压缩长尾', () => {
    const v1 = graphNodeVal(1);
    const v4 = graphNodeVal(4);
    const v100 = graphNodeVal(100);
    expect(v1).toBeGreaterThan(graphNodeVal(0));
    expect(v4).toBeGreaterThan(v1);
    expect(v100).toBeGreaterThan(v4);
    // sqrt 压缩：度 100 的体积远小于度 1 的 100 倍。
    expect(v100).toBeLessThan(v1 * 20);
  });
});

describe('buildRenderGraph', () => {
  it('节点数 ≤ 阈值：全量渲染，无隐藏', () => {
    const nodes = [node('a', 1), node('b', 0)];
    const edges = [edge('a', 'a')];
    const g = buildRenderGraph(nodes, edges, false);
    expect(g.nodes).toHaveLength(2);
    expect(g.hiddenIsolated).toBe(0);
  });

  it('节点数 > 阈值：默认隐藏孤立节点，边端点保持一致', () => {
    // 构造阈值 + 2 个节点：a-b 相连，iso 孤立。
    const nodes = [node('a', 1), node('b', 1), node('iso', 0)];
    for (let i = 0; i < GRAPH_ISOLATED_HIDE_THRESHOLD - 1; i++) {
      nodes.push(node(`pad${i}`, 0));
    }
    const edges = [edge('a', 'b')];
    const g = buildRenderGraph(nodes, edges, false);
    expect(g.nodes.map((n) => n.doc_id).sort()).toEqual(['a', 'b']);
    expect(g.edges).toHaveLength(1);
    expect(g.hiddenIsolated).toBe(nodes.length - 2);
  });

  it('showIsolated = true：超阈值也全量渲染', () => {
    const nodes = [node('a', 1)];
    for (let i = 0; i < GRAPH_ISOLATED_HIDE_THRESHOLD; i++) {
      nodes.push(node(`pad${i}`, 0));
    }
    const g = buildRenderGraph(nodes, [], true);
    expect(g.nodes).toHaveLength(nodes.length);
    expect(g.hiddenIsolated).toBe(0);
  });
});

describe('sortedGraphNodes', () => {
  it('连接度降序，同度按名称升序，不改原数组', () => {
    const nodes = [node('a', 1, 'beta'), node('b', 3, 'alpha'), node('c', 1, 'alpha')];
    const sorted = sortedGraphNodes(nodes);
    expect(sorted.map((n) => n.doc_id)).toEqual(['b', 'c', 'a']);
    expect(nodes.map((n) => n.doc_id)).toEqual(['a', 'b', 'c']);
  });
});

describe('filterGraphNodes', () => {
  const nodes = [
    node('a', 0, '季度报告', 'report', 'notes/q1.md'),
    node('b', 0, 'Roadmap', 'note', 'plans/Roadmap.md'),
  ];

  it('空 query 返回全部', () => {
    expect(filterGraphNodes(nodes, '')).toHaveLength(2);
    expect(filterGraphNodes(nodes, '  ')).toHaveLength(2);
  });

  it('名称/路径/类型子串匹配，大小写不敏感', () => {
    expect(filterGraphNodes(nodes, '季度').map((n) => n.doc_id)).toEqual(['a']);
    expect(filterGraphNodes(nodes, 'roadmap').map((n) => n.doc_id)).toEqual(['b']);
    expect(filterGraphNodes(nodes, 'REPORT').map((n) => n.doc_id)).toEqual(['a']);
    expect(filterGraphNodes(nodes, 'plans/').map((n) => n.doc_id)).toEqual(['b']);
    expect(filterGraphNodes(nodes, 'zzz')).toHaveLength(0);
  });
});

describe('oneHopNeighborIds', () => {
  it('含自身 + 双向一跳邻居', () => {
    const edges = [edge('a', 'b'), edge('c', 'a'), edge('b', 'd')];
    const ids = oneHopNeighborIds('a', edges);
    expect([...ids].sort()).toEqual(['a', 'b', 'c']);
  });

  it('孤立节点仅自身', () => {
    expect([...oneHopNeighborIds('x', [edge('a', 'b')])]).toEqual(['x']);
  });
});

describe('GRAPH_LINK_TYPES', () => {
  it('与后端 link_type 三类一致', () => {
    expect([...GRAPH_LINK_TYPES]).toEqual(['explicit', 'entity', 'semantic']);
  });
});

// G5-D D-5：局部图谱 N 跳 BFS（设计 §V12.8-1「聚焦邻域」）。
describe('bfsNeighborhoodIds / buildNeighborhoodGraph', () => {
  // 链式图：a-b-c-d-e；另有孤立 x
  const edges = [edge('a', 'b'), edge('b', 'c'), edge('c', 'd'), edge('d', 'e')];
  const nodes = [node('a'), node('b'), node('c'), node('d'), node('e'), node('x')];

  it('hops=0 仅根节点', () => {
    expect([...bfsNeighborhoodIds('b', edges, 0)]).toEqual(['b']);
  });

  it('hops=1 一跳邻域（含根）', () => {
    expect([...bfsNeighborhoodIds('b', edges, 1)].sort()).toEqual(['a', 'b', 'c']);
  });

  it('hops=2 二跳邻域；hops 超图径饱和', () => {
    expect([...bfsNeighborhoodIds('b', edges, 2)].sort()).toEqual(['a', 'b', 'c', 'd']);
    expect([...bfsNeighborhoodIds('b', edges, 99)].sort()).toEqual(['a', 'b', 'c', 'd', 'e']);
  });

  it('孤立节点任何跳数都只有自身', () => {
    expect([...bfsNeighborhoodIds('x', edges, 3)]).toEqual(['x']);
  });

  it('无向：从链尾反向可达', () => {
    expect([...bfsNeighborhoodIds('e', edges, 2)].sort()).toEqual(['c', 'd', 'e']);
  });

  it('子图过滤：节点/边端点一致且 doc_type 保留（调色板哈希跨视图同色）', () => {
    const sub = buildNeighborhoodGraph(nodes, edges, 'b', 1);
    expect(sub.nodes.map((n) => n.doc_id).sort()).toEqual(['a', 'b', 'c']);
    expect(sub.edges).toEqual([edge('a', 'b'), edge('b', 'c')]);
    for (const n of sub.nodes) expect(n.doc_type).toBe('note');
  });
});
