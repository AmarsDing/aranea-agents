import { ref, computed, onMounted, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useGraphStore } from '../../stores/graph';
import { storeToRefs } from 'pinia';
import { useRoute, useRouter } from 'vue-router';
import { timeRangeStart } from './graphExecutionsUi';

export function useGraphExecutionsPage() {
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const graphStore = useGraphStore();
  const { executionHistory, executionHistoryLoading, executionHistoryNextToken } = storeToRefs(graphStore);
  const { loadExecutionHistory, fetchGraph } = graphStore;

  const isDark = computed(() => $q.dark.isActive);
  const graphId = ref(route.params.id as string);
  const graphName = ref('');
  const statusFilter = ref('');
  const timeRangeFilter = ref('');

  const serverFilters = computed(() => ({
    status: statusFilter.value || undefined,
    startedAfter: timeRangeStart(timeRangeFilter.value)?.toISOString(),
  }));

  const filteredHistory = computed(() => {
    let list = executionHistory.value;
    if (statusFilter.value) {
      list = list.filter((e) => e.status === statusFilter.value);
    }
    const rangeStart = timeRangeStart(timeRangeFilter.value);
    if (rangeStart) {
      list = list.filter((e) => {
        if (!e.startedAt) return false;
        return new Date(e.startedAt).getTime() >= rangeStart.getTime();
      });
    }
    return list;
  });

  async function reload() {
    await loadExecutionHistory(graphId.value, 30, false, serverFilters.value);
  }

  onMounted(async () => {
    try {
      const g = await fetchGraph(graphId.value);
      graphName.value = g?.name ?? graphId.value;
    } catch {
      graphName.value = graphId.value;
    }
    await reload();
  });

  watch(
    () => route.params.id,
    (newId) => {
      if (newId && typeof newId === 'string') {
        graphId.value = newId;
        reload();
      }
    },
  );

  watch([statusFilter, timeRangeFilter], () => {
    reload();
  });

  function loadMore() {
    if (executionHistoryNextToken.value) {
      loadExecutionHistory(graphId.value, 30, true, serverFilters.value);
    }
  }

  function goToRun(executionId: string) {
    router.push({ name: 'graph-run', params: { id: graphId.value, execId: executionId } });
  }

  function goBack() {
    router.push({ name: 'graphs' });
  }

  return {
    isDark,
    graphId,
    graphName,
    executionHistory,
    filteredHistory,
    loading: executionHistoryLoading,
    hasNextPage: executionHistoryNextToken,
    statusFilter,
    timeRangeFilter,
    loadMore,
    goToRun,
    goBack,
  };
}
