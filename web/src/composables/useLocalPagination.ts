import { computed, ref, watch, type Ref, type ComputedRef } from 'vue';

/**
 * 本地筛选 + 分页 composable。
 * 适用于前端筛选/分页场景（数据量不大，无需服务端分页）。
 */
export function useLocalPagination<T>(options: {
  rows: Ref<T[]> | ComputedRef<T[]> | (() => T[]);
  filter?: Ref<string> | ComputedRef<string> | (() => string);
  filterFn?: (row: T, keyword: string) => boolean;
  defaultRowsPerPage?: number;
}) {
  const page = ref(1);
  const rowsPerPage = ref(options.defaultRowsPerPage ?? 20);

  const resolveRows = () => ('value' in options.rows ? options.rows.value : options.rows());
  const resolveFilter = () => {
    if (!options.filter) return '';
    return 'value' in options.filter ? options.filter.value : options.filter();
  };

  const filteredRows = computed(() => {
    const rows = resolveRows();
    const keyword = resolveFilter();
    if (!keyword || !options.filterFn) return rows;
    return rows.filter((row) => options.filterFn!(row, keyword));
  });

  const pagedRows = computed(() => {
    const start = (page.value - 1) * rowsPerPage.value;
    return filteredRows.value.slice(start, start + rowsPerPage.value);
  });

  const totalPages = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / rowsPerPage.value)));

  // 筛选变化时自动重置页码
  if (options.filter && 'value' in options.filter) {
    watch(options.filter, () => {
      page.value = 1;
    });
  }

  return { page, rowsPerPage, filteredRows, pagedRows, totalPages };
}
