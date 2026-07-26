// web/src/features/memory/graph/__tests__/useUnifiedMemoryGraph.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ref, nextTick } from 'vue';
import type { UnifiedMemoryGraph } from '../../types';

vi.mock('../../api', () => ({
  getUnifiedMemoryGraph: vi.fn(),
}));

import { getUnifiedMemoryGraph } from '../../api';
import {
  useUnifiedMemoryGraph,
  DEFAULT_GRAPH_HOPS,
  DEFAULT_GRAPH_MIN_WEIGHT,
} from '../composables/useUnifiedMemoryGraph';

const mockApi = vi.mocked(getUnifiedMemoryGraph);

function sampleGraph(): UnifiedMemoryGraph {
  return {
    focus: 'e1',
    nodes: [
      { id: 'e1', layer: 'L4', kind: 'entity', label: 'Acme Corp', weight: 0.9, meta_json: '{}' },
      { id: 'f1', layer: 'L3', kind: 'fact', label: 'Acme 营收超预期', weight: 0.8, meta_json: '{}' },
      { id: 'ep1', layer: 'L2', kind: 'episode', label: 'Q3 复盘会议', weight: 0.7, meta_json: '{}' },
    ],
    edges: [
      { source: 'e1', target: 'f1', type: 'entity_fact', label: '提及', weight: 0.8, polarity: '' },
      { source: 'f1', target: 'ep1', type: 'fact_source', label: '来源于', weight: 0.7, polarity: '' },
    ],
    node_count: 3,
    edge_count: 2,
    filtered_edge_count: 5,
    empty_reason: '',
  };
}

describe('useUnifiedMemoryGraph', () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it('agentId 为空时不请求后端，graph 保持 null', async () => {
    useUnifiedMemoryGraph(ref(null));
    await nextTick();
    expect(mockApi).not.toHaveBeenCalled();
  });

  it('加载成功：默认参数 hops=2 / min_weight=0.35 / 三层全开，focus 留空', async () => {
    mockApi.mockResolvedValue(sampleGraph());
    const g = useUnifiedMemoryGraph(ref('agent-1'));
    await vi.waitFor(() => expect(g.nodes.value.length).toBe(3));

    expect(mockApi).toHaveBeenCalledWith('agent-1', {
      focus: '',
      hops: DEFAULT_GRAPH_HOPS,
      min_weight: DEFAULT_GRAPH_MIN_WEIGHT,
      layers: ['L4', 'L3', 'L2'],
    });
    expect(g.edges.value.length).toBe(2);
    expect(g.focusId.value).toBe('e1');
    expect(g.filteredEdgeCount.value).toBe(5);
    expect(g.error.value).toBe('');
  });

  it('加载失败：error 填充、graph 置空', async () => {
    mockApi.mockRejectedValue(new Error('boom'));
    const g = useUnifiedMemoryGraph(ref('agent-1'));
    await vi.waitFor(() => expect(g.error.value).toBe('boom'));
    expect(g.graph.value).toBeNull();
    expect(g.nodes.value).toEqual([]);
  });

  it('层级开关：关闭 L2 后以 layers=[L4,L3] 重新请求', async () => {
    mockApi.mockResolvedValue(sampleGraph());
    const g = useUnifiedMemoryGraph(ref('agent-1'));
    await vi.waitFor(() => expect(g.nodes.value.length).toBe(3));

    g.toggleLayer('L2');
    await vi.waitFor(() =>
      expect(mockApi).toHaveBeenLastCalledWith(
        'agent-1',
        expect.objectContaining({ layers: expect.arrayContaining(['L4', 'L3']) }),
      ),
    );
    const lastCall = mockApi.mock.calls.at(-1)![1];
    expect(lastCall?.layers).toHaveLength(2);
    expect(lastCall?.layers).not.toContain('L2');
  });

  it('选中节点：selectedEdges 只含相连边；重载后节点消失则清除选中', async () => {
    mockApi.mockResolvedValue(sampleGraph());
    const g = useUnifiedMemoryGraph(ref('agent-1'));
    await vi.waitFor(() => expect(g.nodes.value.length).toBe(3));

    g.selectNode('f1');
    expect(g.selectedNode.value?.label).toBe('Acme 营收超预期');
    expect(g.selectedEdges.value).toHaveLength(2);

    mockApi.mockResolvedValue({ ...sampleGraph(), focus: 'e1', nodes: [sampleGraph().nodes[0]], edges: [] });
    await g.load();
    expect(g.selectedNodeId.value).toBeNull();
    expect(g.selectedEdges.value).toEqual([]);
  });

  it('searchNodes：按名称模糊匹配（大小写不敏感），最多 6 条', async () => {
    mockApi.mockResolvedValue(sampleGraph());
    const g = useUnifiedMemoryGraph(ref('agent-1'));
    await vi.waitFor(() => expect(g.nodes.value.length).toBe(3));

    expect(g.searchNodes('acme')).toHaveLength(2);
    expect(g.searchNodes('复盘')).toHaveLength(1);
    expect(g.searchNodes('  ')).toEqual([]);
    expect(g.searchNodes('不存在')).toEqual([]);
  });

  it('empty_reason 透传', async () => {
    mockApi.mockResolvedValue({ ...sampleGraph(), nodes: [], edges: [], empty_reason: 'no_memory_data' });
    const g = useUnifiedMemoryGraph(ref('agent-1'));
    await vi.waitFor(() => expect(g.emptyReason.value).toBe('no_memory_data'));
  });
});
