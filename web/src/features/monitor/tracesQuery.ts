/**
 * Runs（Traces）列表查询与实时事件纯函数 —— 无 Store/Vue 依赖，可单测。
 *
 * 状态模型（方案 C3）：
 * - 默认视图排除内部域（system/skill：cron、skill 同步等高频噪音），只显示「对话 + 团队」等业务运行；
 * - 显式选择 domain chip 时优先于 exclude_internal（与后端 monitorTracesWhere 同语义），
 *   使内部域仍可达（排障场景）；
 * - status_counts / domain_counts 由服务端在「各自忽略自身维度过滤」下聚合，喂给筛选 chips。
 */
import type { MonitorTracesQuery } from './types';

/** 运行域筛选：'' = 默认视图（排除内部噪音）；显式值精确过滤且优先于 exclude_internal */
export const TRACE_DOMAIN_FILTERS = ['', 'chat', 'team', 'graph', 'system', 'skill'] as const;

/** 状态筛选：'' = 全部；其余取值对齐后端 monitor_traces.status 枚举 */
export const TRACE_STATUS_FILTERS = ['', 'running', 'ok', 'error', 'timeout', 'interrupted', 'cancelled'] as const;

export type MonitorTracesFilters = {
  keyword: string;
  status: string;
  /** '' = 默认（排除内部域）；显式 domain 优先于 exclude_internal */
  domain: string;
  page: number;
  pageSize: number;
};

/** 组装 Runs 服务端查询（纯函数）：显式 domain 优先，默认视图 exclude_internal=true */
export function buildMonitorTracesQuery(f: MonitorTracesFilters): MonitorTracesQuery {
  const domain = String(f.domain || '').trim();
  const query: MonitorTracesQuery = {
    limit: f.pageSize,
    offset: Math.max(0, (f.page - 1) * f.pageSize),
    keyword: String(f.keyword || '').trim(),
    status: String(f.status || '').trim(),
  };
  if (domain) {
    query.domain = domain;
  } else {
    query.exclude_internal = true;
  }
  return query;
}

/** 触发 Runs 刷新的 WS 事件类型（run_finished = chat runner completion 的 Activity 映射） */
export const RUN_LIVE_REFRESH_EVENT_TYPES = [
  'run_finished',
  'runner_completion',
  'team_run_started',
  'team_run_finished',
  'team_run_failed',
] as const;

/** 事件类型是否为运行生命周期事件（新运行出现 / 运行终结 → 列表需刷新） */
export function isRunLifecycleEventType(type: string): boolean {
  return (RUN_LIVE_REFRESH_EVENT_TYPES as readonly string[]).includes(String(type || '').trim());
}
