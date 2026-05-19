import type { PlatformResource } from "../platform/api";

export type McpTransport = "stdio" | "sse" | "streamable_http";
export type McpHealthStatus = "ok" | "error" | "unknown" | "degraded" | string;

export type McpKeyValue = {
  key: string;
  value: string;
};

export type McpAuthConfig = {
  type?: "api_key" | "bearer" | "oauth2" | "oauth2_client_credentials" | "oauth2_refresh" | "oauth2_static" | string;
  api_key?: string;
  header_name?: string;
  token_url?: string;
  client_id?: string;
  client_secret?: string;
  scope?: string;
  access_token?: string;
  refresh_token?: string;
};

export type McpServerConfig = {
  transport?: McpTransport;
  url?: string;
  command?: string;
  args?: string[];
  headers?: Record<string, string>;
  env?: Record<string, string>;
  auth?: McpAuthConfig;
  tool_prefix?: string;
  timeout_sec?: number;
  session_reconnect_max?: number;
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
  session_reconnect_max: number;
  auth_type: string;
  auth_api_key: string;
  auth_header_name: string;
  auth_token_url: string;
  auth_client_id: string;
  auth_client_secret: string;
  auth_scope: string;
  auth_access_token: string;
  auth_refresh_token: string;
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
