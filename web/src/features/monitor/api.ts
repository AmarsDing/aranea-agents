import { createMonitorService } from "../../services/index";
import type {
  AuditLog,
  ModelUsageQuery,
  MonitorLogLine,
  MonitorLogSnapshot,
  MonitorTraceEvent,
  PlatformResource,
  TeamRunEvent
} from "./types";
import { listModelUsageEvents } from "../usage/api";
import { useEnvelopeStream } from "../chat/useEnvelopeStream";
import type { Envelope } from "../chat/envelope";

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
    created_at: String(r.created_at ?? r.createdAt ?? "")
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

export async function listMonitorAudit(limit = 200): Promise<AuditLog[]> {
  const res = await monitor.ListAuditLogs({ limit });
  const items = res.items ?? [];
  return items.map((item: unknown) => auditFromWire(item));
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

export function subscribeMonitorLogsWs(
  sessionId: string,
  onLine: (line: MonitorLogLine) => void,
  onError?: (error: string) => void,
  onConnected?: () => void
) {
  const stream = useEnvelopeStream({ sessionId, channels: ["monitor", "system"], autoConnect: false, logEnabled: false });

  stream.onType("log", (env: Envelope) => {
    const level = (env.metadata?.level as MonitorLogLine["level"]) ?? "INFO";
    onLine({
      id: env.id,
      time: env.timestamp,
      level,
      message: env.content?.text ?? "",
      source: env.author ?? "monitor",
      created_at: env.timestamp
    });
  });

  stream.onType("error", (env: Envelope) => {
    onError?.(env.error?.message ?? "monitor ws error");
  });

  stream.onType("connected", () => {
    onConnected?.();
  });

  stream.connect();

  return {
    close: () => stream.disconnect(),
    connected: stream.connected,
    enableLog: (enabled: boolean) => stream.enableLog(enabled),
  };
}

export function subscribeMonitorRuntimeEventsWs(
  sessionId: string,
  onEvent: (event: TeamRunEvent) => void,
  onError?: (error: string) => void
) {
  const stream = useEnvelopeStream({ sessionId, channels: ["monitor", "system"], autoConnect: false, logEnabled: false });

  stream.onType("log", (env: Envelope) => {
    const eventType = (env.metadata?.event_type as string) ?? env.type;
    onEvent({
      type: eventType,
      team_id: env.team_id ?? "",
      run_id: "",
      session_id: env.session_id,
      payload: env.metadata ?? {}
    });
  });

  stream.onType("intent_pass", (env: Envelope) => {
    onEvent({
      type: "intent_pass",
      team_id: env.team_id ?? "",
      run_id: "",
      session_id: env.session_id,
      payload: env.metadata ?? {}
    });
  });

  stream.onType("error", (env: Envelope) => {
    onError?.(env.error?.message ?? "monitor ws error");
  });

  stream.connect();

  return {
    close: () => stream.disconnect(),
    connected: stream.connected
  };
}

export async function listMonitorTraceEvents(query: ModelUsageQuery = {}): Promise<MonitorTraceEvent[]> {
  const rows = await listModelUsageEvents(query);
  return rows as MonitorTraceEvent[];
}
