import { onMounted, ref } from "vue";
import { useRoute } from "vue-router";
import type { QTableColumn, QTableProps } from "quasar";
import {
  CALLBACK_POINT_OPTIONS,
  PLUGIN_RUN_KEY_PRESETS,
  PLUGIN_RUN_STATUS_OPTIONS,
  pluginRunsQueryFromRoute
} from "../callback/constants";
import type { PluginRun } from "./types";
import { registryCol } from "../ui/registryTableColumns";
import { usePluginsStore } from "../../stores/plugins";

export function usePluginRunsPage() {
  const route = useRoute();
  const pluginsStore = usePluginsStore();

  const pluginKey = ref("");
  const agentId = ref("");
  const callbackPoint = ref("");
  const status = ref("");
  const from = ref("");
  const to = ref("");
  const rows = ref<PluginRun[]>([]);
  const loading = ref(false);
  const error = ref("");
  const tablePagination = ref({ page: 1, rowsPerPage: 20, rowsNumber: 0 });
  const detailOpen = ref(false);
  const detailText = ref("");

  const callbackPointOptions = CALLBACK_POINT_OPTIONS;
  const statusOptions = PLUGIN_RUN_STATUS_OPTIONS;
  const pluginKeyOptions = ref([...PLUGIN_RUN_KEY_PRESETS.map((p) => p.value)]);

  const columns: QTableColumn<PluginRun>[] = [
    { name: "created_at", label: "时间", field: "created_at", align: "left", ...registryCol.time },
    { name: "plugin_key", label: "Plugin / Hook", field: "plugin_key", align: "left", ...registryCol.plugin },
    { name: "agent_id", label: "Agent", field: "agent_id", align: "left", ...registryCol.agent },
    { name: "callback_point", label: "生命周期点", field: "callback_point", align: "left", ...registryCol.phase },
    { name: "status", label: "结果", field: "status", align: "left", ...registryCol.status },
    { name: "duration_ms", label: "耗时(ms)", field: "duration_ms", align: "right", ...registryCol.duration },
    { name: "detail_json", label: "摘要", field: "detail_json", align: "right", ...registryCol.actions }
  ];

  function filterPluginKeys(val: string, update: (fn: () => void) => void) {
    update(() => {
      const needle = val.toLowerCase();
      pluginKeyOptions.value = PLUGIN_RUN_KEY_PRESETS.map((p) => p.value).filter((k) => k.toLowerCase().includes(needle));
    });
  }

  function statusColor(st: string) {
    if (st === "blocked") return "orange";
    if (st === "error") return "negative";
    return "positive";
  }

  function toRFC3339(local: string): string | undefined {
    const t = local.trim();
    if (!t) return undefined;
    const d = new Date(t);
    if (Number.isNaN(d.getTime())) return undefined;
    return d.toISOString();
  }

  async function loadRows(page = tablePagination.value.page, pageSize = tablePagination.value.rowsPerPage) {
    loading.value = true;
    error.value = "";
    try {
      const data = await pluginsStore.loadPluginRuns({
        plugin_key: pluginKey.value.trim() || undefined,
        agent_id: agentId.value.trim() || undefined,
        callback_point: callbackPoint.value || undefined,
        status: status.value || undefined,
        from: toRFC3339(from.value),
        to: toRFC3339(to.value),
        page,
        page_size: pageSize
      });
      rows.value = data.items;
      tablePagination.value = { ...tablePagination.value, page, rowsPerPage: pageSize, rowsNumber: data.total };
    } catch (err) {
      error.value = err instanceof Error ? err.message : "加载运行记录失败";
    } finally {
      loading.value = false;
    }
  }

  const onTableRequest: QTableProps["onRequest"] = (props) => {
    void loadRows(props.pagination.page, props.pagination.rowsPerPage);
  };

  function onFilterChange() {
    tablePagination.value.page = 1;
    void loadRows(1, tablePagination.value.rowsPerPage);
  }

  function resetFilters() {
    pluginKey.value = "";
    agentId.value = "";
    callbackPoint.value = "";
    status.value = "";
    from.value = "";
    to.value = "";
    tablePagination.value.page = 1;
    void loadRows(1, tablePagination.value.rowsPerPage);
  }

  function openDetail(row: PluginRun) {
    detailText.value = row.detail_json?.trim() ? row.detail_json : JSON.stringify(row, null, 2);
    detailOpen.value = true;
  }

  function applyRouteQuery() {
    const q = pluginRunsQueryFromRoute(route.query as Record<string, unknown>);
    if (q.plugin_key) pluginKey.value = q.plugin_key;
    if (q.agent_id) agentId.value = q.agent_id;
    if (q.callback_point) callbackPoint.value = q.callback_point;
    if (q.status) status.value = q.status;
  }

  onMounted(() => {
    applyRouteQuery();
    void loadRows();
  });

  return {
    pluginKey,
    agentId,
    callbackPoint,
    status,
    from,
    to,
    rows,
    loading,
    error,
    tablePagination,
    detailOpen,
    detailText,
    callbackPointOptions,
    statusOptions,
    pluginKeyOptions,
    columns,
    filterPluginKeys,
    statusColor,
    loadRows,
    onTableRequest,
    onFilterChange,
    resetFilters,
    openDetail
  };
}
