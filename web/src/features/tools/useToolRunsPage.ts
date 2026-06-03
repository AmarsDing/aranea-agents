import { computed, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { toolInvocationStatusOptions } from '../../components/tools/toolUi';
import type { ToolInvocation } from './types';
import { useToolsStore } from '../../stores/tools';

export function useToolRunsPage() {
  const route = useRoute();
  const toolsStore = useToolsStore();

  const toolKey = ref('');
  const agentId = ref('');
  const status = ref('');
  const from = ref('');
  const page = ref(1);
  const pageSize = ref(20);
  const rows = ref<ToolInvocation[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const error = ref('');

  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
  const statusOptions = [...toolInvocationStatusOptions, { label: '失败', value: 'failed' }];

  async function loadRows() {
    loading.value = true;
    error.value = '';
    try {
      const data = await toolsStore.loadToolRuns({
        tool_key: toolKey.value,
        agent_id: agentId.value,
        status: status.value,
        from: from.value,
        page: page.value,
        page_size: pageSize.value,
      });
      rows.value = data.items;
      total.value = data.total;
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载调用记录失败';
    } finally {
      loading.value = false;
    }
  }

  function resetFilters() {
    toolKey.value = '';
    agentId.value = '';
    status.value = '';
    from.value = '';
    page.value = 1;
    void loadRows();
  }

  watch([toolKey, agentId, status, from], () => {
    page.value = 1;
    void loadRows();
  });
  watch([page, pageSize], () => {
    void loadRows();
  });

  onMounted(() => {
    if (typeof route.query.tool_key === 'string') {
      toolKey.value = route.query.tool_key;
    }
    void loadRows();
  });

  return {
    toolKey,
    agentId,
    status,
    from,
    page,
    pageSize,
    rows,
    total,
    loading,
    error,
    pageMax,
    statusOptions,
    loadRows,
    resetFilters,
  };
}
