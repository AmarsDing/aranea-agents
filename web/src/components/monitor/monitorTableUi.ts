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

/** TraceList 列定义 */
export const MONITOR_TRACES_TABLE_COLUMNS: QTableColumn<MonitorTrace>[] = [
  registryCol<MonitorTrace>('name', '名称', 'name', 'left', REGISTRY_COL_W.content),
  registryCol<MonitorTrace>('agent', 'Agent', 'agent_id', 'left', REGISTRY_COL_W.nameWide),
  registryCol<MonitorTrace>('provider', 'Provider', 'provider', 'left', REGISTRY_COL_W.nameWide),
  registryCol<MonitorTrace>('model', '模型', 'model', 'left', REGISTRY_COL_W.nameWide),
  registryCol<MonitorTrace>('time', '时间', 'created_at', 'left', REGISTRY_COL_W.nameWide),
  registryColActions<MonitorTrace>(REGISTRY_COL_W.traceAction, ''),
];

/** Trace 状态颜色 */
export function traceStatusColor(status?: string) {
  if (status === 'ok' || status === 'success') return 'positive';
  if (status === 'cancelled') return 'grey';
  if (status === 'timeout') return 'orange';
  return 'negative';
}
