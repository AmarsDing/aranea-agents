import type { QTableColumn } from 'quasar';
import type { AuditLog, MonitorTrace } from '../../features/monitor/types';
import type { MonitorViewEvent } from '../../features/monitor/useMonitorRealtimeEvents';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../../features/ui/registryTableColumns';

/** vue-i18n t() 的最小签名（兼容带 named 参数调用） */
type Translate = (key: string, ...args: unknown[]) => string;

/** AuditTable 列定义 */
export function createAuditColumns(t: Translate): QTableColumn<AuditLog>[] {
  return [
    registryCol<AuditLog>('event', t('monitorPage.audit.colEvent'), 'action', 'left', REGISTRY_COL_W.name),
    registryCol<AuditLog>('resource', t('monitorPage.audit.colResource'), 'resource', 'left', REGISTRY_COL_W.name),
    registryCol<AuditLog>('actor', t('monitorPage.audit.colActor'), 'actor', 'left', REGISTRY_COL_W.agent),
    registryCol<AuditLog>('request', 'Request ID', 'request_id', 'left', REGISTRY_COL_W.agent),
    registryCol<AuditLog>('detail', t('monitorPage.audit.colDetail'), 'detail', 'left', REGISTRY_COL_W.contentWide),
    registryCol<AuditLog>('created', t('monitorPage.audit.colTime'), 'created_at', 'left', REGISTRY_COL_W.timeWide),
  ];
}

/** RealtimeEvents 列定义 */
export function createMonitorEventColumns(t: Translate): QTableColumn<MonitorViewEvent>[] {
  return [
    registryCol<MonitorViewEvent>('title', t('monitorPage.events.colTitle'), 'title', 'left', REGISTRY_COL_W.contentWide),
    registryCol<MonitorViewEvent>('tags', t('monitorPage.events.colType'), 'type', 'left', REGISTRY_COL_W.content),
    registryCol<MonitorViewEvent>('time', t('monitorPage.events.colTime'), 'time', 'left', REGISTRY_COL_W.agent),
    registryColActions<MonitorViewEvent>(REGISTRY_COL_W.actionsWide, ''),
  ];
}

/** TraceList 列定义 — Runs 列表（OPT-05 monitor_traces 真相源，含 Token/延迟/成本） */
export function createMonitorTraceColumns(t: Translate): QTableColumn<MonitorTrace>[] {
  return [
    registryCol<MonitorTrace>('name', t('monitorPage.traces.colName'), 'name', 'left', REGISTRY_COL_W.content),
    registryCol<MonitorTrace>('agent', t('monitorPage.traces.colAgent'), 'agent_id', 'left', REGISTRY_COL_W.name),
    registryCol<MonitorTrace>('model', t('monitorPage.traces.colModel'), 'model', 'left', REGISTRY_COL_W.name),
    registryCol<MonitorTrace>('tokens', t('monitorPage.traces.colTokens'), 'total_tokens', 'right', REGISTRY_COL_W.agent),
    registryCol<MonitorTrace>('latency', t('monitorPage.traces.colLatency'), 'duration_ms', 'right', REGISTRY_COL_W.agent),
    registryCol<MonitorTrace>('cost', t('monitorPage.traces.colCost'), 'total_cost_usd', 'right', REGISTRY_COL_W.agent),
    registryCol<MonitorTrace>('status', t('monitorPage.traces.colStatus'), 'status', 'center', REGISTRY_COL_W.status),
    registryCol<MonitorTrace>('time', t('monitorPage.traces.colTime'), 'created_at', 'left', REGISTRY_COL_W.nameWide),
    registryColActions<MonitorTrace>(REGISTRY_COL_W.traceAction, ''),
  ];
}

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

/** Trace 状态配色 — 语义色（running 为进行中蓝色，非错误红）；与语言无关 */
const TRACE_STATUS_COLORS: Record<string, string> = {
  running: 'info',
  ok: 'positive',
  success: 'positive',
  error: 'negative',
  cancelled: 'grey',
  timeout: 'orange',
  interrupted: 'warning',
};

/** Trace 状态 → i18n key（ok/success 共用 ok） */
const TRACE_STATUS_I18N_KEYS: Record<string, string> = {
  running: 'monitorPage.traces.status.running',
  ok: 'monitorPage.traces.status.ok',
  success: 'monitorPage.traces.status.ok',
  error: 'monitorPage.traces.status.error',
  cancelled: 'monitorPage.traces.status.cancelled',
  timeout: 'monitorPage.traces.status.timeout',
  interrupted: 'monitorPage.traces.status.interrupted',
};

/** Trace 状态颜色 */
export function traceStatusColor(status?: string): string {
  return TRACE_STATUS_COLORS[String(status ?? '')] ?? 'grey';
}

/** Trace 状态本地化标签（未知枚举回退原值） */
export function traceStatusLabel(t: Translate, status?: string): string {
  const s = String(status ?? '').trim();
  if (!s) return t('monitorPage.traces.unknownStatus');
  const key = TRACE_STATUS_I18N_KEYS[s];
  return key ? t(key) : s;
}

/** Trace 域（运行类型）配色；与语言无关 */
const TRACE_DOMAIN_COLORS: Record<string, string> = {
  chat: 'blue-grey',
  team: 'deep-purple',
  graph: 'teal',
};

/** Trace 域颜色 */
export function traceDomainColor(domain?: string): string {
  return TRACE_DOMAIN_COLORS[String(domain ?? '').trim()] ?? 'blue-grey';
}

/** Trace 域本地化标签（未知域回退原值，空串返回空） */
export function traceDomainLabel(t: Translate, domain?: string): string {
  const d = String(domain ?? '').trim();
  if (!d) return '';
  if (!(d in TRACE_DOMAIN_COLORS)) return d;
  return t(`monitorPage.traces.domain.${d}`);
}
