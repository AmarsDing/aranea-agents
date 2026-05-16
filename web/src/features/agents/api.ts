import { createAgentService } from "../../services";
import type {
  CreateAgentRequest as KratosCreateAgentRequest
} from "../../services/kratos/agent/v1/index";
import type {
  Agent,
  AgentListQuery,
  AgentListResult,
  AgentPromptFile,
  AgentRuntimeSettings,
  EvolutionMetrics,
  EvolutionSuggestion
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
  AgentRuntimeSettings,
  EvolutionMetrics,
  EvolutionSuggestion
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

export async function getAgentEvolutionMetrics(
  agentId: string,
  timeRange: string = "30d"
): Promise<EvolutionMetrics> {
  const svc = createAgentService();
  const res = await svc.GetAgentEvolutionMetrics({ agentId, timeRange });
  return {
    agent_id: res.agentId ?? agentId,
    time_range: res.timeRange ?? timeRange,
    tool_success_rate: res.toolSuccessRate ?? 0,
    retrieval_quality: res.retrievalQuality ?? 0,
    total_episodes: res.totalEpisodes ?? 0,
    negative_feedback: res.negativeFeedback ?? 0,
    tool_success_series: (res.toolSuccessSeries ?? []).map((p) => ({
      date: p.date ?? "",
      value: p.value ?? 0
    })),
    retrieval_quality_series: (res.retrievalQualitySeries ?? []).map((p) => ({
      date: p.date ?? "",
      value: p.value ?? 0
    }))
  };
}

export async function getAgentEvolutionSuggestions(
  agentId: string,
  status?: string
): Promise<EvolutionSuggestion[]> {
  const svc = createAgentService();
  const res = await svc.GetAgentEvolutionSuggestions({ agentId, status });
  return (res.items ?? []).map((item) => ({
    id: item.id ?? "",
    agent_id: item.agentId ?? "",
    type: item.type ?? "",
    title: item.title ?? "",
    content: item.content ?? "",
    status: item.status ?? "",
    diff_preview: item.diffPreview ?? "",
    created_at: item.createdAt ?? "",
    applied_at: item.appliedAt ?? ""
  }));
}

export async function applyEvolutionSuggestion(
  agentId: string,
  suggestionId: string
): Promise<EvolutionSuggestion> {
  const svc = createAgentService();
  const res = await svc.ApplyEvolutionSuggestion({ agentId, suggestionId });
  return {
    id: res.id ?? "",
    agent_id: res.agentId ?? "",
    type: res.type ?? "",
    title: res.title ?? "",
    content: res.content ?? "",
    status: res.status ?? "",
    diff_preview: res.diffPreview ?? "",
    created_at: res.createdAt ?? "",
    applied_at: res.appliedAt ?? ""
  };
}

export async function rejectEvolutionSuggestion(
  agentId: string,
  suggestionId: string
): Promise<EvolutionSuggestion> {
  const svc = createAgentService();
  const res = await svc.RejectEvolutionSuggestion({ agentId, suggestionId });
  return {
    id: res.id ?? "",
    agent_id: res.agentId ?? "",
    type: res.type ?? "",
    title: res.title ?? "",
    content: res.content ?? "",
    status: res.status ?? "",
    diff_preview: res.diffPreview ?? "",
    created_at: res.createdAt ?? "",
    applied_at: res.appliedAt ?? ""
  };
}

export {
  listPlatformResources as listAgentDependencies,
  validateModel,
  type PlatformResource
} from "../platform/api";
