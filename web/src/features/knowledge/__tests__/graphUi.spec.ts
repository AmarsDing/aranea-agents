// G4-F：3D 知识图谱纯函数测试（V12.7）。
import { describe, it, expect } from 'vitest';
import {
  graphLinkColor,
  graphDocTypeColor,
  graphNodeVal,
  buildRenderGraph,
  sortedGraphNodes,
  filterGraphNodes,
  oneHopNeighborIds,
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
