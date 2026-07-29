/** 联邦 A2A 网络类型（FederationService，9 RPC）。字段与 proto 响应 snake_case 对齐。 */
import type { A2AAgentCard, A2ARemoteAgent } from './types';

export type FederationOrg = {
  id: string;
  name: string;
  domain: string;
  public_base_url: string;
  /** untrusted | neutral | trusted */
  trust_level: string;
  /** none | api_key | bearer | mtls */
  auth_type: string;
  /** auth_config_json 为 write-only（设计 F.8）；此处仅报告是否已配置。 */
  auth_config_set: boolean;
  /** active | suspended */
  status: string;
  joined_at: string;
  updated_at: string;
};

export type RegisterFederationOrgInput = {
  name: string;
  domain: string;
  public_base_url?: string;
  trust_level?: string;
  auth_type?: string;
  auth_config_json?: string;
};

export type FederationPolicy = {
  id: string;
  /** caller_org_id = "local" 即本组织（出站策略）。 */
  caller_org_id: string;
  callee_org_id: string;
  /** allow | deny（approval 本期按 deny 处理） */
  action: string;
  /** 每分钟调用上限；0 = 不限 */
  max_per_min: number;
  /** 每日 decision=allowed 调用上限；0 = 不限 */
  daily_quota: number;
  created_at: string;
  updated_at: string;
};

export type UpsertFederationPolicyInput = {
  caller_org_id: string;
  callee_org_id: string;
  action?: string;
  max_per_min?: number;
  daily_quota?: number;
};

export type FederationAgentEntry = {
  org: FederationOrg;
  remote_agent: A2ARemoteAgent;
  card: A2AAgentCard;
};

export type FederationAuditEntry = {
  id: string;
  /** outbound（inbound 预留下期） */
  direction: string;
  caller_org_id: string;
  callee_org_id: string;
  caller_agent_id: string;
  callee_agent_id: string;
  capability: string;
  /** allowed | denied_trust | denied_policy | denied_quota */
  decision: string;
  /** pending | success | error | timeout */
  status: string;
  latency_ms: number;
  error_message: string;
  created_at: string;
};

export type FederatedInvokeInput = {
  org_id: string;
  agent_id: string;
  capability: string;
  payload_json: string;
  timeout_seconds?: number;
  workspace?: string;
  caller_agent_id?: string;
};

export type FederatedInvokeResult = {
  audit_id: string;
  status: string;
  result_json: string;
  error_message: string;
  latency_ms: number;
};

export type FederationAuditFilter = {
  caller_org_id?: string;
  callee_org_id?: string;
  decision?: string;
  status?: string;
  limit?: number;
  offset?: number;
};

export type FederationAuditResult = {
  items: FederationAuditEntry[];
  total: number;
};
