import type { QTableColumn } from 'quasar';
import type { AuditLog, MonitorTrace } from '../../features/monitor/types';
import type { MonitorViewEvent } from '../../features/monitor/useMonitorRealtimeEvents';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../../features/ui/registryTableColumns';

/** AuditTable 列定义 */
export const AUDIT_TABLE_COLUMNS: QTableColumn<AuditLog>[] = [
  registryCol<AuditLog>('event', '事件', 'action', 'left', REGISTRY_COL_W.nameWide),
  registryCol<AuditLog>('resource', '实体', 'resource', 'left', REGISTRY_COL_W.name),
  registryCol<AuditLog>('actor', '操作者', 'actor', 'left', REGISTRY_COL_W.agent),
  registryCol<AuditLog>('request', 'Request ID', 'request_id', 'left', REGISTRY_COL_W.agent),
  registryCol<AuditLog>('created', '时间', 'created_at', 'left', REGISTRY_COL_W.timeWide),
];

/** RealtimeEvents 列定义 */
export const MONITOR_EVENTS_TABLE_COLUMNS: QTableColumn<MonitorViewEvent>[] = [
  registryCol<MonitorViewEvent>('title', '事件', 'title', 'left', REGISTRY_COL_W.contentWide),
  registryCol<MonitorViewEvent>('tags', '类型', 'type', 'left', REGISTRY_COL_W.content),
  registryCol<MonitorViewEvent>('time', '时间', 'time', 'left', REGISTRY_COL_W.agent),
  registryColActions<MonitorViewEvent>(REGISTRY_COL_W.actionsWide, ''),
];

/** TraceList 列定义 — Runs 列表（OPT-05 monitor_traces 真相源，含 Token/延迟/成本） */
export const MONITOR_TRACES_TABLE_COLUMNS: QTableColumn<MonitorTrace>[] = [
  registryCol<MonitorTrace>('name', '名称', 'name', 'left', REGISTRY_COL_W.content),
  registryCol<MonitorTrace>('agent', 'Agent', 'agent_id', 'left', REGISTRY_COL_W.nameWide),
  registryCol<MonitorTrace>('provider', 'Provider', 'provider', 'left', REGISTRY_COL_W.nameWide),
  registryCol<MonitorTrace>('model', '模型', 'model', 'left', REGISTRY_COL_W.nameWide),
  registryCol<MonitorTrace>('tokens', 'Tokens', 'total_tokens', 'right', REGISTRY_COL_W.agent),
  registryCol<MonitorTrace>('latency', '延迟', 'duration_ms', 'right', REGISTRY_COL_W.agent),
  registryCol<MonitorTrace>('cost', '成本', 'total_cost_usd', 'right', REGISTRY_COL_W.agent),
  registryCol<MonitorTrace>('time', '时间', 'created_at', 'left', REGISTRY_COL_W.nameWide),
  registryColActions<MonitorTrace>(REGISTRY_COL_W.traceAction, ''),
];

/** Extract run metrics packed into config_json by ListMonitorTraces. */
export function traceRunMetrics(row: {
  config_json?: string;
  duration_ms?: number;
  total_tokens?: number;
  total_cost_usd?: number;
}): { duration_ms: number; total_tokens: number; total_cost_usd: number } {
  if (typeof row.duration_ms === 'number' || typeof row.total_tokens === 'number') {
    return {
      duration_ms: Number(row.duration_ms ?? 0),
      total_tokens: Number(row.total_tokens ?? 0),
      total_cost_usd: Number(row.total_cost_usd ?? 0),
    };
  }
  try {
    const cfg = JSON.parse(row.config_json || '{}') as Record<string, unknown>;
    return {
      duration_ms: Number(cfg.duration_ms ?? 0),
      total_tokens: Number(cfg.total_tokens ?? 0),
      total_cost_usd: Number(cfg.total_cost_usd ?? 0),
    };
  } catch {
    return { duration_ms: 0, total_tokens: 0, total_cost_usd: 0 };
  }
}

/** Trace 状态颜色 */
export function traceStatusColor(status?: string) {
  if (status === 'ok' || status === 'success') return 'positive';
  if (status === 'cancelled') return 'grey';
  if (status === 'timeout') return 'orange';
  return 'negative';
}
