import type { ModelTokenUsageEvent, ModelUsageQuery } from "../usage/types";
import type { TeamRunEvent } from "../teams/types";
import type { PlatformResource } from "../platform/api";

export type {
  ModelTokenUsageEvent,
  ModelUsageQuery,
  PlatformResource,
  TeamRunEvent
};

export type MonitorTraceEvent = ModelTokenUsageEvent & {
  date_key?: string;
  hour_key?: string;
  request_id?: string;
  team_id?: string;
  user_id?: string;
  usage_kind?: string;
  cached_input_tokens?: number;
  reasoning_tokens?: number;
  embedding_tokens?: number;
  input_cost_micro_usd?: number;
  output_cost_micro_usd?: number;
  error_code?: string;
  retry_count?: number;
  time_to_first_token_ms?: number;
  metadata_json?: string;
  created_at?: string;
};

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
};

export type MonitorEventsQuery = {
  limit?: number;
  offset?: number;
  event_type?: string;
  agent_id?: string;
  status?: string;
};

export type MonitorTracesQuery = {
  limit?: number;
  offset?: number;
  agent_id?: string;
  provider?: string;
  model?: string;
  status?: string;
};

export type PaginatedResult<T> = {
  items: T[];
  total: number;
};

export type MonitorLogLine = {
  id: string;
  time: string;
  level: "DEBUG" | "INFO" | "WARN" | "ERROR" | string;
  message: string;
  source: string;
  created_at: string;
};

export type MonitorLogSnapshot = {
  items: MonitorLogLine[];
  enabled: boolean;
  message: string;
};

export type LoadState = "idle" | "loading" | "success" | "empty" | "error";
export type StreamState = "connecting" | "live" | "paused" | "error";
