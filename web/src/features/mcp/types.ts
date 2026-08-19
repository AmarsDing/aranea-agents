import type { PlatformResource } from '../platform/types';

export type McpTransport = 'stdio' | 'sse' | 'streamable_http';
export type McpHealthStatus = 'ok' | 'error' | 'unknown' | 'degraded' | string;

/** 健康指示灯色调：healthTone 推导结果，对应 health-dot--* 样式类。 */
export type McpHealthTone = 'ok' | 'error' | 'degraded' | 'unknown';

export type McpKeyValue = {
  key: string;
  value: string;
};

export type McpAuthConfig = {
  type?: 'api_key' | 'bearer' | 'oauth2' | 'oauth2_client_credentials' | 'oauth2_refresh' | 'oauth2_static' | string;
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
  allow_adhoc_http?: boolean;
  probe_mode?: 'connectivity' | 'auth_aware' | string;
};

export type McpServerMetadata = {
  health_status?: McpHealthStatus;
  last_health_at?: string;
  last_error_message?: string;
  last_reconnect_at?: string;
  reconnect_count?: number;
  /** P2 工具发现（真实握手 initialize+tools/list 写入，与 health_* 正交）。 */
  tool_count?: number;
  tool_names?: string[];
  tools_discovered_at?: string;
  tools_error_message?: string;
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
  allow_adhoc_http: boolean;
  probe_mode: 'connectivity' | 'auth_aware';
};

export type McpServerRow = PlatformResource & {
  /** 内置/共享服务器（workspace_id=""）：全租户可读，但租户不可编辑/删除/启停。 */
  shared?: boolean;
};

export type McpServerTestResult = {
  ok: boolean;
  status: McpHealthStatus;
  message: string;
  details?: Record<string, unknown>;
};

export type McpServerValidateResult = McpServerTestResult;

export type McpUserCredential = {
  id: string;
  mcp_server_id: string;
  user_id: string;
  credential_key: string;
  status: string;
  configured: boolean;
  masked_preview: string;
  created_at: string;
  updated_at: string;
};

export type McpUserCredentialInput = {
  credential_key: string;
  secret: string;
  status?: string;
};
