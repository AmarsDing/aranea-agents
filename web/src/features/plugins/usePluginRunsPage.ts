import { computed, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  CALLBACK_POINT_OPTIONS,
  PLUGIN_RUN_KEY_PRESETS,
  PLUGIN_RUN_STATUS_OPTIONS,
  pluginRunsQueryFromRoute,
} from '../callback/constants';
import type { PluginRun } from './types';

import { PLUGIN_RUN_TABLE_COLUMNS } from './pluginRunsTableUi';
import { usePluginsStore } from '../../stores/plugins';

export function usePluginRunsPage() {
  const route = useRoute();
  const router = useRouter();
  const pluginsStore = usePluginsStore();

  const pluginKey = ref('');
  const agentId = ref('');
  const callbackPoint = ref('');
  const status = ref('');
  const from = ref('');
  const to = ref('');
  const rows = ref<PluginRun[]>([]);
  const loading = ref(false);
  const error = ref('');
  const total = ref(0);
  const page = ref(1);
  const pageSize = ref(20);
  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
  const detailOpen = ref(false);
  const detailText = ref('');

  const callbackPointOptions = CALLBACK_POINT_OPTIONS;
  const statusOptions = PLUGIN_RUN_STATUS_OPTIONS;
  const pluginKeyOptions = ref([...PLUGIN_RUN_KEY_PRESETS.map((p) => p.value)]);

  const columns = PLUGIN_RUN_TABLE_COLUMNS;

  function filterPluginKeys(val: string, update: (fn: () => void) => void) {
    update(() => {
      const needle = val.toLowerCase();
      pluginKeyOptions.value = PLUGIN_RUN_KEY_PRESETS.map((p) => p.value).filter((k) =>
        k.toLowerCase().includes(needle),
      );
    });
  }

  function statusColor(st: string) {
    if (st === 'blocked') return 'orange';
    if (st === 'error') return 'negative';
    return 'positive';
  }

  function toRFC3339(local: string): string | undefined {
    const t = local.trim();
    if (!t) return undefined;
    const d = new Date(t);
    if (Number.isNaN(d.getTime())) return undefined;
    return d.toISOString();
  }

  async function loadRows(nextPage = page.value, nextPageSize = pageSize.value) {
    loading.value = true;
    error.value = '';
    try {
      const data = await pluginsStore.loadPluginRuns({
        plugin_key: pluginKey.value.trim() || undefined,
        agent_id: agentId.value.trim() || undefined,
        callback_point: callbackPoint.value || undefined,
        status: status.value || undefined,
        from: toRFC3339(from.value),
        to: toRFC3339(to.value),
        page: nextPage,
        page_size: nextPageSize,
      });
      rows.value = data.items;
      total.value = data.total;
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载运行记录失败';
    } finally {
      loading.value = false;
    }
  }

  function onFilterChange() {
    if (page.value === 1) {
      void loadRows(1, pageSize.value);
    } else {
      page.value = 1;
    }
    syncQueryToRoute();
  }

  function syncQueryToRoute() {
    const query: Record<string, string> = {};
    if (pluginKey.value) query.plugin_key = pluginKey.value;
    if (agentId.value) query.agent_id = agentId.value;
    if (callbackPoint.value) query.callback_point = callbackPoint.value;
    if (status.value) query.status = status.value;
    if (from.value) query.from = from.value;
    if (to.value) query.to = to.value;
    void router.replace({ query });
  }

  function resetFilters() {
    pluginKey.value = '';
    agentId.value = '';
    callbackPoint.value = '';
    status.value = '';
    from.value = '';
    to.value = '';
    page.value = 1;
    void router.replace({ query: {} });
    void loadRows(1, pageSize.value);
  }

  function openDetail(row: PluginRun) {
    detailText.value = detailPreview(row) || JSON.stringify(row, null, 2);
    detailOpen.value = true;
  }

  function detailPreview(row: PluginRun) {
    return row.detail_json?.trim() || '';
  }

  function applyRouteQuery() {
    const q = pluginRunsQueryFromRoute(route.query as Record<string, unknown>);
    if (q.plugin_key) pluginKey.value = q.plugin_key;
    if (q.agent_id) agentId.value = q.agent_id;
    if (q.callback_point) callbackPoint.value = q.callback_point;
    if (q.status) status.value = q.status;
    if (q.from) from.value = toDatetimeLocal(q.from);
    if (q.to) to.value = toDatetimeLocal(q.to);
  }

  function toDatetimeLocal(iso: string): string {
    if (!iso) return '';
    const match = iso.match(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}/);
    return match ? match[0] : iso.slice(0, 16);
  }

  watch([page, pageSize], () => void loadRows());

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
    total,
    page,
    pageSize,
    pageMax,
    detailOpen,
    detailText,
    callbackPointOptions,
    statusOptions,
    pluginKeyOptions,
    columns,
    filterPluginKeys,
    statusColor,
    loadRows,
    onFilterChange,
    resetFilters,
    openDetail,
    detailPreview,
  };
}
