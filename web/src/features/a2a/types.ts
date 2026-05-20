export type A2ACapability = {
  name: string;
  description: string;
  input_schema_json: string;
  output_schema_json: string;
};

export type A2AAgentCard = {
  agent_id: string;
  display_name: string;
  workspace: string;
  enabled: boolean;
  capabilities: A2ACapability[];
  updated_at: string;
  /** local | remote */
  source?: string;
  endpoint_url?: string;
  remote_url?: string;
};

export type A2ARuntimeConfig = {
  public_base_url: string;
  public_base_url_source: string;
};

export type A2AGatewayEntry = {
  card: A2AAgentCard;
  source: string;
  registry_id: string;
  endpoint_url: string;
  remote_url: string;
  healthy: boolean;
};

export type A2AInvokeInput = {
  callee_agent_id: string;
  capability: string;
  payload_json: string;
  caller_session_id?: string;
  timeout_seconds?: number;
  workspace?: string;
};

export type A2ARemoteAgent = {
  id: string;
  workspace: string;
  display_name: string;
  remote_url: string;
  agent_card_url: string;
  auth_type: string;
  enabled: boolean;
  discovered_card?: A2AAgentCard;
  created_at: string;
  updated_at: string;
};

export type RegisterRemoteAgentInput = {
  workspace?: string;
  remote_url: string;
  agent_card_url?: string;
  display_name?: string;
  auth_type?: string;
  auth_config_json?: string;
  enabled?: boolean;
};

export type DiscoverRemoteInput = {
  remote_url: string;
  auth_type?: string;
  auth_config_json?: string;
};

export type A2AInvokeResult = {
  invoke_id: string;
  /** success | error | timeout */
  status: string;
  result_json: string;
  error_message: string;
  duration_ms: number;
};

export type A2AAuditEntry = {
  id: string;
  invoke_id: string;
  caller_agent_id: string;
  callee_agent_id: string;
  capability: string;
  status: string;
  duration_ms: number;
  workspace: string;
  created_at: string;
};

export type UpdateAgentCardInput = {
  enabled: boolean;
  capabilities: A2ACapability[];
};

export type DiscoverParams = {
  workspace?: string;
  capability?: string;
};

export type ListAuditParams = {
  caller_agent_id?: string;
  callee_agent_id?: string;
  limit?: number;
  offset?: number;
};

export type ListAuditResult = {
  items: A2AAuditEntry[];
  total: number;
};
