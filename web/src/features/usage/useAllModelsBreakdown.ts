/**
 * useAllModelsBreakdown —— 全模型消耗总览表的 composable。
 *
 * 职责：
 *   - 维护分页/搜索/排序状态（page、pageSize、search、sortBy）
 *   - 监听父级 range + provider 变化时自动重新查询（通过 watch）
 *   - 调用 listAllModelsBreakdown API 并暴露 rows/loading/error
 *
 * 与 useOverviewPage 的关系：
 *   - range 和 provider_code 从父组件 props 传入，不在此处管理
 *   - 此 composable 只管理表格内部状态（分页、搜索、排序）
 *
 * 设计决策（避免 double-fetch）：
 *   - onRequest (QTable 分页/排序变化) → 显式调用 loadBreakdown()
 *   - commitSearch (搜索 debounce) → 显式调用 loadBreakdown()
 *   - watch(range, providerCode) (父级筛选变化) → 显式调用 loadBreakdown()
 *   - 不使用 watch(search, sortBy) 避免与 onRequest 的显式调用重复触发
 */
import { ref, watch, computed } from 'vue';
import { listAllModelsBreakdown } from './api';
import type { ModelUsageBreakdownRow } from './types';

export type BreakdownSortBy = {
  field: string; // snake_case: call_count / total_tokens / total_cost_micro_usd / success_rate / avg_latency_ms
  direction: 'asc' | 'desc';
};

export type UseAllModelsBreakdownOptions = {
  range: () => string; // 返回当前 range（来自父组件）
  providerCode: () => string; // 返回当前 provider_code（来自父组件）
  initialPageSize?: number; // 默认 10
};

export function useAllModelsBreakdown(opts: UseAllModelsBreakdownOptions) {
  const rows = ref<ModelUsageBreakdownRow[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const error = ref<string | null>(null);

  const page = ref(1);
  const pageSize = ref(opts.initialPageSize ?? 10);
  const search = ref('');
  const sortBy = ref<BreakdownSortBy>({ field: 'total_cost_micro_usd', direction: 'desc' });

  // debounce search: store pending input separately so typing doesn't trigger per-keystroke fetch
  const searchInput = ref('');
  let searchTimer: ReturnType<typeof setTimeout> | null = null;

  function commitSearch() {
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = setTimeout(() => {
      const next = searchInput.value.trim();
      const changed = next !== search.value;
      search.value = next;
      page.value = 1; // 搜索时重置到第一页
      // 即使 search.value 没变（用户重输相同关键词），也强制重新查询，
      // 确保 page=1 重置后 UI 数据与分页状态一致。
      void loadBreakdown();
      void changed; // 显式标注：变更检测仅用于调试，不参与控制流
    }, 350);
  }

  function onSearchInput() {
    commitSearch();
  }

  // Quasar QTable onRequest 兼容的请求处理器。
  // payload 命名避免与外层 props 混淆。
  function onRequest(payload: {
    pagination: { page: number; rowsPerPage: number; sortBy: string; descending: boolean };
  }) {
    const p = payload.pagination;
    page.value = p.page;
    pageSize.value = p.rowsPerPage === 0 ? 100 : p.rowsPerPage; // 0 = "all" in Quasar, cap at 100
    if (p.sortBy) {
      sortBy.value = {
        field: p.sortBy,
        direction: p.descending ? 'desc' : 'asc',
      };
    }
    void loadBreakdown();
  }

  async function loadBreakdown() {
    loading.value = true;
    error.value = null;
    try {
      const result = await listAllModelsBreakdown({
        range: opts.range() || undefined,
        provider_code: opts.providerCode() || undefined,
        search: search.value || undefined,
        sort_field: sortBy.value.field,
        sort_dir: sortBy.value.direction,
        page: page.value,
        page_size: pageSize.value,
      });
      rows.value = result.items;
      total.value = result.total;
      // 后端可能规范化了 page/pageSize（shared.PageToLimitOffset），这里同步回前端状态
      if (result.page > 0) page.value = result.page;
      if (result.page_size > 0) pageSize.value = result.page_size;
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
      rows.value = [];
      total.value = 0;
    } finally {
      loading.value = false;
    }
  }

  // 监听父级 range/provider 变化 → 重置到第一页并重新查询
  watch([opts.range, opts.providerCode], () => {
    page.value = 1;
    void loadBreakdown();
  });

  // 初始加载（watch 默认不 immediate，所以需要显式触发一次）
  void loadBreakdown();

  // Quasar QTable 的 pagination 对象（双向同步）
  const qPagination = computed(() => ({
    page: page.value,
    rowsPerPage: pageSize.value,
    sortBy: sortBy.value.field,
    descending: sortBy.value.direction === 'desc',
    rowsNumber: total.value,
  }));

  return {
    rows,
    total,
    loading,
    error,
    page,
    pageSize,
    search,
    searchInput,
    sortBy,
    qPagination,
    onSearchInput,
    onRequest,
    reload: loadBreakdown,
  };
}
