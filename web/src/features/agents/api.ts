import { createAgentService } from "../../services";
import type {
  CreateAgentRequest as KratosCreateAgentRequest
} from "../../services/kratos/agent/v1/index";
import type {
  Agent,
  AgentListQuery,
  AgentListResult,
  AgentPromptFile,
  AgentRuntimeSettings
} from "./types";
import {
  normalizeAgentFromService,
  partialAgentToWire,
  promptFileToWire,
  runtimeSettingsToWire
} from "./wireNormalize";

export type {
  Agent,
  AgentListQuery,
  AgentListResult,
  AgentPromptFile,
  AgentRuntimeSettings
} from "./types";

export async function listAgentsPaged(query: AgentListQuery = {}): Promise<AgentListResult> {
  const svc = createAgentService();
  const res = await svc.ListAgents({
    keyword: query.keyword,
    status: query.status,
    provider: query.provider,
    categoryId: query.category_id,
    limit: query.limit,
    offset: query.offset
  });
  return {
    items: (res.items ?? []).map((row) => normalizeAgentFromService(row)),
    total: Number(res.total ?? res.items?.length ?? 0),
    limit: Number(res.limit ?? query.limit ?? 24),
    offset: Number(res.offset ?? query.offset ?? 0)
  };
}

export async function listAgents(query: AgentListQuery = {}): Promise<Agent[]> {
  const result = await listAgentsPaged(query);
  return result.items;
}

export async function createAgent(payload: {
  agent_key: string;
  display_name: string;
  provider: string;
  model: string;
  icon?: string;
  agent_description?: string;
  category_position_id?: string;
  system_prompt_mode?: string;
  context_window?: number;
  budget_monthly_cents?: number;
  config_json?: string;
  settings?: AgentRuntimeSettings;
  files?: AgentPromptFile[];
}): Promise<Agent> {
  const svc = createAgentService();
  const req: KratosCreateAgentRequest = {
    agentKey: payload.agent_key,
    displayName: payload.display_name,
    provider: payload.provider,
    model: payload.model,
    icon: payload.icon,
    agentDescription: payload.agent_description,
    categoryPositionId: payload.category_position_id,
    systemPromptMode: payload.system_prompt_mode,
    contextWindow: payload.context_window,
    budgetMonthlyCents: payload.budget_monthly_cents,
    configJson: payload.config_json,
    settings: payload.settings ? runtimeSettingsToWire(payload.settings) : undefined,
    files: payload.files?.map(promptFileToWire)
  };
  const data = await svc.CreateAgent(req);
  return normalizeAgentFromService(data);
}

export async function getAgent(id: string): Promise<Agent> {
  const svc = createAgentService();
  const data = await svc.GetAgent({ id });
  return normalizeAgentFromService(data);
}

export async function updateAgent(id: string, payload: Partial<Agent>): Promise<Agent> {
  const svc = createAgentService();
  const data = await svc.UpdateAgent({
    id,
    agent: partialAgentToWire(payload)
  });
  return normalizeAgentFromService(data);
}

export async function getAgentPromptPreview(id: string, mode?: string): Promise<string> {
  const svc = createAgentService();
  const res = await svc.GetAgentPromptPreview({ id, mode });
  return res.preview ?? "";
}

export async function deleteAgent(id: string): Promise<void> {
  const svc = createAgentService();
  await svc.DeleteAgent({ id });
}

export {
  listPlatformResources as listAgentDependencies,
  validateModel,
  type PlatformResource
} from "../platform/api";
