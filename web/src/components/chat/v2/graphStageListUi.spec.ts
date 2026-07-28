import { describe, expect, it } from 'vitest';
import { defaultGraphNodeExpanded, deriveGraphStageStatus, orderGraphNodesForList } from './graphStageListUi';

describe('deriveGraphStageStatus', () => {
  it('prefers terminal backend status over node aggregation', () => {
    expect(deriveGraphStageStatus('completed', [{ Status: 'running' }])).toBe('completed');
    expect(deriveGraphStageStatus('failed', [{ Status: 'completed' }])).toBe('failed');
    expect(deriveGraphStageStatus('interrupted', [])).toBe('interrupted');
  });

  it('falls back to running when there are no nodes', () => {
    expect(deriveGraphStageStatus('', [])).toBe('running');
    expect(deriveGraphStageStatus('running', [])).toBe('running');
  });

  it('aggregates node status: all completed → completed', () => {
    expect(deriveGraphStageStatus('running', [{ Status: 'completed' }, { Status: 'completed' }])).toBe('completed');
  });

  it('aggregates node status: failed wins over interrupted and running', () => {
    const nodes = [{ Status: 'running' }, { Status: 'interrupted' }, { Status: 'failed' }];
    expect(deriveGraphStageStatus('running', nodes)).toBe('failed');
  });

  it('aggregates node status: interrupted wins over running', () => {
    expect(deriveGraphStageStatus('running', [{ Status: 'running' }, { Status: 'interrupted' }])).toBe('interrupted');
  });

  it('aggregates node status: any running → running', () => {
    expect(deriveGraphStageStatus('running', [{ Status: 'pending' }, { Status: 'running' }])).toBe('running');
  });
});

describe('orderGraphNodesForList', () => {
  const layers = new Map([
    ['a', 0],
    ['b', 1],
    ['c', 1],
    ['d', 2],
  ]);
  const orderInLayer = new Map([
    ['a', 0],
    ['b', 1],
    ['c', 0],
    ['d', 0],
  ]);

  it('orders by layer asc then in-layer order asc', () => {
    const nodes = [{ ID: 'd' }, { ID: 'b' }, { ID: 'c' }, { ID: 'a' }];
    expect(orderGraphNodesForList(nodes, layers, orderInLayer).map((n) => n.ID)).toEqual(['a', 'c', 'b', 'd']);
  });

  it('appends nodes missing from the layout maps at the end, keeping input order', () => {
    const nodes = [{ ID: 'x' }, { ID: 'b' }, { ID: 'y' }, { ID: 'a' }];
    expect(orderGraphNodesForList(nodes, layers, orderInLayer).map((n) => n.ID)).toEqual(['a', 'b', 'x', 'y']);
  });

  it('does not mutate the input array', () => {
    const nodes = [{ ID: 'b' }, { ID: 'a' }];
    orderGraphNodesForList(nodes, layers, orderInLayer);
    expect(nodes.map((n) => n.ID)).toEqual(['b', 'a']);
  });
});

describe('defaultGraphNodeExpanded', () => {
  it('expands attention-needing statuses', () => {
    expect(defaultGraphNodeExpanded('running')).toBe(true);
    expect(defaultGraphNodeExpanded('failed')).toBe(true);
    expect(defaultGraphNodeExpanded('interrupted')).toBe(true);
  });

  it('collapses quiet statuses', () => {
    expect(defaultGraphNodeExpanded('completed')).toBe(false);
    expect(defaultGraphNodeExpanded('pending')).toBe(false);
    expect(defaultGraphNodeExpanded('')).toBe(false);
  });
});
