import { computed, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { toolInvocationStatusOptions } from '../../components/tools/toolUi';
import type { ToolInvocation, ToolInvocationParamDetail } from './types';
import { useToolsStore } from '../../stores/tools';

export function useToolRunsPage() {
  const route = useRoute();
  const { t } = useI18n();
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
  const statusOptions = [...toolInvocationStatusOptions];

  // 请求序号守卫：筛选连改/翻页叠加时丢弃过期响应，避免旧数据覆盖新筛选结果。
  let loadSeq = 0;

  async function loadRows() {
    const seq = ++loadSeq;
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
      if (seq !== loadSeq) return;
      rows.value = data.items;
      total.value = data.total;
    } catch (err) {
      if (seq !== loadSeq) return;
      error.value = err instanceof Error ? err.message : t('toolsPage.runs.loadFailed');
    } finally {
      if (seq === loadSeq) loading.value = false;
    }
  }

  function resetFilters() {
    // 合并 watch 后同一 tick 的多次赋值只触发一次 loadRows。
    toolKey.value = '';
    agentId.value = '';
    sessionId.value = '';
    status.value = '';
    hasError.value = false;
    from.value = '';
    page.value = 1;
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
      detailParamsError.value = t('toolsPage.runs.missingInvocationId');
      return;
    }
    detailParamsLoading.value = true;
    try {
      detailParams.value = await toolsStore.fetchInvocationParams(row.invocation_id);
    } catch (err) {
      detailParamsError.value = err instanceof Error ? err.message : t('toolsPage.runs.paramsLoadFailed');
    } finally {
      detailParamsLoading.value = false;
    }
  }

  // 单 watch 合并筛选 + 分页：筛选变化先归一到第 1 页（page 变化复用同一 watch），避免双 watch 重复请求。
  watch([toolKey, agentId, sessionId, status, hasError, from, page, pageSize], (newVals, oldVals) => {
    const filtersChanged = newVals.slice(0, 6).some((v, i) => v !== oldVals[i]);
    if (filtersChanged && page.value !== 1) {
      page.value = 1;
      return;
    }
    void loadRows();
  });

  onMounted(() => {
    // 路由 query 预置会触发上方 watch 完成首次加载，无需重复请求。
    const hasQuery = typeof route.query.tool_key === 'string' || typeof route.query.session_id === 'string';
    if (typeof route.query.tool_key === 'string') {
      toolKey.value = route.query.tool_key;
    }
    if (typeof route.query.session_id === 'string') {
      sessionId.value = route.query.session_id;
    }
    if (!hasQuery) {
      void loadRows();
    }
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
