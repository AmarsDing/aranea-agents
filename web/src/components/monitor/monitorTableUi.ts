import type { QTableColumn } from 'quasar';
import type { AuditLog, MonitorTraceEvent } from '../../features/monitor/types';
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
  registryCol<MonitorViewEvent>('title', '事件', 'title', 'left', '28%'),
  registryCol<MonitorViewEvent>('tags', '类型', 'type', 'left', '25%'),
  registryCol<MonitorViewEvent>('time', '时间', 'time', 'left', '10%'),
  registryColActions<MonitorViewEvent>(REGISTRY_COL_W.actionsWide, ''),
];

/** TraceList 列定义 */
export const MONITOR_TRACES_TABLE_COLUMNS: QTableColumn<MonitorTraceEvent>[] = [
  registryCol<MonitorTraceEvent>('name', 'Agent', 'agent_key', 'left', '25%'),
  registryCol<MonitorTraceEvent>('tokens', 'Token 入/出', 'total_tokens', 'left', '18%'),
  registryCol<MonitorTraceEvent>('latency', '延迟', 'latency_ms', 'left', '18%'),
  registryCol<MonitorTraceEvent>('cost', '费用', 'total_cost_micro_usd', 'left', '18%'),
  registryCol<MonitorTraceEvent>('time', '时间', 'occurred_at', 'left', '18%'),
  registryColActions<MonitorTraceEvent>('30px', ''),
];

/** Trace 状态颜色 */
export function traceStatusColor(status?: string) {
  if (status === 'ok' || status === 'success') return 'positive';
  if (status === 'cancelled') return 'grey';
  if (status === 'timeout') return 'orange';
  return 'negative';
}
