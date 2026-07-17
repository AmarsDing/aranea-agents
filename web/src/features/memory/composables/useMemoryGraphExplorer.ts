// FD4 fix: extract neighborhood BFS data fetching + error handling from
// MemoryGraphExplorer.vue into composable so the .vue file only handles template.
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { GraphNeighborhood } from '../types';
import { useMemoryApi } from './useMemoryApi';

export function useMemoryGraphExplorer() {
  const { t } = useI18n();
  const { getMemoryNeighborhood } = useMemoryApi();
  const neighborhood = ref<GraphNeighborhood | null>(null);
  const loadingGraph = ref(false);
  const graphError = ref('');

  async function loadNeighborhood(selectedId: string, hops: number) {
    if (!selectedId) return;
    loadingGraph.value = true;
    graphError.value = '';
    try {
      neighborhood.value = await getMemoryNeighborhood(selectedId, { hops, max_nodes: 48 });
    } catch (err) {
      neighborhood.value = null;
      graphError.value = err instanceof Error ? err.message : t('memory.graph.loadFailed');
    } finally {
      loadingGraph.value = false;
    }
  }

  function resetNeighborhood() {
    neighborhood.value = null;
    graphError.value = '';
  }

  return { neighborhood, loadingGraph, graphError, loadNeighborhood, resetNeighborhood };
}
