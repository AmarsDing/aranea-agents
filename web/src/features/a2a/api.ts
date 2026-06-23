/**
 * A2A 协议：**`createA2AService()`** → **`/v1/a2a/...`**。
 * Invoke 经 AgentTurnRunner 分发；AgentCard / Discover / Audit 可用。
 */
import { createA2AService } from '../../services';
import { asRecord, pickBool, pickI32, pickStr } from '../../shared/wireJson';
import type {
  A2AAgentCard,
  A2AAuditEntry,
  A2ACapability,
  A2AInvokeInput,
  A2AInvokeResult,
  DiscoverParams,
  ListAuditParams,
  ListAuditResult,
  DiscoverRemoteInput,
  RegisterRemoteAgentInput,
  A2ARemoteAgent,
  UpdateAgentCardInput,
  A2AGatewayEntry,
  A2ARuntimeConfig,
} from './types';

const svc = createA2AService();

function mapCapability(raw: unknown): A2ACapability {
  const r = asRecord(raw);
  return {
    name: pickStr(r, 'name', 'name'),
    description: pickStr(r, 'description', 'description'),
    input_schema_json: pickStr(r, 'input_schema_json', 'inputSchemaJson'),
    output_schema_json: pickStr(r, 'output_schema_json', 'outputSchemaJson'),
  };
}

function mapAgentCard(raw: unknown): A2AAgentCard {
  const r = asRecord(raw);
  const capsRaw = r.capabilities ?? r.Capabilities;
  const capabilities = Array.isArray(capsRaw) ? capsRaw.map(mapCapability) : [];
  return {
    agent_id: pickStr(r, 'agent_id', 'agentId'),
    display_name: pickStr(r, 'display_name', 'displayName'),
    workspace: pickStr(r, 'workspace', 'workspace'),
    enabled: pickBool(r, 'enabled', 'enabled'),
    capabilities,
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
    source: pickStr(r, 'source', 'source'),
    endpoint_url: pickStr(r, 'endpoint_url', 'endpointUrl'),
    remote_url: pickStr(r, 'remote_url', 'remoteUrl'),
  };
}

function mapAuditEntry(raw: unknown): A2AAuditEntry {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    invoke_id: pickStr(r, 'invoke_id', 'invokeId'),
    caller_agent_id: pickStr(r, 'caller_agent_id', 'callerAgentId'),
    callee_agent_id: pickStr(r, 'callee_agent_id', 'calleeAgentId'),
    capability: pickStr(r, 'capability', 'capability'),
    status: pickStr(r, 'status', 'status'),
    duration_ms: pickI32(r, 'duration_ms', 'durationMs'),
    workspace: pickStr(r, 'workspace', 'workspace'),
    created_at: pickStr(r, 'created_at', 'createdAt'),
  };
}

// ---------- Discover ----------

export async function discoverAgents(params: DiscoverParams = {}): Promise<A2AAgentCard[]> {
  const res = asRecord(await svc.Discover({ workspace: params.workspace ?? '', capability: params.capability ?? '' }));
  const agentsRaw = res.agents ?? res.Agents;
  return Array.isArray(agentsRaw) ? agentsRaw.map(mapAgentCard) : [];
}

// ---------- AgentCard ----------

export async function getAgentCard(agentId: string): Promise<A2AAgentCard> {
  const raw = await svc.GetAgentCard({ agentId });
  return mapAgentCard(raw);
}

export async function updateAgentCard(agentId: string, input: UpdateAgentCardInput): Promise<A2AAgentCard> {
  const capabilities = input.capabilities.map((c) => ({
    name: c.name,
    description: c.description,
    inputSchemaJson: c.input_schema_json,
    outputSchemaJson: c.output_schema_json,
  }));
  const raw = await svc.UpdateAgentCard({ agentId, enabled: input.enabled, capabilities });
  return mapAgentCard(raw);
}

// ---------- Invoke ----------

/** Invoke dispatches via ChatService / NewInvoker (Admin 鉴权 + 工作区策略). */
export async function invokeA2A(input: A2AInvokeInput): Promise<A2AInvokeResult> {
  const res = asRecord(
    await svc.Invoke({
      calleeAgentId: input.callee_agent_id,
      capability: input.capability,
      payloadJson: input.payload_json,
      callerSessionId: input.caller_session_id ?? '',
      timeoutSeconds: input.timeout_seconds ?? 0,
      workspace: input.workspace ?? '',
    }),
  );
  return {
    invoke_id: pickStr(res, 'invoke_id', 'invokeId'),
    status: pickStr(res, 'status', 'status'),
    result_json: pickStr(res, 'result_json', 'resultJson'),
    error_message: pickStr(res, 'error_message', 'errorMessage'),
    duration_ms: pickI32(res, 'duration_ms', 'durationMs'),
  };
}

// ---------- Audit ----------

export async function listA2AAudit(params: ListAuditParams = {}): Promise<ListAuditResult> {
  const res = asRecord(
    await svc.ListAudit({
      callerAgentId: params.caller_agent_id ?? '',
      calleeAgentId: params.callee_agent_id ?? '',
      limit: params.limit ?? 0,
      offset: params.offset ?? 0,
    }),
  );
  const itemsRaw = res.items ?? res.Items;
  const items = Array.isArray(itemsRaw) ? itemsRaw.map(mapAuditEntry) : [];
  return { items, total: pickI32(res, 'total', 'total') };
}

function mapRemoteAgent(raw: unknown): A2ARemoteAgent {
  const r = asRecord(raw);
  const cardRaw = r.discoveredCard ?? r.discovered_card;
  return {
    id: pickStr(r, 'id', 'id'),
    workspace: pickStr(r, 'workspace', 'workspace'),
    display_name: pickStr(r, 'display_name', 'displayName'),
    remote_url: pickStr(r, 'remote_url', 'remoteUrl'),
    agent_card_url: pickStr(r, 'agent_card_url', 'agentCardUrl'),
    auth_type: pickStr(r, 'auth_type', 'authType'),
    enabled: pickBool(r, 'enabled', 'enabled'),
    discovered_card: cardRaw ? mapAgentCard(cardRaw) : undefined,
    created_at: pickStr(r, 'created_at', 'createdAt'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
    healthy: pickBool(r, 'healthy', 'healthy') || false,
    last_health_at: pickStr(r, 'last_health_at', 'lastHealthAt'),
  };
}

export async function listRemoteAgents(workspace = ''): Promise<A2ARemoteAgent[]> {
  const res = asRecord(await svc.ListRemoteAgents({ workspace }));
  const itemsRaw = res.items ?? res.Items;
  return Array.isArray(itemsRaw) ? itemsRaw.map(mapRemoteAgent) : [];
}

export async function registerRemoteAgent(input: RegisterRemoteAgentInput): Promise<A2ARemoteAgent> {
  const raw = await svc.RegisterRemoteAgent({
    workspace: input.workspace ?? '',
    remoteUrl: input.remote_url,
    agentCardUrl: input.agent_card_url ?? '',
    displayName: input.display_name ?? '',
    authType: input.auth_type ?? '',
    authConfigJson: input.auth_config_json ?? '',
    enabled: input.enabled ?? true,
  });
  return mapRemoteAgent(raw);
}

export async function deleteRemoteAgent(id: string): Promise<void> {
  await svc.DeleteRemoteAgent({ id });
}

export async function discoverRemoteAgent(input: DiscoverRemoteInput): Promise<A2AAgentCard> {
  const raw = await svc.DiscoverRemoteAgent({
    remoteUrl: input.remote_url,
    authType: input.auth_type ?? '',
    authConfigJson: input.auth_config_json ?? '',
  });
  return mapAgentCard(raw);
}

export async function getA2AConfig(): Promise<A2ARuntimeConfig> {
  const r = asRecord(await svc.GetA2AConfig({}));
  return {
    public_base_url: pickStr(r, 'public_base_url', 'publicBaseUrl'),
    public_base_url_source: pickStr(r, 'public_base_url_source', 'publicBaseUrlSource'),
  };
}

export async function gatewayDiscoverAgents(
  params: {
    workspace?: string;
    capability?: string;
    check_health?: boolean;
  } = {},
): Promise<A2AGatewayEntry[]> {
  const res = asRecord(
    await svc.GatewayDiscover({
      workspace: params.workspace ?? '',
      capability: params.capability ?? '',
      checkHealth: params.check_health ?? false,
    }),
  );
  const itemsRaw = res.items ?? res.Items;
  if (!Array.isArray(itemsRaw)) return [];
  return itemsRaw.map((raw) => {
    const r = asRecord(raw);
    const cardRaw = r.card ?? r.Card;
    return {
      card: mapAgentCard(cardRaw),
      source: pickStr(r, 'source', 'source'),
      registry_id: pickStr(r, 'registry_id', 'registryId'),
      endpoint_url: pickStr(r, 'endpoint_url', 'endpointUrl'),
      remote_url: pickStr(r, 'remote_url', 'remoteUrl'),
      healthy: pickBool(r, 'healthy', 'healthy'),
    };
  });
}
