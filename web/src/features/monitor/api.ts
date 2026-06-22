import { createMonitorService } from '../../services/index';
import { GLOBAL_WS_SESSION_ID } from '../../config/runtime';
import type {
  AuditLog,
  AuditQuery,
  CodeExecutorCapability,
  MonitorTraceDetail,
  MonitorTracesQuery,
  MonitorTraceRow,
  MonitorAlertRule,
  MonitorLogLine,
  MonitorLogSnapshot,
  PaginatedResult,
  PlatformResource,
  RunnerMetricsSummary,
  SelfCheckReport,
  TeamRunEvent,
} from './types';

export type { CodeExecutorCapability } from './types';
import { useEnvelopeStream } from '../../realtime/useEnvelopeStream';
import type { Envelope } from '../../realtime/envelope';
import { flowSeverityToLevel, monitorLogLineFromFlowEnvelope } from './flow';
import { TEAM_RUNTIME_ENVELOPE_TYPES, teamRunEventFromEnvelope } from '../teams/teamRunEventFromEnvelope';

const monitor = createMonitorService();

export async function listFlowLogs(params: {
  traceId?: string;
  sessionId?: string;
  runId?: string;
  severity?: string;
  domain?: string;
  since?: string;
  until?: string;
  limit?: number;
  offset?: number;
}): Promise<{ items: MonitorLogLine[]; total: number }> {
  const data = await monitor.ListFlowLogs({
    traceId: params.traceId,
    sessionId: params.sessionId,
    runId: params.runId,
    severity: params.severity,
    domain: params.domain,
    since: params.since,
    until: params.until,
    limit: params.limit,
    offset: params.offset,
  });
  const items = (data.items ?? []).map((row) => {
    const r = obj(row);
    const severity = String(r.severity ?? 'info');
    return {
      id: String(r.id ?? ''),
      time: String(r.createdAt ?? r.created_at ?? ''),
      level: flowSeverityToLevel(severity),
      message: [r.title, r.message].filter(Boolean).join(' — ') || String(r.stepId ?? ''),
      source: String(r.agentKey ?? r.agent_key ?? 'flow'),
      created_at: String(r.createdAt ?? r.created_at ?? ''),
      kind: 'flow' as const,
      severity,
      title: String(r.title ?? ''),
      step_id: String(r.stepId ?? r.step_id ?? ''),
      trace_id: String(r.traceId ?? r.trace_id ?? ''),
      run_id: String(r.runId ?? r.run_id ?? ''),
      session_id: String(r.sessionId ?? r.session_id ?? ''),
    };
  });
  return { items, total: Number(data.total ?? items.length) };
}

function obj(v: unknown): Record<string, unknown> {
  return v !== null && typeof v === 'object' ? (v as Record<string, unknown>) : {};
}

function auditFromWire(raw: unknown): AuditLog {
  const r = obj(raw);
  return {
    id: String(r.id ?? ''),
    action: String(r.action ?? ''),
    resource: String(r.resource ?? ''),
    resource_id: String(r.resource_id ?? r.resourceId ?? ''),
    request_id: String(r.request_id ?? r.requestId ?? ''),
    detail: String(r.detail ?? ''),
    created_at: String(r.created_at ?? r.createdAt ?? ''),
    actor: String(r.actor ?? ''),
    ip: String(r.ip ?? ''),
    user_agent: String(r.user_agent ?? r.userAgent ?? ''),
    severity: String(r.severity ?? ''),
    metadata_json: String(r.metadata_json ?? r.metadataJson ?? ''),
  };
}

function platformResourceFromWire(raw: unknown): PlatformResource {
  const r = obj(raw);
  const resource = String(r.resource ?? '');
  const resourceKind = resource === 'monitor-traces' ? ('monitor-traces' as const) : ('monitor-events' as const);
  return {
    id: String(r.id ?? ''),
    resource: resourceKind,
    key: String(r.key ?? ''),
    name: String(r.name ?? ''),
    description: String(r.description ?? ''),
    status: String(r.status ?? ''),
    enabled: Boolean(r.enabled ?? true),
    sort_order: Number(r.sort_order ?? r.sortOrder ?? 0),
    parent_id: String(r.parent_id ?? r.parentId ?? ''),
    level: String(r.level ?? ''),
    agent_id: String(r.agent_id ?? r.agentId ?? ''),
    provider: String(r.provider ?? ''),
    model: String(r.model ?? ''),
    is_system: Boolean(r.is_system ?? r.isSystem ?? false),
    config_json: String(r.config_json ?? r.configJson ?? '{}'),
    metadata_json: String(r.metadata_json ?? r.metadataJson ?? '{}'),
    created_at: String(r.created_at ?? r.createdAt ?? ''),
    updated_at: String(r.updated_at ?? r.updatedAt ?? ''),
    deleted_at: String(r.deleted_at ?? r.deletedAt ?? ''),
  };
}

function logLineFromWire(raw: unknown): MonitorLogLine {
  const r = obj(raw);
  return {
    id: String(r.id ?? ''),
    time: String(r.time ?? ''),
    level: String(r.level ?? 'INFO') as MonitorLogLine['level'],
    message: String(r.message ?? ''),
    source: String(r.source ?? ''),
    created_at: String(r.created_at ?? r.createdAt ?? ''),
  };
}

export async function listMonitorAudit(query: AuditQuery = {}): Promise<PaginatedResult<AuditLog>> {
  const res = await monitor.ListAuditLogs({
    limit: query.limit ?? 200,
    offset: query.offset ?? 0,
    action: query.action ?? '',
    resource: query.resource ?? '',
    actor: query.actor ?? '',
    keyword: query.keyword ?? '',
  });
  const items = (res.items ?? []).map((item: unknown) => auditFromWire(item));
  return { items, total: Number(res.total ?? items.length) };
}

export async function listMonitorEvents(): Promise<PlatformResource[]> {
  const res = await monitor.ListMonitorEvents({
    limit: undefined,
    offset: undefined,
    eventType: undefined,
    agentId: undefined,
    status: undefined,
  });
  const items = res.items ?? [];
  return items.map((item: unknown) => platformResourceFromWire(item));
}

export async function getMonitorEvent(id: string): Promise<PlatformResource> {
  const row = await monitor.GetMonitorEvent({ id });
  return platformResourceFromWire(row as unknown);
}

export async function getMonitorLogs(): Promise<MonitorLogSnapshot> {
  const data = await monitor.GetMonitorLogs({});
  return {
    items: (data.items ?? []).map((line: unknown) => logLineFromWire(line)),
    enabled: Boolean(data.enabled),
    message: data.message ?? '',
  };
}

type MonitorWsSub = {
  close: () => void;
  connected: ReturnType<typeof useEnvelopeStream>['connected'];
  enableLog?: (enabled: boolean) => void;
};

function resolveMonitorSessionId(sessionId?: string): string {
  const trimmed = String(sessionId ?? '').trim();
  if (!trimmed || trimmed === 'monitor') {
    return GLOBAL_WS_SESSION_ID;
  }
  return trimmed;
}

/** Live monitor logs via `WS /v1/ws` (`session_id=*` for admin-wide stream). */
export function subscribeMonitorLogsWs(
  sessionId: string,
  onLine: (line: MonitorLogLine) => void,
  onError?: (error: string) => void,
  onConnected?: () => void,
): MonitorWsSub {
  const stream = useEnvelopeStream({
    sessionId: resolveMonitorSessionId(sessionId),
    channels: ['monitor', 'system'],
    autoConnect: false,
    logEnabled: false,
    onConnected: () => onConnected?.(),
  });

  stream.onType('flow_log', (env: Envelope) => {
    const line = monitorLogLineFromFlowEnvelope(env);
    if (line) onLine(line);
  });

  stream.onType('log', (env: Envelope) => {
    if (env.metadata?.flow_step || env.metadata?.schema_version === 'flow_log/v1') {
      return;
    }
    const level = (env.metadata?.level as MonitorLogLine['level']) ?? 'INFO';
    onLine({
      id: env.id,
      time: env.timestamp,
      level,
      message: env.content?.text ?? '',
      source: env.author ?? 'monitor',
      created_at: env.timestamp,
      kind: 'process',
    });
  });

  stream.onType('error', (env: Envelope) => {
    onError?.(env.error?.message ?? 'monitor ws error');
  });

  stream.connect();

  return {
    close: () => stream.disconnect(),
    connected: stream.connected,
    enableLog: (enabled: boolean) => stream.enableLog(enabled),
  };
}

export { createMonitorLogHub, useMonitorLogHub } from './useLogStreamHub';
export type { MonitorLogHub } from './useLogStreamHub';

/** Team / runtime monitor events via `WS /v1/ws` (global `session_id=*` by default). */
export function subscribeMonitorRuntimeEventsWs(
  sessionId: string,
  onEvent: (event: TeamRunEvent) => void,
  onError?: (error: string) => void,
  onConnected?: () => void,
  onDisconnected?: () => void,
): MonitorWsSub {
  const stream = useEnvelopeStream({
    sessionId: resolveMonitorSessionId(sessionId),
    channels: ['monitor', 'team', 'system'],
    autoConnect: false,
    logEnabled: false,
    onConnected: () => onConnected?.(),
    onDisconnected: () => onDisconnected?.(),
  });

  const dispatch = (env: Envelope) => {
    const mapped = teamRunEventFromEnvelope(env);
    if (mapped) {
      onEvent(mapped);
    }
  };

  stream.onType(TEAM_RUNTIME_ENVELOPE_TYPES, dispatch);
  stream.onType('log', dispatch);

  stream.onType('error', (env: Envelope) => {
    onError?.(env.error?.message ?? 'monitor ws error');
  });

  stream.connect();

  return {
    close: () => stream.disconnect(),
    connected: stream.connected,
  };
}

function traceRowFromWire(raw: unknown): MonitorTraceRow {
  const r = obj(raw);
  return {
    id: String(r.id ?? ''),
    resource: String(r.resource ?? 'monitor-traces'),
    key: String(r.key ?? ''),
    name: String(r.name ?? ''),
    description: String(r.description ?? ''),
    status: String(r.status ?? ''),
    enabled: Boolean(r.enabled ?? true),
    sort_order: Number(r.sort_order ?? r.sortOrder ?? 0),
    parent_id: String(r.parent_id ?? r.parentId ?? ''),
    level: String(r.level ?? ''),
    agent_id: String(r.agent_id ?? r.agentId ?? ''),
    provider: String(r.provider ?? ''),
    model: String(r.model ?? ''),
    config_json: String(r.config_json ?? r.configJson ?? '{}'),
    metadata_json: String(r.metadata_json ?? r.metadataJson ?? '{}'),
    created_at: String(r.created_at ?? r.createdAt ?? ''),
    updated_at: String(r.updated_at ?? r.updatedAt ?? ''),
    deleted_at: String(r.deleted_at ?? r.deletedAt ?? ''),
  };
}

export async function listMonitorTraces(query: MonitorTracesQuery = {}): Promise<PaginatedResult<MonitorTraceRow>> {
  const res = await monitor.ListMonitorTraces({
    limit: query.limit,
    offset: query.offset,
    agentId: query.agent_id,
    provider: query.provider,
    model: query.model,
    status: query.status,
  });
  const items = (res.items ?? []).map((item: unknown) => traceRowFromWire(item));
  return { items, total: Number(res.total ?? items.length) };
}

/** @deprecated Use listMonitorTraces instead. */
export async function listMonitorTraceEvents(query: MonitorTracesQuery = {}): Promise<MonitorTraceRow[]> {
  const result = await listMonitorTraces(query);
  return result.items;
}

function traceDetailFromWire(raw: unknown): MonitorTraceDetail {
  const r = obj(raw);
  const traceRaw = r.trace ?? r.Trace;
  return {
    trace: traceRowFromWire(traceRaw),
    config_json: String(r.config_json ?? r.configJson ?? '{}'),
    metadata_json: String(r.metadata_json ?? r.metadataJson ?? '{}'),
    spans_json: String(r.spans_json ?? r.spansJson ?? '[]'),
  };
}

export async function getMonitorTrace(id: string): Promise<MonitorTraceDetail> {
  const res = await monitor.GetMonitorTrace({ id });
  return traceDetailFromWire(res);
}

function alertRuleFromWire(raw: unknown): MonitorAlertRule {
  const r = obj(raw);
  return {
    id: String(r.id ?? ''),
    name: String(r.name ?? ''),
    metric_key: String(r.metric_key ?? r.metricKey ?? ''),
    threshold: Number(r.threshold ?? 0),
    window_minutes: Number(r.window_minutes ?? r.windowMinutes ?? 60),
    enabled: Boolean(r.enabled ?? true),
    severity: String(r.severity ?? 'warning'),
    notify_webhook_url: String(r.notify_webhook_url ?? r.notifyWebhookUrl ?? ''),
    notify_channel_id: String(r.notify_channel_id ?? r.notifyChannelId ?? ''),
    cooldown_minutes: Number(r.cooldown_minutes ?? r.cooldownMinutes ?? 60),
  };
}

export async function listMonitorAlertRules(): Promise<MonitorAlertRule[]> {
  const res = await monitor.ListMonitorAlertRules({});
  const items = (res as { items?: unknown[] }).items ?? [];
  return items.map(alertRuleFromWire);
}

export async function putMonitorAlertRules(rules: MonitorAlertRule[]): Promise<MonitorAlertRule[]> {
  const res = await monitor.PutMonitorAlertRules({
    items: rules.map((r) => ({
      id: r.id,
      name: r.name,
      metricKey: r.metric_key,
      threshold: r.threshold,
      windowMinutes: r.window_minutes,
      enabled: r.enabled,
      severity: r.severity,
      notifyWebhookUrl: r.notify_webhook_url ?? '',
      notifyChannelId: r.notify_channel_id ?? '',
      cooldownMinutes: r.cooldown_minutes ?? 60,
    })),
  });
  const items = (res as { items?: unknown[] }).items ?? [];
  return items.map(alertRuleFromWire);
}

export type { RunnerMetricsSummary } from './types';

export async function getRunnerMetrics(windowMinutes = 60): Promise<RunnerMetricsSummary> {
  const res = await monitor.GetRunnerMetrics({ windowMinutes });
  const r = obj(res);
  return {
    window_minutes: Number(r.window_minutes ?? r.windowMinutes ?? windowMinutes),
    total_runs: Number(r.total_runs ?? r.totalRuns ?? 0),
    error_runs: Number(r.error_runs ?? r.errorRuns ?? 0),
    error_rate: Number(r.error_rate ?? r.errorRate ?? 0),
    success_rate: Number(r.success_rate ?? r.successRate ?? 0),
    avg_duration_ms: r.avg_duration_ms != null ? Number(r.avg_duration_ms ?? r.avgDurationMs) : undefined,
    p50_duration_ms: r.p50_duration_ms != null ? Number(r.p50_duration_ms ?? r.p50DurationMs) : undefined,
    p95_duration_ms: r.p95_duration_ms != null ? Number(r.p95_duration_ms ?? r.p95DurationMs) : undefined,
    p99_duration_ms: r.p99_duration_ms != null ? Number(r.p99_duration_ms ?? r.p99DurationMs) : undefined,
  };
}

export async function getCodeExecutorCapabilities(): Promise<CodeExecutorCapability[]> {
  const res = await monitor.GetCodeExecutorCapabilities({});
  const backends = (res as { backends?: unknown[] }).backends ?? [];
  return backends.map((raw) => {
    const r = obj(raw);
    return {
      type: String(r.type ?? ''),
      available: Boolean(r.available ?? false),
      reason: String(r.reason ?? ''),
    };
  });
}

function selfCheckResultFromWire(raw: unknown): SelfCheckReport['check_results'][number] {
  const r = obj(raw);
  return {
    check_id: String(r.checkId ?? r.check_id ?? ''),
    checker: String(r.checker ?? ''),
    status: String(r.status ?? 'passed') as SelfCheckReport['check_results'][number]['status'],
    message: String(r.message ?? ''),
    details_json: String(r.detailsJson ?? r.details_json ?? ''),
    checked_at: String(r.checkedAt ?? r.checked_at ?? ''),
  };
}

function repairActionFromWire(raw: unknown): SelfCheckReport['repair_actions'][number] {
  const r = obj(raw);
  return {
    success: Boolean(r.success ?? false),
    action: String(r.action ?? ''),
    message: String(r.message ?? ''),
  };
}

function selfCheckReportFromWire(raw: unknown): SelfCheckReport {
  const r = obj(raw);
  const checkResults = (r.checkResults ?? r.check_results ?? []) as unknown[];
  const repairActions = (r.repairActions ?? r.repair_actions ?? []) as unknown[];
  return {
    id: String(r.id ?? ''),
    check_results: checkResults.map(selfCheckResultFromWire),
    overall_status: String(r.overallStatus ?? r.overall_status ?? 'passed') as SelfCheckReport['overall_status'],
    repair_actions: repairActions.map(repairActionFromWire),
    started_at: String(r.startedAt ?? r.started_at ?? ''),
    finished_at: String(r.finishedAt ?? r.finished_at ?? ''),
    duration_ms: Number(r.durationMs ?? r.duration_ms ?? 0),
  };
}

export async function triggerSelfCheck(): Promise<SelfCheckReport> {
  const res = await monitor.TriggerSelfCheck({});
  const report = (res as { report?: unknown }).report;
  return selfCheckReportFromWire(report);
}

export async function listSelfCheckReports(limit = 20, offset = 0): Promise<PaginatedResult<SelfCheckReport>> {
  const res = await monitor.ListSelfCheckReports({ limit, offset });
  const items = ((res as { items?: unknown[] }).items ?? []).map(selfCheckReportFromWire);
  return { items, total: Number((res as { total?: number }).total ?? items.length) };
}
