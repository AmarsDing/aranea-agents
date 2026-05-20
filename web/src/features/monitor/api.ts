import { createMonitorService } from "../../services/index";
import { GLOBAL_WS_SESSION_ID } from "../../config/runtime";
import type {
  AuditLog,
  AuditQuery,
  ModelUsageQuery,
  MonitorAlertRule,
  MonitorLogLine,
  MonitorLogSnapshot,
  MonitorTraceEvent,
  PaginatedResult,
  PlatformResource,
  TeamRunEvent
} from "./types";
import { listModelUsageEvents } from "../usage/api";
import { useEnvelopeStream } from "../chat/useEnvelopeStream";
import type { Envelope, EnvelopeType } from "../chat/envelope";
import { monitorLogLineFromFlowEnvelope } from "./flow";
import { TEAM_RUNTIME_ENVELOPE_TYPES, teamRunEventFromEnvelope } from "../teams/teamRunEventFromEnvelope";

const monitor = createMonitorService();

function obj(v: unknown): Record<string, unknown> {
  return v !== null && typeof v === "object" ? (v as Record<string, unknown>) : {};
}

function auditFromWire(raw: unknown): AuditLog {
  const r = obj(raw);
  return {
    id: String(r.id ?? ""),
    action: String(r.action ?? ""),
    resource: String(r.resource ?? ""),
    resource_id: String(r.resource_id ?? r.resourceId ?? ""),
    request_id: String(r.request_id ?? r.requestId ?? ""),
    detail: String(r.detail ?? ""),
    created_at: String(r.created_at ?? r.createdAt ?? ""),
    actor: String(r.actor ?? ""),
    ip: String(r.ip ?? ""),
    user_agent: String(r.user_agent ?? r.userAgent ?? ""),
    severity: String(r.severity ?? ""),
    metadata_json: String(r.metadata_json ?? r.metadataJson ?? "")
  };
}

function platformResourceFromWire(raw: unknown): PlatformResource {
  const r = obj(raw);
  const resource = String(r.resource ?? "");
  const resourceKind =
    resource === "monitor-traces" ? ("monitor-traces" as const) : ("monitor-events" as const);
  return {
    id: String(r.id ?? ""),
    resource: resourceKind,
    key: String(r.key ?? ""),
    name: String(r.name ?? ""),
    description: String(r.description ?? ""),
    status: String(r.status ?? ""),
    enabled: Boolean(r.enabled ?? true),
    sort_order: Number(r.sort_order ?? r.sortOrder ?? 0),
    parent_id: String(r.parent_id ?? r.parentId ?? ""),
    level: String(r.level ?? ""),
    agent_id: String(r.agent_id ?? r.agentId ?? ""),
    provider: String(r.provider ?? ""),
    model: String(r.model ?? ""),
    config_json: String(r.config_json ?? r.configJson ?? "{}"),
    metadata_json: String(r.metadata_json ?? r.metadataJson ?? "{}"),
    created_at: String(r.created_at ?? r.createdAt ?? ""),
    updated_at: String(r.updated_at ?? r.updatedAt ?? ""),
    deleted_at: String(r.deleted_at ?? r.deletedAt ?? "")
  };
}

function logLineFromWire(raw: unknown): MonitorLogLine {
  const r = obj(raw);
  return {
    id: String(r.id ?? ""),
    time: String(r.time ?? ""),
    level: String(r.level ?? "INFO") as MonitorLogLine["level"],
    message: String(r.message ?? ""),
    source: String(r.source ?? ""),
    created_at: String(r.created_at ?? r.createdAt ?? "")
  };
}

export async function listMonitorAudit(query: AuditQuery = {}): Promise<PaginatedResult<AuditLog>> {
  const res = await monitor.ListAuditLogs({
    limit: query.limit ?? 200,
    offset: query.offset ?? 0,
    action: query.action ?? "",
    resource: query.resource ?? "",
    actor: query.actor ?? "",
    keyword: query.keyword ?? ""
  });
  const items = (res.items ?? []).map((item: unknown) => auditFromWire(item));
  return { items, total: Number(res.total ?? items.length) };
}

export async function listMonitorEvents(): Promise<PlatformResource[]> {
  const res = await monitor.ListMonitorEvents({});
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
    message: data.message ?? ""
  };
}

type MonitorWsSub = {
  close: () => void;
  connected: ReturnType<typeof useEnvelopeStream>["connected"];
  enableLog?: (enabled: boolean) => void;
};

function resolveMonitorSessionId(sessionId?: string): string {
  const trimmed = String(sessionId ?? "").trim();
  if (!trimmed || trimmed === "monitor") {
    return GLOBAL_WS_SESSION_ID;
  }
  return trimmed;
}

/** Live monitor logs via `WS /v1/ws` (`session_id=*` for admin-wide stream). */
export function subscribeMonitorLogsWs(
  sessionId: string,
  onLine: (line: MonitorLogLine) => void,
  onError?: (error: string) => void,
  onConnected?: () => void
): MonitorWsSub {
  const stream = useEnvelopeStream({
    sessionId: resolveMonitorSessionId(sessionId),
    channels: ["monitor", "system"],
    autoConnect: false,
    logEnabled: false,
  });

  stream.onType("flow_log", (env: Envelope) => {
    const line = monitorLogLineFromFlowEnvelope(env);
    if (line) onLine(line);
  });

  stream.onType("log", (env: Envelope) => {
    if (env.metadata?.flow_step || env.metadata?.schema_version === "flow_log/v1") {
      return;
    }
    const level = (env.metadata?.level as MonitorLogLine["level"]) ?? "INFO";
    onLine({
      id: env.id,
      time: env.timestamp,
      level,
      message: env.content?.text ?? "",
      source: env.author ?? "monitor",
      created_at: env.timestamp,
      kind: "process"
    });
  });

  stream.onType("error", (env: Envelope) => {
    onError?.(env.error?.message ?? "monitor ws error");
  });

  stream.onType("connected" as EnvelopeType, () => {
    onConnected?.();
  });

  stream.connect();

  return {
    close: () => stream.disconnect(),
    connected: stream.connected,
    enableLog: (enabled: boolean) => stream.enableLog(enabled),
  };
}

/** Team / runtime monitor events via `WS /v1/ws` (global `session_id=*` by default). */
export function subscribeMonitorRuntimeEventsWs(
  sessionId: string,
  onEvent: (event: TeamRunEvent) => void,
  onError?: (error: string) => void
): MonitorWsSub {
  const stream = useEnvelopeStream({
    sessionId: resolveMonitorSessionId(sessionId),
    channels: ["monitor", "team", "system"],
    autoConnect: false,
    logEnabled: false,
  });

  const dispatch = (env: Envelope) => {
    const mapped = teamRunEventFromEnvelope(env);
    if (mapped) {
      onEvent(mapped);
    }
  };

  stream.onType(TEAM_RUNTIME_ENVELOPE_TYPES, dispatch);
  stream.onType("log", dispatch);

  stream.onType("error", (env: Envelope) => {
    onError?.(env.error?.message ?? "monitor ws error");
  });

  stream.connect();

  return {
    close: () => stream.disconnect(),
    connected: stream.connected,
  };
}

export async function listMonitorTraceEvents(query: ModelUsageQuery = {}): Promise<MonitorTraceEvent[]> {
  const rows = await listModelUsageEvents(query);
  return rows as MonitorTraceEvent[];
}

function alertRuleFromWire(raw: unknown): MonitorAlertRule {
  const r = obj(raw);
  return {
    id: String(r.id ?? ""),
    name: String(r.name ?? ""),
    metric_key: String(r.metric_key ?? r.metricKey ?? ""),
    threshold: Number(r.threshold ?? 0),
    window_minutes: Number(r.window_minutes ?? r.windowMinutes ?? 60),
    enabled: Boolean(r.enabled ?? true),
    severity: String(r.severity ?? "warning"),
    notify_webhook_url: String(r.notify_webhook_url ?? r.notifyWebhookUrl ?? ""),
    notify_channel_id: String(r.notify_channel_id ?? r.notifyChannelId ?? ""),
    cooldown_minutes: Number(r.cooldown_minutes ?? r.cooldownMinutes ?? 60)
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
      metric_key: r.metric_key,
      threshold: r.threshold,
      window_minutes: r.window_minutes,
      enabled: r.enabled,
      severity: r.severity,
      notify_webhook_url: r.notify_webhook_url ?? "",
      notify_channel_id: r.notify_channel_id ?? "",
      cooldown_minutes: r.cooldown_minutes ?? 60
    }))
  });
  const items = (res as { items?: unknown[] }).items ?? [];
  return items.map(alertRuleFromWire);
}

export type RunnerMetricsSummary = {
  window_minutes: number;
  total_runs: number;
  error_runs: number;
  error_rate: number;
  success_rate: number;
};

export async function getRunnerMetrics(windowMinutes = 60): Promise<RunnerMetricsSummary> {
  const res = await monitor.GetRunnerMetrics({ windowMinutes });
  const r = obj(res);
  return {
    window_minutes: Number(r.window_minutes ?? r.windowMinutes ?? windowMinutes),
    total_runs: Number(r.total_runs ?? r.totalRuns ?? 0),
    error_runs: Number(r.error_runs ?? r.errorRuns ?? 0),
    error_rate: Number(r.error_rate ?? r.errorRate ?? 0),
    success_rate: Number(r.success_rate ?? r.successRate ?? 0)
  };
}
