import { computed, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { toolInvocationStatusOptions } from '../../components/tools/toolUi';
import { getToolInvocationParams } from './api';
import type { ToolInvocation, ToolInvocationParamDetail } from './types';
import { useToolsStore } from '../../stores/tools';

export function useToolRunsPage() {
  const route = useRoute();
  const toolsStore = useToolsStore();

  const toolKey = ref('');
  const agentId = ref('');
  const sessionId = ref('');
  const status = ref('');
  const hasError = ref(false);
  const from = ref('');
  const page = ref(1);
  const pageSize = ref(20);
  const rows = ref<ToolInvocation[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const error = ref('');

  // 详情弹窗
  const detailOpen = ref(false);
  const detailRow = ref<ToolInvocation | null>(null);
  const detailParams = ref<ToolInvocationParamDetail | null>(null);
  const detailParamsLoading = ref(false);
  const detailParamsError = ref('');

  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
  const statusOptions = [...toolInvocationStatusOptions, { label: '失败', value: 'failed' }];

  async function loadRows() {
    loading.value = true;
    error.value = '';
    try {
      const data = await toolsStore.loadToolRuns({
        tool_key: toolKey.value,
        agent_id: agentId.value,
        session_id: sessionId.value,
        status: status.value,
        has_error: hasError.value || undefined,
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
    sessionId.value = '';
    status.value = '';
    hasError.value = false;
    from.value = '';
    page.value = 1;
    void loadRows();
  }

  function openDetail(row: ToolInvocation) {
    detailRow.value = row;
    detailOpen.value = true;
    void loadDetailParams(row);
  }

  async function loadDetailParams(row: ToolInvocation) {
    detailParams.value = null;
    detailParamsError.value = '';
    if (!row.invocation_id) {
      detailParamsError.value = '该记录缺少 invocation_id，无法加载参数详情';
      return;
    }
    detailParamsLoading.value = true;
    try {
      detailParams.value = await getToolInvocationParams(row.invocation_id);
    } catch (err) {
      detailParamsError.value = err instanceof Error ? err.message : '加载参数详情失败';
    } finally {
      detailParamsLoading.value = false;
    }
  }

  watch([toolKey, agentId, sessionId, status, hasError, from], () => {
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
    if (typeof route.query.session_id === 'string') {
      sessionId.value = route.query.session_id;
    }
    void loadRows();
  });

  return {
    toolKey,
    agentId,
    sessionId,
    status,
    hasError,
    from,
    page,
    pageSize,
    rows,
    total,
    loading,
    error,
    pageMax,
    statusOptions,
    detailOpen,
    detailRow,
    detailParams,
    detailParamsLoading,
    detailParamsError,
    openDetail,
    loadRows,
    resetFilters,
  };
}
