/**
 * A2A 协议：**`createA2AService()`** → **`/v1/a2a/...`**。
 *
 * 注意：A2A Invoke 后端尚为 stub（EP-A2A-01），远端鉴权未完成（EP-A2A-02）。
 * 前端展示 AgentCard / Discover 是可用的，发起 Invoke 需注意当前为演示状态。
 */
import { createA2AService } from "../../services";
import { asRecord, pickBool, pickI32, pickStr } from "../../shared/wireJson";
import type {
  A2AAgentCard,
  A2AAuditEntry,
  A2ACapability,
  A2AInvokeInput,
  A2AInvokeResult,
  DiscoverParams,
  ListAuditParams,
  ListAuditResult,
  UpdateAgentCardInput
} from "./types";

const svc = createA2AService();

function mapCapability(raw: unknown): A2ACapability {
  const r = asRecord(raw);
  return {
    name: pickStr(r, "name", "name"),
    description: pickStr(r, "description", "description"),
    input_schema_json: pickStr(r, "input_schema_json", "inputSchemaJson"),
    output_schema_json: pickStr(r, "output_schema_json", "outputSchemaJson")
  };
}

function mapAgentCard(raw: unknown): A2AAgentCard {
  const r = asRecord(raw);
  const capsRaw = r.capabilities ?? r.Capabilities;
  const capabilities = Array.isArray(capsRaw) ? capsRaw.map(mapCapability) : [];
  return {
    agent_id: pickStr(r, "agent_id", "agentId"),
    display_name: pickStr(r, "display_name", "displayName"),
    workspace: pickStr(r, "workspace", "workspace"),
    enabled: pickBool(r, "enabled", "enabled"),
    capabilities,
    updated_at: pickStr(r, "updated_at", "updatedAt")
  };
}

function mapAuditEntry(raw: unknown): A2AAuditEntry {
  const r = asRecord(raw);
  return {
    id: pickStr(r, "id", "id"),
    invoke_id: pickStr(r, "invoke_id", "invokeId"),
    caller_agent_id: pickStr(r, "caller_agent_id", "callerAgentId"),
    callee_agent_id: pickStr(r, "callee_agent_id", "calleeAgentId"),
    capability: pickStr(r, "capability", "capability"),
    status: pickStr(r, "status", "status"),
    duration_ms: pickI32(r, "duration_ms", "durationMs"),
    workspace: pickStr(r, "workspace", "workspace"),
    created_at: pickStr(r, "created_at", "createdAt")
  };
}

// ---------- Discover ----------

export async function discoverAgents(params: DiscoverParams = {}): Promise<A2AAgentCard[]> {
  const res = asRecord(
    await svc.Discover({ workspace: params.workspace ?? "", capability: params.capability ?? "" })
  );
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
    outputSchemaJson: c.output_schema_json
  }));
  const raw = await svc.UpdateAgentCard({ agentId, enabled: input.enabled, capabilities });
  return mapAgentCard(raw);
}

// ---------- Invoke ----------

/** Invoke 当前为演示状态（EP-A2A-01），实际派发逻辑待后端完善。 */
export async function invokeA2A(input: A2AInvokeInput): Promise<A2AInvokeResult> {
  const res = asRecord(
    await svc.Invoke({
      calleeAgentId: input.callee_agent_id,
      capability: input.capability,
      payloadJson: input.payload_json,
      callerSessionId: input.caller_session_id ?? "",
      timeoutSeconds: input.timeout_seconds ?? 0
    })
  );
  return {
    invoke_id: pickStr(res, "invoke_id", "invokeId"),
    status: pickStr(res, "status", "status"),
    result_json: pickStr(res, "result_json", "resultJson"),
    error_message: pickStr(res, "error_message", "errorMessage"),
    duration_ms: pickI32(res, "duration_ms", "durationMs")
  };
}

// ---------- Audit ----------

export async function listA2AAudit(params: ListAuditParams = {}): Promise<ListAuditResult> {
  const res = asRecord(
    await svc.ListAudit({
      callerAgentId: params.caller_agent_id ?? "",
      calleeAgentId: params.callee_agent_id ?? "",
      limit: params.limit ?? 0,
      offset: params.offset ?? 0
    })
  );
  const itemsRaw = res.items ?? res.Items;
  const items = Array.isArray(itemsRaw) ? itemsRaw.map(mapAuditEntry) : [];
  return { items, total: pickI32(res, "total", "total") || items.length };
}
