import { describe, expect, it } from 'vitest';
import type { InspectorEvent } from '../eventFilter';
import { buildBranchTree, filterEnvelopes, defaultEventFilterState } from '../eventFilter';

function env(partial: Partial<InspectorEvent> & Pick<InspectorEvent, 'id' | 'type'>): InspectorEvent {
  return {
    author: 'agent',
    timestamp: '2026-01-01T00:00:00Z',
    ...partial,
  };
}

describe('filterEnvelopes', () => {
  it('filters by type and keyword', () => {
    const events = [
      env({ id: '1', type: 'tool_call', tool_call: { name: 'search', status: 'ok' } }),
      env({ id: '2', type: 'text_delta', content: { text: 'hello' } }),
    ];
    const filters = { ...defaultEventFilterState(), typeFilter: 'tool_call' };
    expect(filterEnvelopes(events, filters)).toHaveLength(1);
    expect(filterEnvelopes(events, { ...defaultEventFilterState(), keyword: 'hello' })).toHaveLength(1);
  });

  it('filters by filterKey prefix', () => {
    const events = [
      env({ id: '1', type: 'tool_call', filter_key: 'agent_a/agent_b' }),
      env({ id: '2', type: 'tool_call', filter_key: 'agent_x' }),
    ];
    const filters = { ...defaultEventFilterState(), filterKey: 'agent_a' };
    expect(filterEnvelopes(events, filters)).toHaveLength(1);
    expect(filterEnvelopes(events, filters)[0]?.id).toBe('1');
  });
});

describe('buildBranchTree', () => {
  it('builds parent-child invocation tree', () => {
    const events = [
      env({ id: '1', type: 'tool_call', invocation_id: 'root', parent_invocation_id: '' }),
      env({ id: '2', type: 'tool_result', invocation_id: 'child', parent_invocation_id: 'root' }),
    ];
    const tree = buildBranchTree(events);
    expect(tree).toHaveLength(1);
    expect(tree[0]?.children).toHaveLength(1);
    expect(tree[0]?.children[0]?.id).toBe('child');
  });
});
