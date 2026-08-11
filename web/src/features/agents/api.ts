import { createAgentService } from '../../services';
import type { CreateAgentRequest as KratosCreateAgentRequest } from '../../services/kratos/agent/v1/index';
import type {
  Agent,
  AgentListQuery,
  AgentListResult,
  AgentPromptFile,
  AgentRuntimeSettings,
  AgentCreatorOption,
  AgentTemplatePreset,
  EvolutionMetrics,
  EvolutionSuggestion,
  AgentPromptPreview,
} from './types';
import {
  normalizeAgentFromService,
  normalizePromptFileFromWire,
  partialAgentToWire,
  promptFileToWire,
  runtimeSettingsToWire,
  a2aProxyToWire,
} from './wireNormalize';
import type { A2AProxyConfig, AgentKind } from './types';
import { asRecord, pickI32, pickStr } from '../../shared/wireJson';
import { tokenEstimateFor } from './agentUtils';

export type {
  Agent,
  AgentListQuery,
  AgentListResult,
  AgentPromptFile,
  AgentRuntimeSettings,
  AgentCreatorOption,
  AgentTemplatePreset,
  EvolutionMetrics,
  EvolutionSuggestion,
  AgentPromptPreview,
  AgentPromptSection,
} from './types';

export async function listAgentsPaged(query: AgentListQuery = {}): Promise<AgentListResult> {
  const svc = createAgentService();
  const res = await svc.ListAgents({
    keyword: query.keyword,
    status: query.status,
    provider: query.provider,
    orgNodeId: query.org_node_id,
    createdBy: query.created_by,
    limit: query.limit,
    offset: query.offset,
  });
  return {
    items: (res.items ?? []).map((row) => normalizeAgentFromService(row)),
    total: Number(res.total ?? res.items?.length ?? 0),
    limit: Number(res.limit ?? query.limit ?? 24),
    offset: Number(res.offset ?? query.offset ?? 0),
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
  agent_kind?: AgentKind;
  a2a_proxy_config?: A2AProxyConfig;
  icon?: string;
  agent_description?: string;
  position_key?: string;
  agent_variant?: string;
  variant_description?: string;
  taxonomy_position_id?: string;
  system_prompt_mode?: string;
  context_window?: number;
  budget_monthly_cents?: number;
  config_json?: string;
  settings?: AgentRuntimeSettings;
  files?: AgentPromptFile[];
}): Promise<Agent> {
  const req: KratosCreateAgentRequest = {
    agentKey: payload.agent_key,
    displayName: payload.display_name,
    provider: payload.provider,
    model: payload.model,
    agentKind: payload.agent_kind,
    a2aProxyConfig: payload.a2a_proxy_config ? a2aProxyToWire(payload.a2a_proxy_config) : undefined,
    icon: payload.icon,
    agentDescription: payload.agent_description,
    positionKey: payload.position_key,
    agentVariant: payload.agent_variant,
    variantDescription: payload.variant_description,
    positionId: payload.taxonomy_position_id,
    systemPromptMode: payload.system_prompt_mode,
    contextWindow: payload.context_window,
    budgetMonthlyCents: payload.budget_monthly_cents,
    configJson: payload.config_json,
    settings: payload.settings ? runtimeSettingsToWire(payload.settings) : undefined,
    files: payload.files?.map(promptFileToWire),
  };
  const svc = createAgentService();
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
    agent: partialAgentToWire(payload),
  });
  return normalizeAgentFromService(data);
}

export async function listAgentTemplates(): Promise<AgentTemplatePreset[]> {
  const svc = createAgentService();
  const res = await svc.ListAgentTemplates({});
  return (res.items ?? []).map((row) => ({
    key: row.key ?? '',
    label: row.label ?? '',
    icon: row.icon ?? '',
    description: row.description ?? '',
    display_name: row.displayName ?? '',
    provider: row.provider ?? '',
    model: row.model ?? '',
  }));
}

export async function listAgentCreators(): Promise<AgentCreatorOption[]> {
  const svc = createAgentService();
  const res = await svc.ListAgentCreators({});
  return (res.items ?? []).map((row) => ({
    user_id: row.userId ?? '',
    label: row.label ?? row.userId ?? '',
  }));
}

export async function duplicateAgent(id: string): Promise<Agent> {
  const svc = createAgentService();
  const res = await svc.DuplicateAgent({ id });
  return normalizeAgentFromService(res);
}

export async function estimateAgentTokens(agentId: string): Promise<{
  total_tokens: number;
  file_estimates: Array<{ file_id: string; file_name: string; estimated_tokens: number }>;
}> {
  const svc = createAgentService();
  const res = await svc.EstimateTokens({ agentId });
  return {
    total_tokens: Number(res.totalTokens ?? 0),
    file_estimates: (res.fileEstimates ?? []).map((row) => ({
      file_id: row.fileId ?? '',
      file_name: row.fileName ?? '',
      estimated_tokens: Number(row.estimatedTokens ?? 0),
    })),
  };
}

export async function getAgentPromptPreview(id: string, mode?: string): Promise<AgentPromptPreview> {
  const svc = createAgentService();
  const res = await svc.GetAgentPromptPreview({ id, mode });
  const r = asRecord(res);
  const sectionsRaw = r.sections;
  const sections = Array.isArray(sectionsRaw)
    ? sectionsRaw.map((row) => {
        const s = asRecord(row);
        return {
          key: pickStr(s, 'key', 'key'),
          label: pickStr(s, 'label', 'label'),
          est_tokens: pickI32(s, 'est_tokens', 'estTokens'),
          source: pickStr(s, 'source', 'source'),
        };
      })
    : [];
  const instruction = pickStr(r, 'instruction', 'instruction');
  const summary = pickStr(r, 'preview', 'preview');
  const textForStatic = instruction || summary;
  let staticTotal = pickI32(r, 'static_total_tokens', 'staticTotalTokens');
  let runtimeOverlay = pickI32(r, 'runtime_overlay_est_tokens', 'runtimeOverlayEstTokens');
  if (staticTotal <= 0 && textForStatic) {
    staticTotal = tokenEstimateFor(textForStatic);
  }
  if (runtimeOverlay <= 0) {
    runtimeOverlay = sections
      .filter((row) => row.source === 'runtime' && row.est_tokens > 0)
      .reduce((sum, row) => sum + row.est_tokens, 0);
  }
  return {
    summary,
    instruction,
    sections,
    static_total_tokens: staticTotal,
    runtime_overlay_est_tokens: runtimeOverlay,
    runtime_note: pickStr(r, 'runtime_note', 'runtimeNote'),
  };
}

export async function deleteAgent(id: string): Promise<void> {
  const svc = createAgentService();
  await svc.DeleteAgent({ id });
}

export async function toggleAgentFavorite(id: string): Promise<Agent> {
  const svc = createAgentService();
  const res = await svc.ToggleFavorite({ id });
  return normalizeAgentFromService(res);
}

export async function getAgentEvolutionMetrics(agentId: string, timeRange: string = '30d'): Promise<EvolutionMetrics> {
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
      date: p.date ?? '',
      value: p.value ?? 0,
    })),
    retrieval_quality_series: (res.retrievalQualitySeries ?? []).map((p) => ({
      date: p.date ?? '',
      value: p.value ?? 0,
    })),
  };
}

type EvolutionSuggestionWire = {
  id?: string;
  agentId?: string;
  type?: string;
  title?: string;
  content?: string;
  status?: string;
  diffPreview?: string;
  createdAt?: string;
  appliedAt?: string;
  applicable?: boolean;
};

function toEvolutionSuggestion(item: EvolutionSuggestionWire): EvolutionSuggestion {
  return {
    id: item.id ?? '',
    agent_id: item.agentId ?? '',
    type: item.type ?? '',
    title: item.title ?? '',
    content: item.content ?? '',
    status: item.status ?? '',
    diff_preview: item.diffPreview ?? '',
    created_at: item.createdAt ?? '',
    applied_at: item.appliedAt ?? '',
    applicable: Boolean(item.applicable),
  };
}

export async function getAgentEvolutionSuggestions(agentId: string, status?: string): Promise<EvolutionSuggestion[]> {
  const svc = createAgentService();
  const res = await svc.GetAgentEvolutionSuggestions({ agentId, status });
  return (res.items ?? []).map(toEvolutionSuggestion);
}

export async function applyEvolutionSuggestion(agentId: string, suggestionId: string): Promise<EvolutionSuggestion> {
  const svc = createAgentService();
  const res = await svc.ApplyEvolutionSuggestion({ agentId, suggestionId });
  return toEvolutionSuggestion(res);
}

export async function checkAgentKey(agentKey: string): Promise<{ available: boolean; message: string }> {
  const svc = createAgentService();
  const res = await svc.CheckAgentKey({ agentKey });
  return {
    available: Boolean(res.available),
    message: res.message ?? '',
  };
}

export async function rejectEvolutionSuggestion(
  agentId: string,
  suggestionId: string,
  reason?: string,
): Promise<EvolutionSuggestion> {
  const svc = createAgentService();
  const res = await svc.RejectEvolutionSuggestion({ agentId, suggestionId, reason });
  return toEvolutionSuggestion(res);
}

export async function rollbackEvolutionSuggestion(agentId: string, suggestionId: string): Promise<EvolutionSuggestion> {
  const svc = createAgentService();
  const res = await svc.RollbackEvolutionSuggestion({ agentId, suggestionId });
  return toEvolutionSuggestion(res);
}

export async function updateAgentToolPolicy(
  agentId: string,
  payload: { tools_enabled?: boolean; profile?: string; allow?: string[]; deny?: string[] },
): Promise<void> {
  const svc = createAgentService();
  await svc.UpdateAgentToolPolicy({
    agentId,
    toolsEnabled: payload.tools_enabled,
    profile: payload.profile,
    allow: payload.allow,
    deny: payload.deny,
  });
}

export async function createAgentPromptFile(
  agentId: string,
  payload: { name: string; body: string; sort_order: number },
): Promise<AgentPromptFile> {
  const svc = createAgentService();
  const res = await svc.CreateAgentPromptFile({
    agentId,
    name: payload.name,
    body: payload.body,
    sortOrder: payload.sort_order,
  });
  return normalizePromptFileFromWire(res);
}

export async function updateAgentPromptFile(
  agentId: string,
  fileId: string,
  payload: { name?: string; body?: string; sort_order?: number },
): Promise<AgentPromptFile> {
  const svc = createAgentService();
  const res = await svc.UpdateAgentPromptFile({
    agentId,
    id: fileId,
    name: payload.name,
    body: payload.body,
    sortOrder: payload.sort_order,
  });
  return normalizePromptFileFromWire(res);
}

export async function deleteAgentPromptFile(agentId: string, fileId: string): Promise<void> {
  const svc = createAgentService();
  await svc.DeleteAgentPromptFile({ agentId, id: fileId });
}

export { listPlatformResources as listAgentDependencies, validateModel, type PlatformResource } from '../platform/api';
