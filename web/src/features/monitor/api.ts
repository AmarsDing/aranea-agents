import { api } from "../../api/http";
import { getBackendBaseURL } from "../../config/runtime";
import type {
  AuditLog,
  ModelUsageQuery,
  MonitorLogLine,
  MonitorLogSnapshot,
  MonitorTraceEvent,
  PlatformResource,
  TeamRunEvent
} from "./types";

export async function listMonitorAudit(limit = 200): Promise<AuditLog[]> {
  const { data } = await api.get("/monitor/audit", { params: { limit } });
  return data.items ?? [];
}

export async function listMonitorEvents(): Promise<PlatformResource[]> {
  const { data } = await api.get("/monitor/events");
  return data.items ?? [];
}

export async function getMonitorEvent(id: string): Promise<PlatformResource> {
  const { data } = await api.get(`/monitor/events/${id}`);
  return data;
}

export async function getMonitorLogs(): Promise<MonitorLogSnapshot> {
  const { data } = await api.get("/monitor/logs");
  return {
    items: data.items ?? [],
    enabled: Boolean(data.enabled),
    message: data.message ?? ""
  };
}

export function subscribeMonitorLogs(onLine: (line: MonitorLogLine) => void, onError?: (error: Event) => void): EventSource {
  const source = new EventSource(`${getBackendBaseURL()}/monitor/logs/stream`);
  source.addEventListener("log", (event) => {
    onLine(JSON.parse((event as MessageEvent).data) as MonitorLogLine);
  });
  source.onerror = (event) => onError?.(event);
  return source;
}

export function subscribeMonitorRuntimeEvents(onEvent: (event: TeamRunEvent) => void, onError?: (error: Event) => void): EventSource {
  const source = new EventSource(`${getBackendBaseURL()}/team-run-events`);
  for (const eventName of ["run_started", "step_finished", "run_finished", "tool.call", "tool.result", "run.failed"]) {
    source.addEventListener(eventName, (event) => {
      onEvent(JSON.parse((event as MessageEvent).data) as TeamRunEvent);
    });
  }
  source.onerror = (event) => onError?.(event);
  return source;
}

export async function listMonitorTraceEvents(query: ModelUsageQuery = {}): Promise<MonitorTraceEvent[]> {
  const { data } = await api.get("/model-usage/events", { params: cleanQuery(query) });
  return data.items ?? [];
}

function cleanQuery(query: ModelUsageQuery) {
  return Object.fromEntries(
    Object.entries(query).filter(([, value]) => value !== "" && value !== undefined && value !== null)
  );
}
