import type { Industry } from './types';

export type IndustryStatusFilter = 'all' | 'enabled' | 'disabled';
export type IndustrySourceFilter = 'all' | 'system' | 'custom';

export interface IndustryFilters {
  query: string;
  status: IndustryStatusFilter;
  source: IndustrySourceFilter;
}

/**
 * 行业市场页的搜索/筛选纯函数
 *
 * - query: 匹配 name / key / description（大小写不敏感，trim 后非空才参与）
 * - status: 'all' = 全部；'enabled' = enabled === true；'disabled' = enabled === false
 * - source: 'all' = 全部；'system' / 'custom' 依据 key 前缀（system: 由种子数据生成；custom: 用户自建 — 但当前 Industry 类型未带 source 字段，保留扩展点）
 *
 * 当前 Industry 类型不含 `source` 字段；该筛选项为未来扩展保留，传入 source !== 'all' 时对所有行业不命中（empty result），由调用方在 UI 上 disabled 该 chip。
 */
export function filterIndustries(industries: Industry[], filters: IndustryFilters): Industry[] {
  const q = filters.query.trim().toLowerCase();

  return industries.filter((ind) => {
    if (q) {
      const haystack = `${ind.name} ${ind.key} ${ind.description ?? ''}`.toLowerCase();
      if (!haystack.includes(q)) return false;
    }
    if (filters.status === 'enabled' && !ind.enabled) return false;
    if (filters.status === 'disabled' && ind.enabled) return false;
    // source filter: 未来 Industry 增 source 字段时启用；当前类型不含，所有行业都视为 system
    if (filters.source === 'custom') return false;
    return true;
  });
}

export interface IndustrySummary {
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
 * - departments / positions 来自 Industry.deptCount / posCount（可选，0 兜底）
 * - agents 来自 Industry.agentCount（可选；如缺省按 positions × 1 兜底，即 1 岗 1 Agent MVP 模型）
 * - installed 来自 Industry.installed（可选，0 兜底；当前类型不含，固定 0）
 */
export function summarizeIndustries(industries: Industry[]): IndustrySummary {
  const summary: IndustrySummary = {
    total: industries.length,
    enabled: industries.filter((i) => i.enabled).length,
    disabled: industries.filter((i) => !i.enabled).length,
    departments: 0,
    positions: 0,
    agents: 0,
    installed: 0,
  };
  for (const ind of industries) {
    summary.departments += ind.deptCount ?? 0;
    summary.positions += ind.posCount ?? 0;
    summary.agents += ind.agentCount ?? ind.posCount ?? 0; // fallback: 1 pos = 1 agent
    summary.installed += ind.installed ?? 0;
  }
  return summary;
}
