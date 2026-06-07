import { computed, ref } from 'vue';
import { listCompanies, listDepartments, listPositions, invalidateCache } from './api';
import type { Company, Department, Position } from './types';
import {
  filterCompanies,
  summarizeCompanies,
  type OrgFilters,
  type OrgSummary,
} from './orgMarketFilters';

export type {
  OrgFilters,
  OrgSummary,
  OrgStatusFilter,
  OrgSourceFilter,
} from './orgMarketFilters';
export { filterCompanies, summarizeCompanies } from './orgMarketFilters';

export interface OrgDetail {
  departments: Department[];
  positionsByDept: Record<string, Position[]>;
}

/**
 * 组织市场页的 composable
 *
 * - fetchCompanies: 拉取 company 列表 + 并行 fetch 每公司的 departments/positions 填充 counts
 *   当前后端 list endpoint 不返回 deptCount/posCount，故前端并行拉取。3 公司 = 6 次请求，无明显开销。
 *   未来后端聚合时移除并行 fetch。
 * - fetchOrgDetail: 拉取单个公司的部门+岗位详情（供 Drawer 使用）
 * - summary: KPI strip 用聚合
 */
export function useOrgMarket() {
  const companies = ref<Company[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);
  const detailLoading = ref(false);
  const orgDetail = ref<OrgDetail>({ departments: [], positionsByDept: {} });

  async function fetchCompanies() {
    loading.value = true;
    error.value = null;
    invalidateCache(); // 清除 organization 缓存以获取最新数据
    try {
      const result = await listCompanies();
      const items = result.items;

      // 并行拉取每公司的部门/岗位数
      const enriched = await Promise.all(
        items.map(async (comp) => {
          try {
            const [deptRes, posRes] = await Promise.all([listDepartments(comp.key), listPositions(comp.key)]);
            return {
              ...comp,
              deptCount: deptRes.items.length,
              posCount: posRes.items.length,
              // Agent 数 = 岗位数（MVP：1 岗 1 Agent）；未来支持 1:N 时改为后端聚合
              agentCount: posRes.items.length,
            } satisfies Company;
          } catch {
            // 单个公司 fetch 失败不影响整体
            return comp;
          }
        }),
      );

      companies.value = enriched;
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : 'Failed to load companies';
    } finally {
      loading.value = false;
    }
  }

  /**
   * 拉取单个公司的部门+岗位详情，供 Drawer 展示
   */
  async function fetchOrgDetail(companyKey: string) {
    detailLoading.value = true;
    try {
      const [deptRes, posRes] = await Promise.all([listDepartments(companyKey), listPositions(companyKey)]);
      const grouped: Record<string, Position[]> = {};
      for (const pos of posRes.items) {
        (grouped[pos.department_key] ??= []).push(pos);
      }
      orgDetail.value = { departments: deptRes.items, positionsByDept: grouped };
    } catch {
      orgDetail.value = { departments: [], positionsByDept: {} };
    } finally {
      detailLoading.value = false;
    }
  }

  function clearOrgDetail() {
    orgDetail.value = { departments: [], positionsByDept: {} };
  }

  const summary = computed<OrgSummary>(() => summarizeCompanies(companies.value));

  /**
   * 应用筛选条件（页面级状态由调用方管理，本函数仅做纯计算）
   */
  function applyFilters(filters: OrgFilters): Company[] {
    return filterCompanies(companies.value, filters);
  }

  return {
    companies,
    loading,
    error,
    summary,
    fetchCompanies,
    applyFilters,
    orgDetail,
    detailLoading,
    fetchOrgDetail,
    clearOrgDetail,
  };
}
