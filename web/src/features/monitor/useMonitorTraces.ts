/**
 * Runs（Traces）列表状态 composable —— 服务端分页/筛选/搜索 + 筛选 chips 计数。
 * 查询组装与筛选常量见 tracesQuery.ts（纯函数，已单测）。
 */
import { computed, ref, watch } from 'vue';
import { useMonitorStore } from '../../stores/monitor';
import type { MonitorTrace } from './types';
import { MONITOR_TRACES_PAGE_SIZE } from '../constants/queryLimits';
import { buildMonitorTracesQuery } from './tracesQuery';

export function useMonitorTraces() {
  const monitorStore = useMonitorStore();

  // ── 列表数据 + 筛选 chips 计数 ──
  const rows = ref<MonitorTrace[]>([]);
  const total = ref(0);
  const statusCounts = ref<Record<string, number>>({});
  const domainCounts = ref<Record<string, number>>({});
  const loading = ref(false);

  // ── 筛选/分页状态 ──
  const keyword = ref('');
  const status = ref('');
  const domain = ref('');
  const page = ref(1);
  const pageSize = ref(MONITOR_TRACES_PAGE_SIZE);

  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

  async function refresh() {
    loading.value = true;
    try {
      const result = await monitorStore.fetchTraceEvents(
        buildMonitorTracesQuery({
          keyword: keyword.value,
          status: status.value,
          domain: domain.value,
          page: page.value,
          pageSize: pageSize.value,
        }),
      );
      rows.value = result.items;
      total.value = result.total;
      statusCounts.value = result.status_counts;
      domainCounts.value = result.domain_counts;
    } finally {
      loading.value = false;
    }
  }

  // 单一 watcher：过滤条件变化先归第 1 页（链式触发本轮查询），翻页/页大小变化仅重新查询——
  // 避免「过滤 + 非第 1 页」时双 watcher 各发一次相同请求（同 useMonitorRealtimeEvents 模式）。
  // 初始加载由 Page 的 loadAll 负责（与 Audit/Events 并发语义一致）。
  watch([keyword, status, domain, page, pageSize], ([kw, st, dm], [oldKw, oldSt, oldDm]) => {
    const filterChanged = kw !== oldKw || st !== oldSt || dm !== oldDm;
    if (filterChanged && page.value !== 1) {
      page.value = 1;
      return;
    }
    void refresh();
  });

  // 外部数据收缩（如 WS 实时刷新后总数减少）导致页码越界时收敛到末页。
  watch(pageMax, (max) => {
    if (page.value > max) page.value = max;
  });

  function resetFilters() {
    keyword.value = '';
    status.value = '';
    domain.value = '';
    page.value = 1;
  }

  return {
    rows,
    total,
    statusCounts,
    domainCounts,
    loading,
    keyword,
    status,
    domain,
    page,
    pageSize,
    pageMax,
    refresh,
    resetFilters,
  };
}
