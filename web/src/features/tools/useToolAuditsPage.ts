import { computed, onMounted, ref, watch } from "vue";
import { toolInvocationStatusOptions } from "../../components/tools/toolUi";
import type { ToolInvocationAudit } from "./types";
import { useToolsStore } from "../../stores/tools";

export function useToolAuditsPage() {
  const toolsStore = useToolsStore();

  const toolKey = ref("");
  const agentId = ref("");
  const userId = ref("");
  const status = ref("");
  const page = ref(1);
  const pageSize = ref(20);
  const rows = ref<ToolInvocationAudit[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const error = ref("");

  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
  const statusOptions = [...toolInvocationStatusOptions];

  async function loadRows() {
    loading.value = true;
    error.value = "";
    try {
      const data = await toolsStore.loadToolAudits({
        tool_key: toolKey.value,
        agent_id: agentId.value,
        user_id: userId.value,
        status: status.value,
        page: page.value,
        page_size: pageSize.value
      });
      rows.value = data.items;
      total.value = data.total;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "加载审计日志失败";
    } finally {
      loading.value = false;
    }
  }

  function resetFilters() {
    toolKey.value = "";
    agentId.value = "";
    userId.value = "";
    status.value = "";
    page.value = 1;
    void loadRows();
  }

  watch([page, pageSize], () => {
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
    resetFilters
  };
}
