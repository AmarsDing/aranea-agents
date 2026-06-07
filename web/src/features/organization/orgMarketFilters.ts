import type { Company } from './types';

export type OrgStatusFilter = 'all' | 'enabled' | 'disabled';
export type OrgSourceFilter = 'all' | 'system' | 'custom';

export interface OrgFilters {
  query: string;
  status: OrgStatusFilter;
  source: OrgSourceFilter;
}

/**
 * 组织市场页的搜索/筛选纯函数
 *
 * - query: 匹配 name / key / description（大小写不敏感，trim 后非空才参与）
 * - status: 'all' = 全部；'enabled' = enabled === true；'disabled' = enabled === false
 * - source: 'all' = 全部；'system' / 'custom' 依据 key 前缀（system: 由种子数据生成；custom: 用户自建 — 但当前 Company 类型未带 source 字段，保留扩展点）
 *
 * 当前 Company 类型不含 `source` 字段；该筛选项为未来扩展保留，传入 source !== 'all' 时对所有公司不命中（empty result），由调用方在 UI 上 disabled 该 chip。
 */
export function filterCompanies(companies: Company[], filters: OrgFilters): Company[] {
  const q = filters.query.trim().toLowerCase();

  return companies.filter((comp) => {
    if (q) {
      const haystack = `${comp.name} ${comp.key} ${comp.description ?? ''}`.toLowerCase();
      if (!haystack.includes(q)) return false;
    }
    if (filters.status === 'enabled' && !comp.enabled) return false;
    if (filters.status === 'disabled' && comp.enabled) return false;
    // source filter: 未来 Company 增 source 字段时启用；当前类型不含，所有公司都视为 system
    if (filters.source === 'custom') return false;
    return true;
  });
}

export interface OrgSummary {
  total: number;
  enabled: number;
  disabled: number;
  departments: number;
  positions: number;
  agents: number;
  installed: number;
}

/**
 * 顶部 KPI strip 数据聚合
 *
 * - departments / positions 来自 Company.deptCount / posCount（可选，0 兜底）
 * - agents 来自 Company.agentCount（可选；如缺省按 positions × 1 兜底，即 1 岗 1 Agent MVP 模型）
 * - installed 来自 Company.installed（可选，0 兜底；当前类型不含，固定 0）
 */
export function summarizeCompanies(companies: Company[]): OrgSummary {
  const summary: OrgSummary = {
    total: companies.length,
    enabled: companies.filter((c) => c.enabled).length,
    disabled: companies.filter((c) => !c.enabled).length,
    departments: 0,
    positions: 0,
    agents: 0,
    installed: 0,
  };
  for (const comp of companies) {
    summary.departments += comp.deptCount ?? 0;
    summary.positions += comp.posCount ?? 0;
    summary.agents += comp.agentCount ?? comp.posCount ?? 0; // fallback: 1 pos = 1 agent
    summary.installed += comp.installed ?? 0;
  }
  return summary;
}
