import type { TeamRunEvent } from '../teams/types';
import type { PlatformResource } from '../platform/types';

export type { PlatformResource, TeamRunEvent };

/** Monitor trace row — mirrors backend MonitorPlatformRow from ListMonitorTraces. */
export type MonitorTrace = {
  id: string;
  resource: string;
  key: string;
  name: string;
  description: string;
  status: string;
  enabled: boolean;
  sort_order: number;
  parent_id: string;
  level: string;
  agent_id: string;
  provider: string;
  model: string;
  config_json: string;
  metadata_json: string;
  created_at: string;
  updated_at: string;
  deleted_at: string;
  /** Resolved display names joined from agents/teams (traces only); '' when dangling. */
  agent_name: string;
  team_name: string;
  /** Correlation keys (traces only); '' for event rows. Used by detail dialog flow-log queries. */
  session_id: string;
  run_id: string;
  /** Denormalized from config_json for table columns (OPT-05 Runs metrics). */
  duration_ms?: number;
  total_tokens?: number;
  total_cost_usd?: number;
  /** Original stored domain (chat/team/graph/...) from config_json; shown as type badge. */
  domain?: string;
};

/** Monitor trace detail — mirrors backend GetMonitorTrace response. */
export type MonitorTraceDetail = {
  trace: MonitorTrace;
  config_json: string;
  metadata_json: string;
  spans_json: string;
};

/** Alias for MonitorTrace — use MonitorTraceRow when the context is specifically a trace list row. */
export type MonitorTraceRow = MonitorTrace;

/**
 * @deprecated Use MonitorTraceRow (or MonitorTrace) instead. This type was incorrectly based on
 * ModelTokenUsageEvent and will be removed in a future release.
 */
export type MonitorTraceEvent = MonitorTraceRow;

export type AuditLog = {
  id: string;
  action: string;
  resource: string;
  resource_id: string;
  request_id: string;
  detail: string;
  created_at: string;
  actor: string;
  ip: string;
  user_agent: string;
  severity: string;
  metadata_json: string;
};

export type AuditQuery = {
  limit?: number;
  offset?: number;
  action?: string;
  resource?: string;
  actor?: string;
  keyword?: string;
  /** 隐藏系统噪音（sync.* 动作）；显式 action 过滤时忽略 */
  exclude_system?: boolean;
};

export type MonitorEventsQuery = {
  limit?: number;
  offset?: number;
  event_type?: string;
  agent_id?: string;
  status?: string;
  /** 前缀匹配任一（与 event_type 并集） */
  event_types?: string[];
  /** 前缀排除（如 ['skill.filesystem.'] 隐藏治理噪音） */
  exclude_event_types?: string[];
};

export type MonitorTracesQuery = {
  limit?: number;
  offset?: number;
  agent_id?: string;
  provider?: string;
  model?: string;
  status?: string;
  /** 服务端子串搜索（name/trace_key/agent_id/provider/model + 显示名） */
  keyword?: string;
  /** 隐藏内部域（system/skill：cron、skill 同步等高频噪音） */
  exclude_internal?: boolean;
  /** 精确运行域过滤（chat/team/graph/system/skill），优先于 exclude_internal */
  domain?: string;
};

export type PaginatedResult<T> = {
  items: T[];
  total: number;
};

/** Runs 列表响应：分页 + 筛选 chips 计数（各自忽略自身维度的过滤条件） */
export type MonitorTraceListResult = PaginatedResult<MonitorTraceRow> & {
  status_counts: Record<string, number>;
  domain_counts: Record<string, number>;
};

export type MonitorLogLine = {
  id: string;
  time: string;
  level: 'DEBUG' | 'INFO' | 'WARN' | 'ERROR' | string;
  message: string;
  source: string;
  created_at: string;
  /** flow_log (v2) vs gateway process log */
  kind?: 'flow' | 'process';
  severity?: string;
  title?: string;
  step_id?: string;
  trace_id?: string;
  run_id?: string;
  session_id?: string;
  hint?: string;
};

export type MonitorLogSnapshot = {
  items: MonitorLogLine[];
  enabled: boolean;
  message: string;
};

export type LoadState = 'idle' | 'loading' | 'success' | 'empty' | 'error';
export type StreamState = 'connecting' | 'connected' | 'live' | 'paused' | 'error';

export type RunnerMetricsSummary = {
  window_minutes: number;
  total_runs: number;
  error_runs: number;
  error_rate: number;
  success_rate: number;
  avg_duration_ms?: number;
  p50_duration_ms?: number;
  p95_duration_ms?: number;
  p99_duration_ms?: number;
};

export type CodeExecutorCapability = {
  type: string;
  available: boolean;
  reason?: string;
};

export type MonitorAlertRule = {
  id: string;
  name: string;
  metric_key: string;
  threshold: number;
  window_minutes: number;
  enabled: boolean;
  severity: string;
  notify_webhook_url?: string;
  notify_channel_id?: string;
  cooldown_minutes?: number;
};

/** Alert metric directory entry — mirrors backend AlertMetricInfo from ListAlertMetrics. */
export type AlertMetricInfo = {
  /** Technical key, e.g. "runner.error_rate". */
  key: string;
  /** Short English display name; UI localizes known keys and falls back to this. */
  name: string;
  /** What the metric measures and when it fires. */
  description: string;
  /** "ratio" (0..1) or "count". */
  unit: string;
  default_window_minutes: number;
  suggested_threshold: number;
  /** Value evaluated at request time over the default window. */
  current_value: number;
  evaluated_at: string;
};

// Self-check types

export type SelfCheckStatus = 'passed' | 'warning' | 'failed';

export type SelfCheckResult = {
  check_id: string;
  checker: string;
  status: SelfCheckStatus;
  message: string;
  details_json?: string;
  checked_at: string;
};

export type RepairAction = {
  success: boolean;
  action: string;
  message: string;
};

export type SelfCheckReport = {
  id: string;
  check_results: SelfCheckResult[];
  overall_status: SelfCheckStatus;
  repair_actions: RepairAction[];
  started_at: string;
  finished_at: string;
  duration_ms: number;
};
