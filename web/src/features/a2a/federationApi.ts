/**
 * 联邦 A2A 网络：**`createFederationService()`** → **`/v1/a2a/federation/...`**。
 * 组织注册 / 信任等级 / 调用策略 / 联邦目录 / 跨组织调用 / 审计查询。
 */
import { createFederationService } from '../../services';
import { asRecord, pickBool, pickI32, pickStr } from '../../shared/wireJson';
import { mapAgentCard, mapRemoteAgent } from './api';
import type {
  FederatedInvokeInput,
  FederatedInvokeResult,
  FederationAgentEntry,
  FederationAuditEntry,
  FederationAuditFilter,
  FederationAuditResult,
  FederationOrg,
  FederationPolicy,
  RegisterFederationOrgInput,
  UpsertFederationPolicyInput,
} from './federationTypes';

const svc = createFederationService();

function mapFederationOrg(raw: unknown): FederationOrg {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    name: pickStr(r, 'name', 'name'),
    domain: pickStr(r, 'domain', 'domain'),
    public_base_url: pickStr(r, 'public_base_url', 'publicBaseUrl'),
    trust_level: pickStr(r, 'trust_level', 'trustLevel'),
    auth_type: pickStr(r, 'auth_type', 'authType'),
    auth_config_set: pickBool(r, 'auth_config_set', 'authConfigSet'),
    status: pickStr(r, 'status', 'status'),
    joined_at: pickStr(r, 'joined_at', 'joinedAt'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
  };
}

function mapFederationPolicy(raw: unknown): FederationPolicy {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    caller_org_id: pickStr(r, 'caller_org_id', 'callerOrgId'),
    callee_org_id: pickStr(r, 'callee_org_id', 'calleeOrgId'),
    action: pickStr(r, 'action', 'action'),
    max_per_min: pickI32(r, 'max_per_min', 'maxPerMin'),
    daily_quota: pickI32(r, 'daily_quota', 'dailyQuota'),
    created_at: pickStr(r, 'created_at', 'createdAt'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
  };
}

function mapFederationAgentEntry(raw: unknown): FederationAgentEntry {
  const r = asRecord(raw);
  return {
    org: mapFederationOrg(r.org ?? r.Org),
    remote_agent: mapRemoteAgent(r.remoteAgent ?? r.remote_agent),
    card: mapAgentCard(r.card ?? r.Card),
  };
}

function mapFederationAuditEntry(raw: unknown): FederationAuditEntry {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    direction: pickStr(r, 'direction', 'direction'),
    caller_org_id: pickStr(r, 'caller_org_id', 'callerOrgId'),
    callee_org_id: pickStr(r, 'callee_org_id', 'calleeOrgId'),
    caller_agent_id: pickStr(r, 'caller_agent_id', 'callerAgentId'),
    callee_agent_id: pickStr(r, 'callee_agent_id', 'calleeAgentId'),
    capability: pickStr(r, 'capability', 'capability'),
    decision: pickStr(r, 'decision', 'decision'),
    status: pickStr(r, 'status', 'status'),
    latency_ms: pickI32(r, 'latency_ms', 'latencyMs'),
    error_message: pickStr(r, 'error_message', 'errorMessage'),
    created_at: pickStr(r, 'created_at', 'createdAt'),
  };
}

// ---------- Orgs ----------

export async function registerFederationOrg(input: RegisterFederationOrgInput): Promise<FederationOrg> {
  const raw = await svc.RegisterFederationOrg({
    name: input.name,
    domain: input.domain,
    publicBaseUrl: input.public_base_url ?? '',
    trustLevel: input.trust_level ?? '',
    authType: input.auth_type ?? '',
    authConfigJson: input.auth_config_json ?? '',
  });
  return mapFederationOrg(raw);
}

export async function listFederationOrgs(): Promise<FederationOrg[]> {
  const res = asRecord(await svc.ListFederationOrgs({}));
  const itemsRaw = res.items ?? res.Items;
  return Array.isArray(itemsRaw) ? itemsRaw.map(mapFederationOrg) : [];
}

export async function deleteFederationOrg(id: string): Promise<void> {
  await svc.DeleteFederationOrg({ id });
}

export async function setFederationTrustLevel(id: string, trustLevel: string): Promise<FederationOrg> {
  const raw = await svc.SetFederationTrustLevel({ id, trustLevel });
  return mapFederationOrg(raw);
}

/** 手动拉取该组织远程 Agent Card 到目录缓存（FED-F7）；单个失败跳过，返回成功数。 */
export async function syncFederationOrgCards(id: string): Promise<number> {
  const res = asRecord(await svc.SyncFederationOrgCards({ id }));
  return pickI32(res, 'synced', 'synced');
}

// ---------- Policies ----------

export async function upsertFederationPolicy(input: UpsertFederationPolicyInput): Promise<FederationPolicy> {
  const raw = await svc.UpsertFederationPolicy({
    callerOrgId: input.caller_org_id,
    calleeOrgId: input.callee_org_id,
    action: input.action ?? '',
    maxPerMin: input.max_per_min ?? 0,
    dailyQuota: input.daily_quota ?? 0,
  });
  return mapFederationPolicy(raw);
}

// ---------- Directory ----------

export async function discoverFederationAgents(
  params: { capability?: string; org_id?: string } = {},
): Promise<FederationAgentEntry[]> {
  const res = asRecord(
    await svc.DiscoverFederationAgents({ capability: params.capability ?? '', orgId: params.org_id ?? '' }),
  );
  const itemsRaw = res.items ?? res.Items;
  return Array.isArray(itemsRaw) ? itemsRaw.map(mapFederationAgentEntry) : [];
}

// ---------- Invoke ----------

export async function invokeFederatedAgent(input: FederatedInvokeInput): Promise<FederatedInvokeResult> {
  const res = asRecord(
    await svc.InvokeFederatedAgent({
      orgId: input.org_id,
      agentId: input.agent_id,
      capability: input.capability,
      payloadJson: input.payload_json,
      timeoutSeconds: input.timeout_seconds ?? 0,
      workspace: input.workspace ?? '',
      callerAgentId: input.caller_agent_id ?? '',
    }),
  );
  return {
    audit_id: pickStr(res, 'audit_id', 'auditId'),
    status: pickStr(res, 'status', 'status'),
    result_json: pickStr(res, 'result_json', 'resultJson'),
    error_message: pickStr(res, 'error_message', 'errorMessage'),
    latency_ms: pickI32(res, 'latency_ms', 'latencyMs'),
  };
}

// ---------- Audit ----------

export async function queryFederationAuditLogs(params: FederationAuditFilter = {}): Promise<FederationAuditResult> {
  const res = asRecord(
    await svc.QueryFederationAuditLogs({
      callerOrgId: params.caller_org_id ?? '',
      calleeOrgId: params.callee_org_id ?? '',
      decision: params.decision ?? '',
      status: params.status ?? '',
      limit: params.limit ?? 0,
      offset: params.offset ?? 0,
    }),
  );
  const itemsRaw = res.items ?? res.Items;
  const items = Array.isArray(itemsRaw) ? itemsRaw.map(mapFederationAuditEntry) : [];
  return { items, total: pickI32(res, 'total', 'total') };
}
