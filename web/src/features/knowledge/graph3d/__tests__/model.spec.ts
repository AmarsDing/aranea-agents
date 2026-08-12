/**
 * model.spec：G5-A SoA 图模型契约（设计 §V12.8-1 model.ts）。
 */
import { describe, expect, it } from 'vitest';
import {
  buildGraphModel,
  filterGraphByGroups,
  mulberry32,
  seedPositions,
  type GraphEdgeInput,
  type GraphNodeInput,
} from '../model';

function node(docId: string, docType = ''): GraphNodeInput {
  return { docId, name: docId, relPath: `${docId}.md`, docType };
}

describe('buildGraphModel', () => {
  it('构建 SoA 结构：count/edgeCount/degree/数组长度', () => {
    const m = buildGraphModel(
      [node('a'), node('b'), node('c')],
      [
        { source: 'a', target: 'b', type: 'explicit' },
        { source: 'b', target: 'c', type: 'entity' },
      ],
    );
    expect(m.count).toBe(3);
    expect(m.edgeCount).toBe(2);
    expect(m.positions).toHaveLength(9);
    expect(m.velocities).toHaveLength(9);
    expect(m.degree).toHaveLength(3);
    expect(m.groupId).toHaveLength(3);
    expect(m.edges).toHaveLength(4);
    expect(m.edgeTypes).toHaveLength(2);
    expect(Array.from(m.degree)).toEqual([1, 2, 1]);
  });

  it('docId↔index 双射', () => {
    const m = buildGraphModel([node('a'), node('b')], []);
    expect(m.docIdToIndex.get('a')).toBe(0);
    expect(m.docIdToIndex.get('b')).toBe(1);
    expect(m.docIds[0]).toBe('a');
    expect(m.docIds[1]).toBe('b');
  });

  it('去重：同对多条边（不同类型）只保留一条，保留先见类型', () => {
    const m = buildGraphModel(
      [node('a'), node('b')],
      [
        { source: 'a', target: 'b', type: 'explicit' },
        { source: 'b', target: 'a', type: 'entity' }, // 反向同对
        { source: 'a', target: 'b', type: 'semantic' }, // 完全重复
      ],
    );
    expect(m.edgeCount).toBe(1);
    expect(m.edgeTypes[0]).toBe('explicit');
    expect(Array.from(m.degree)).toEqual([1, 1]);
  });

  it('剔除自环与悬空边（端点不在节点集）', () => {
    const edges: GraphEdgeInput[] = [
      { source: 'a', target: 'a', type: 'explicit' }, // 自环
      { source: 'a', target: 'ghost', type: 'entity' }, // 悬空
      { source: 'a', target: 'b', type: 'semantic' },
    ];
    const m = buildGraphModel([node('a'), node('b')], edges);
    expect(m.edgeCount).toBe(1);
    expect(m.edgeTypes[0]).toBe('semantic');
  });

  it('groupId 按 doc_type 分组：同类型同组，分配确定性（groups 排序）', () => {
    const m1 = buildGraphModel([node('a', 'note'), node('b', 'report'), node('c', 'note')], []);
    const groups = m1.groups;
    expect(groups).toEqual([...groups].sort());
    const gNote = groups.indexOf('note');
    const gReport = groups.indexOf('report');
    expect(m1.groupId[0]).toBe(gNote);
    expect(m1.groupId[2]).toBe(gNote);
    expect(m1.groupId[1]).toBe(gReport);
    // 同输入 → 同分组分配（与节点顺序无关）
    const m2 = buildGraphModel([node('c', 'note'), node('a', 'note'), node('b', 'report')], []);
    expect(m2.groups).toEqual(m1.groups);
    expect(m2.groupId[m2.docIdToIndex.get('b')!]).toBe(gReport);
  });

  it('空 doc_type 归为一个组', () => {
    const m = buildGraphModel([node('a', ''), node('b', '')], []);
    expect(m.groups).toHaveLength(1);
    expect(m.groupId[0]).toBe(m.groupId[1]);
  });
});

describe('mulberry32', () => {
  it('同 seed 同序列，不同 seed 不同序列', () => {
    const r1 = mulberry32(42);
    const r2 = mulberry32(42);
    const r3 = mulberry32(7);
    const s1 = [r1(), r1(), r1()];
    const s2 = [r2(), r2(), r2()];
    const s3 = [r3(), r3(), r3()];
    expect(s1).toEqual(s2);
    expect(s1).not.toEqual(s3);
    for (const v of s1) {
      expect(v).toBeGreaterThanOrEqual(0);
      expect(v).toBeLessThan(1);
    }
  });
});

describe('M5 filterGraphByGroups', () => {
  const nodes: GraphNodeInput[] = [
    { docId: 'a', name: 'a', relPath: 'a.md', docType: 'note' },
    { docId: 'b', name: 'b', relPath: 'b.md', docType: 'note' },
    { docId: 'c', name: 'c', relPath: 'c.md', docType: 'image' },
  ];
  const edges: GraphEdgeInput[] = [
    { source: 'a', target: 'b', type: 'explicit' },
    { source: 'b', target: 'c', type: 'semantic' },
  ];

  it('空 hiddenGroups：原样返回（引用相等，零开销）', () => {
    const out = filterGraphByGroups(nodes, edges, new Set());
    expect(out.nodes).toBe(nodes);
    expect(out.edges).toBe(edges);
  });

  it('隐藏 image 组：节点 c 排除 + 边 b-c 级联排除', () => {
    const out = filterGraphByGroups(nodes, edges, new Set(['image']));
    expect(out.nodes.map((n) => n.docId)).toEqual(['a', 'b']);
    expect(out.edges).toEqual([{ source: 'a', target: 'b', type: 'explicit' }]);
  });

  it('隐藏全部组：空图', () => {
    const out = filterGraphByGroups(nodes, edges, new Set(['note', 'image']));
    expect(out.nodes).toEqual([]);
    expect(out.edges).toEqual([]);
  });
});

describe('seedPositions', () => {
  it('确定性：同 seed 同布局', () => {
    const m = buildGraphModel([node('a'), node('b'), node('c')], []);
    seedPositions(m, 42);
    const first = Array.from(m.positions);
    seedPositions(m, 42);
    expect(Array.from(m.positions)).toEqual(first);
  });

  it('球内体采样：所有点模长 ≤ cbrt(N)*20+1', () => {
    const nodes = Array.from({ length: 100 }, (_, i) => node(`n${i}`));
    const m = buildGraphModel(nodes, []);
    seedPositions(m, 1);
    const radius = Math.cbrt(100) * 20 + 1;
    for (let i = 0; i < 100; i++) {
      const x = m.positions[i * 3];
      const y = m.positions[i * 3 + 1];
      const z = m.positions[i * 3 + 2];
      expect(Math.sqrt(x * x + y * y + z * z)).toBeLessThanOrEqual(radius + 1e-4);
    }
  });
});
