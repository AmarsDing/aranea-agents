import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { toolInvocationStatusOptions } from '../../components/tools/toolUi';
import type { ToolInvocationAudit } from './types';
import { useToolsStore } from '../../stores/tools';

export function useToolAuditsPage() {
  const { t } = useI18n();
  const toolsStore = useToolsStore();

  const toolKey = ref('');
  const agentId = ref('');
  const userId = ref('');
  const status = ref('');
  const page = ref(1);
  const pageSize = ref(20);
  const rows = ref<ToolInvocationAudit[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const error = ref('');

  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
  const statusOptions = [...toolInvocationStatusOptions];

  // 请求序号守卫：筛选连改/翻页叠加时丢弃过期响应，避免旧数据覆盖新筛选结果。
  let loadSeq = 0;

  async function loadRows() {
    const seq = ++loadSeq;
    loading.value = true;
    error.value = '';
    try {
      const data = await toolsStore.loadToolAudits({
        tool_key: toolKey.value,
        agent_id: agentId.value,
        user_id: userId.value,
        status: status.value,
        page: page.value,
        page_size: pageSize.value,
      });
      if (seq !== loadSeq) return;
      rows.value = data.items;
      total.value = data.total;
    } catch (err) {
      if (seq !== loadSeq) return;
      error.value = err instanceof Error ? err.message : t('toolsPage.audits.loadFailed');
    } finally {
      if (seq === loadSeq) loading.value = false;
    }
  }

  function resetFilters() {
    // 合并 watch 后同一 tick 的多次赋值只触发一次 loadRows。
    toolKey.value = '';
    agentId.value = '';
    userId.value = '';
    status.value = '';
    page.value = 1;
  }

  // 单 watch 合并筛选 + 分页：筛选变化先归一到第 1 页（page 变化复用同一 watch），避免双 watch 重复请求。
  watch([toolKey, agentId, userId, status, page, pageSize], (newVals, oldVals) => {
    const filtersChanged = newVals.slice(0, 4).some((v, i) => v !== oldVals[i]);
    if (filtersChanged && page.value !== 1) {
      page.value = 1;
      return;
    }
    void loadRows();
  });

  onMounted(() => {
    void loadRows();
  });

  return {
    toolKey,
    agentId,
    userId,
    status,
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
