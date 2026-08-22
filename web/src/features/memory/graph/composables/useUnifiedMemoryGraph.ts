import { computed, ref, watch, type Ref } from 'vue';
import type { UnifiedGraphEdge, UnifiedGraphNode, UnifiedMemoryGraph } from '../../types';
import { useMemoryStore } from '../../../../stores/memory';

export const DEFAULT_GRAPH_HOPS = 2;
export const DEFAULT_GRAPH_MIN_WEIGHT = 0.35;
export const GRAPH_LAYERS = ['L4', 'L3', 'L2'] as const;

/** 跨层关联图谱数据与过滤状态：focus 留空时后端取关系数最多的活跃实体。 */
export function useUnifiedMemoryGraph(agentId: Ref<string | null>) {
  const memoryStore = useMemoryStore();
  const graph = ref<UnifiedMemoryGraph | null>(null);
  const loading = ref(false);
  const error = ref('');
  const hops = ref(DEFAULT_GRAPH_HOPS);
  const minWeight = ref(DEFAULT_GRAPH_MIN_WEIGHT);
  const enabledLayers = ref<string[]>([...GRAPH_LAYERS]);
  const selectedNodeId = ref<string | null>(null);

  const nodes = computed<UnifiedGraphNode[]>(() => graph.value?.nodes ?? []);
  const edges = computed<UnifiedGraphEdge[]>(() => graph.value?.edges ?? []);
  const focusId = computed(() => graph.value?.focus ?? '');
  const emptyReason = computed(() => graph.value?.empty_reason ?? '');
  const filteredEdgeCount = computed(() => graph.value?.filtered_edge_count ?? 0);

  const selectedNode = computed(() => nodes.value.find((n) => n.id === selectedNodeId.value) ?? null);
  const selectedEdges = computed(() => {
    if (!selectedNodeId.value) return [];
    return edges.value.filter((e) => e.source === selectedNodeId.value || e.target === selectedNodeId.value);
  });

  async function load(focus = '') {
    const id = agentId.value;
    if (!id) {
      graph.value = null;
      return;
    }
    loading.value = true;
    error.value = '';
    try {
      graph.value = await memoryStore.loadUnifiedGraph(id, {
        focus,
        hops: hops.value,
        min_weight: minWeight.value,
        layers: enabledLayers.value.length ? [...enabledLayers.value] : undefined,
      });
      if (selectedNodeId.value && !graph.value.nodes.some((n) => n.id === selectedNodeId.value)) {
        selectedNodeId.value = null;
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      graph.value = null;
    } finally {
      loading.value = false;
    }
  }

  function selectNode(id: string | null) {
    selectedNodeId.value = id;
  }

  function toggleLayer(layer: string) {
    const set = new Set(enabledLayers.value);
    if (set.has(layer)) {
      set.delete(layer);
    } else {
      set.add(layer);
    }
    enabledLayers.value = [...set];
  }

  /** 按名称模糊定位已加载节点（FR-R7 搜索定位）。 */
  function searchNodes(keyword: string): UnifiedGraphNode[] {
    const kw = keyword.trim().toLowerCase();
    if (!kw) return [];
    return nodes.value.filter((n) => n.label.toLowerCase().includes(kw)).slice(0, 6);
  }

  // minWeight 滑块防抖；其余过滤条件即时生效。
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;
  watch(minWeight, () => {
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => void load(), 300);
  });
  const layerKey = computed(() => [...enabledLayers.value].sort().join(','));
  watch([agentId, hops, layerKey], () => void load(), { immediate: true });

  return {
    graph,
    nodes,
    edges,
    focusId,
    emptyReason,
    filteredEdgeCount,
    loading,
    error,
    hops,
    minWeight,
    enabledLayers,
    selectedNodeId,
    selectedNode,
    selectedEdges,
    load,
    selectNode,
    toggleLayer,
    searchNodes,
  };
}
