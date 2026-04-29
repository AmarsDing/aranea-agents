import type { PlatformResource } from "../platform/api";

export type McpTransport = "stdio" | "sse" | "streamable_http";
export type McpHealthStatus = "ok" | "error" | "unknown" | "degraded" | string;

export type McpKeyValue = {
  key: string;
  value: string;
};

export type McpServerConfig = {
  transport?: McpTransport;
  url?: string;
  command?: string;
  args?: string[];
  headers?: Record<string, string>;
  env?: Record<string, string>;
  tool_prefix?: string;
  timeout_sec?: number;
  require_user_credentials?: boolean;
};

export type McpServerMetadata = {
  health_status?: McpHealthStatus;
  last_health_at?: string;
  last_error_message?: string;
  [key: string]: unknown;
};

export type McpServerFormValue = {
  name: string;
  display_name: string;
  description: string;
  transport: McpTransport;
  url: string;
  command: string;
  argsText: string;
  headers: McpKeyValue[];
  env: McpKeyValue[];
  tool_prefix: string;
  timeout_sec: number;
  enabled: boolean;
  require_user_credentials: boolean;
};

export type McpServerRow = PlatformResource;

export type McpServerTestResult = {
  ok: boolean;
  status: McpHealthStatus;
  message: string;
  details?: Record<string, unknown>;
};
