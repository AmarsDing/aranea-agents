import { computed, ref } from 'vue';
import { listIndustries, listDepartments, listPositions } from './api';
import type { Industry, Department, Position } from './types';
import { filterIndustries, summarizeIndustries, type IndustryFilters, type IndustrySummary } from './industryMarketFilters';

export type { IndustryFilters, IndustrySummary, IndustryStatusFilter, IndustrySourceFilter } from './industryMarketFilters';
export { filterIndustries, summarizeIndustries } from './industryMarketFilters';

export interface IndustryDetail {
  departments: Department[];
  positionsByDept: Record<string, Position[]>;
}

/**
 * 行业市场页的 composable
 *
 * - fetchIndustries: 拉取 industry 列表 + 并行 fetch 每行业的 departments/positions 填充 counts
 *   当前后端 list endpoint 不返回 deptCount/posCount，故前端并行拉取。3 行业 = 6 次请求，无明显开销。
 *   未来后端聚合时移除并行 fetch。
 * - fetchIndustryDetail: 拉取单个行业的部门+岗位详情（供 Drawer 使用）
 * - summary: KPI strip 用聚合
 */
export function useIndustryMarket() {
  const industries = ref<Industry[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);
  const detailLoading = ref(false);
  const industryDetail = ref<IndustryDetail>({ departments: [], positionsByDept: {} });

  async function fetchIndustries() {
    loading.value = true;
    error.value = null;
    try {
      const result = await listIndustries();
      const items = result.items;

      // 并行拉取每行业的部门/岗位数
      const enriched = await Promise.all(
        items.map(async (ind) => {
          try {
            const [deptRes, posRes] = await Promise.all([
              listDepartments(ind.key),
              listPositions(ind.key),
            ]);
            return {
              ...ind,
              deptCount: deptRes.items.length,
              posCount: posRes.items.length,
              // Agent 数 = 岗位数（MVP：1 岗 1 Agent）；未来支持 1:N 时改为后端聚合
              agentCount: posRes.items.length,
            } satisfies Industry;
          } catch {
            // 单个行业 fetch 失败不影响整体
            return ind;
          }
        }),
      );

      industries.value = enriched;
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : 'Failed to load industries';
    } finally {
      loading.value = false;
    }
  }

  /**
   * 拉取单个行业的部门+岗位详情，供 Drawer 展示
   */
  async function fetchIndustryDetail(industryKey: string) {
    detailLoading.value = true;
    try {
      const [deptRes, posRes] = await Promise.all([
        listDepartments(industryKey),
        listPositions(industryKey),
      ]);
      const grouped: Record<string, Position[]> = {};
      for (const pos of posRes.items) {
        (grouped[pos.department_key] ??= []).push(pos);
      }
      industryDetail.value = { departments: deptRes.items, positionsByDept: grouped };
    } catch {
      industryDetail.value = { departments: [], positionsByDept: {} };
    } finally {
      detailLoading.value = false;
    }
  }

  function clearIndustryDetail() {
    industryDetail.value = { departments: [], positionsByDept: {} };
  }

  const summary = computed<IndustrySummary>(() => summarizeIndustries(industries.value));

  /**
   * 应用筛选条件（页面级状态由调用方管理，本函数仅做纯计算）
   */
  function applyFilters(filters: IndustryFilters): Industry[] {
    return filterIndustries(industries.value, filters);
  }

  return { industries, loading, error, summary, fetchIndustries, applyFilters, industryDetail, detailLoading, fetchIndustryDetail, clearIndustryDetail };
}
