export type ToolPermissions = {
  can_manage: boolean;
};

export type Tool = {
  id: string;
  key: string;
  display_name: string;
  description: string;
  category: string;
  source: 'builtin' | 'mcp' | 'system' | 'external' | string;
  risk_level: 'low' | 'medium' | 'high' | 'critical' | string;
  enabled: boolean;
  readonly: boolean;
  requires_confirmation: boolean;
  supports_streaming: boolean;
  supports_concurrency: boolean;
  parameters_schema_json: string;
  result_schema_json: string;
  config_schema_json: string;
  config_json: string;
  default_config_json: string;
  metadata_json: string;
  runtime_status?: 'available' | 'registered_only' | 'disabled' | string;
  runtime_kind?: 'function' | 'streaming' | 'approval' | string;
  invoke_count: number;
  invoke_count_24h: number;
  success_count: number;
  failure_count: number;
  blocked_count: number;
  agent_override_count: number;
  avg_duration_ms: number | null;
  p95_duration_ms: number;
  last_invoked_at?: string;
  last_status?: string;
  created_at: string;
  updated_at: string;
  permissions: ToolPermissions;
};

export type ToolUpsertInput = {
  key: string;
  display_name: string;
  description: string;
  category: string;
  source: string;
  risk_level: string;
  enabled: boolean;
  readonly: boolean;
  requires_confirmation: boolean;
  supports_streaming: boolean;
  supports_concurrency: boolean;
  parameters_schema_json: string;
  result_schema_json: string;
  config_schema_json: string;
  config_json: string;
  default_config_json: string;
  metadata_json: string;
};

export type ToolSummary = {
  total_tools: number;
  enabled_tools: number;
  high_risk_enabled: number;
  calls_24h: number;
  failure_rate_24h: number;
};

export type ToolListQuery = {
  search?: string;
  category?: string;
  source?: string;
  risk_level?: string;
  enabled?: boolean | null;
  sort?: string;
  /** abnormal=true：仅返回最近一次调用以 error/blocked 收尾的工具（「仅看异常」） */
  abnormal?: boolean;
  page?: number;
  page_size?: number;
};

export type ToolListResponse = {
  items: Tool[];
  page: number;
  page_size: number;
  total: number;
  summary: ToolSummary;
};

export type ToolInvocation = {
  id: string;
  request_id: string;
  invocation_id: string;
  tool_id: string;
  tool_key: string;
  tool_display_name: string;
  agent_id: string;
  agent_key: string;
  agent_display_name: string;
  session_id: string;
  message_id: string;
  user_id: string;
  source: string;
  status: 'success' | 'error' | 'blocked' | 'cancelled' | string;
  started_at: string;
  ended_at: string;
  duration_ms: number;
  input_preview: string;
  input_hash: string;
  output_preview: string;
  output_hash: string;
  error_code: string;
  error_message: string;
  redaction_applied: boolean;
  metadata_json: string;
  created_at: string;
  streaming?: boolean;
  chunk_count?: number;
};

/** 调用记录参数详情（GET /v1/tools/runs/{invocation_id}/params，脱敏后参数）。 */
export type ToolInvocationParamDetail = {
  id: string;
  invocation_id: string;
  tool_key: string;
  params_json: string;
  redaction_applied: boolean;
  created_at: string;
};

export type ToolInvocationAudit = {
  id: string;
  invocation_id: string;
  tool_key: string;
  agent_id: string;
  user_id: string;
  session_id: string;
  action: string;
  result_summary: string;
  status: string;
  source: string;
  created_at: string;
};

export type ToolAuditQuery = {
  tool_key?: string;
  agent_id?: string;
  user_id?: string;
  session_id?: string;
  status?: string;
  from?: string;
  to?: string;
  page?: number;
  page_size?: number;
};

export type ToolRunQuery = {
  tool_key?: string;
  agent_id?: string;
  session_id?: string;
  status?: string;
  from?: string;
  to?: string;
  has_error?: boolean;
  page?: number;
  page_size?: number;
};

export type PaginatedResponse<T> = {
  items: T[];
  page: number;
  page_size: number;
  total: number;
};

export type ToolAgentOverride = {
  id: string;
  tool_id: string;
  tool_key: string;
  agent_id: string;
  enabled: boolean;
  mode: 'inherit' | 'allow' | 'deny' | string;
  config_override_json: string;
  requires_confirmation: boolean;
  created_at: string;
  updated_at: string;
};

export type ToolAgentOverrideInput = {
  tool_id: string;
  agent_id: string;
  enabled?: boolean;
  mode?: string;
  config_override_json?: string;
  requires_confirmation?: boolean;
};

export type AgentEffectiveTool = {
  tool_key: string;
  display_name: string;
  category: string;
  source: string;
  enabled: boolean;
  effective_state: 'allowed' | 'denied' | string;
  reason: string;
};

export type AgentEffectiveTools = {
  tools_enabled: boolean;
  profile: string;
  allow: string[];
  deny: string[];
  items: AgentEffectiveTool[];
};

/** GET /v1/tools/{tool_id}/agent-bindings 单行：某工具在一个 Agent 上的生效状态（后端聚合计算）。 */
export type ToolAgentBinding = {
  agent_id: string;
  agent_key: string;
  agent_name: string;
  agent_status: string;
  tools_enabled: boolean;
  profile: string;
  effective_state: 'allowed' | 'denied' | string;
  reason: string;
  override_mode: string;
};

export type ToolTestResult = {
  status: string;
  result_preview: string;
  error_message: string;
  duration_ms: number;
};
