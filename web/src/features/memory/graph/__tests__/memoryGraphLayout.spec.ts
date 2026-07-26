// web/src/features/memory/graph/__tests__/memoryGraphLayout.spec.ts
import { describe, it, expect } from 'vitest';
import { layoutUnifiedMemoryGraph, unifiedEdgeStyle } from '../memoryGraphLayout';
import type { UnifiedGraphEdge, UnifiedGraphNode } from '../../types';

function node(id: string, layer: string, kind = 'entity'): UnifiedGraphNode {
  return { id, layer, kind, label: id, weight: 0.8, meta_json: '{}' };
}

function edge(source: string, target: string, type: string, polarity = ''): UnifiedGraphEdge {
  return { source, target, type, label: type, weight: 0.7, polarity };
}

describe('layoutUnifiedMemoryGraph（设计 §10.3：L4→L3→L2 自上而下）', () => {
  it('跨层链布局：L4 实体在最上，L3 事实居中，L2 情景在最下', () => {
    const nodes = [node('e1', 'L4'), node('f1', 'L3', 'fact'), node('ep1', 'L2', 'episode')];
    const edges = [edge('e1', 'f1', 'entity_fact'), edge('f1', 'ep1', 'fact_source')];

    const pos = layoutUnifiedMemoryGraph(nodes, edges);

    expect(pos.get('e1')!.y).toBeLessThan(pos.get('f1')!.y);
    expect(pos.get('f1')!.y).toBeLessThan(pos.get('ep1')!.y);
  });

  it('所有节点都获得坐标（含孤立节点兜底）', () => {
    const nodes = [node('e1', 'L4'), node('f1', 'L3', 'fact'), node('ep1', 'L2', 'episode')];

    const pos = layoutUnifiedMemoryGraph(nodes, []);

    expect(pos.size).toBe(3);
    for (const n of nodes) {
      expect(Number.isFinite(pos.get(n.id)!.x)).toBe(true);
      expect(Number.isFinite(pos.get(n.id)!.y)).toBe(true);
    }
  });

  it('空图返回空 Map', () => {
    expect(layoutUnifiedMemoryGraph([], []).size).toBe(0);
  });
});

describe('unifiedEdgeStyle（设计 §10.4 边样式）', () => {
  it('fact_conflict → 红色虚线 #ef5350', () => {
    const style = unifiedEdgeStyle(edge('f1', 'f2', 'fact_conflict'), 'L3');
    expect(style.stroke).toBe('#ef5350');
    expect(style.strokeDasharray).toBe('4 3');
  });

  it('polarity=INHIBIT → 红色虚线（无论边类型）', () => {
    const style = unifiedEdgeStyle(edge('e1', 'e2', 'entity_relation', 'INHIBIT'), 'L4');
    expect(style.stroke).toBe('#ef5350');
    expect(style.strokeDasharray).toBe('4 3');
  });

  it('fact_source → 虚线，颜色随源层级', () => {
    const style = unifiedEdgeStyle(edge('f1', 'ep1', 'fact_source'), 'L3');
    expect(style.stroke).toBe('#ba68c8');
    expect(style.strokeDasharray).toBe('5 4');
  });

  it('entity_relation → 实线，颜色随源层级（L4=#ff8a65）', () => {
    const style = unifiedEdgeStyle(edge('e1', 'e2', 'entity_relation'), 'L4');
    expect(style.stroke).toBe('#ff8a65');
    expect(style.strokeDasharray).toBeUndefined();
  });

  it('entity_fact → 实线，颜色随源层级', () => {
    const style = unifiedEdgeStyle(edge('e1', 'f1', 'entity_fact'), 'L4');
    expect(style.stroke).toBe('#ff8a65');
    expect(style.strokeDasharray).toBeUndefined();
  });
});
