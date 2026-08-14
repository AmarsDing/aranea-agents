import { computed, onMounted, ref, watch, type Ref, type WatchSource } from 'vue';

/**
 * 通用列表页分页 + 筛选联动 composable。
 *
 * 提取多个 use*Page.ts 中重复的分页状态与 watch 逻辑：
 * - page / pageSize / pageMax 状态
 * - 筛选变更 → page 归 1 → 触发加载（若已是第 1 页则直接加载）
 * - page / pageSize 变更 → 触发加载
 * - onMounted → 首次加载（可选关闭）
 *
 * 使用方负责：定义筛选 ref、实现 load 函数（读取当前 page/pageSize）、管理 error/loading。
 */
export function useListPage(opts: {
  /** 筛选条件 watch 源列表 — 变更时重置 page=1 并触发 load */
  filters: WatchSource[];
  /** 总条数（响应式，通常来自 store 或本地 ref） */
  total: Ref<number>;
  /** 加载函数 — 内部应读取 page.value / pageSize.value */
  load: () => Promise<void> | void;
  /** 初始 pageSize（默认 20） */
  defaultPageSize?: number;
  /** 是否在 onMounted 时自动调用 load（默认 true） */
  autoLoad?: boolean;
}) {
  const page = ref(1);
  const pageSize = ref(opts.defaultPageSize ?? 20);
  const pageMax = computed(() => Math.max(1, Math.ceil(Math.max(0, opts.total.value) / pageSize.value)));

  // 筛选变更 → page 归 1（若已是 1 则直接加载；否则由 page watch 触发加载，避免重复请求）
  watch(opts.filters, () => {
    if (page.value === 1) {
      void opts.load();
    } else {
      page.value = 1;
    }
  });

  // page / pageSize 变更 → 加载
  watch([page, pageSize], () => {
    void opts.load();
  });

  if (opts.autoLoad !== false) {
    onMounted(() => void opts.load());
  }

  return { page, pageSize, pageMax };
}
