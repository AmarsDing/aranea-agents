import { createAgentService } from "../../services";
import type {
  Agent as KratosAgentWire,
  AgentRuntimeSettings as KratosRuntimeWire,
  AgentPromptFile as KratosFileWire,
  CreateAgentRequest as KratosCreateAgentRequest
} from "../../services/kratos/agent/v1/index";

export type Agent = {
  id: string;
  agent_key: string;
  display_name: string;
  provider: string;
  model: string;
  status: string;
  is_default: boolean;
  is_favorite: boolean;
  icon: string;
  agent_description: string;
  category_position_id: string;
  system_prompt_mode: string;
  context_window: number;
  budget_monthly_cents: number;
  config_json: string;
  created_at: string;
  updated_at: string;
  deleted_at: string;
  settings?: AgentRuntimeSettings;
  files?: AgentPromptFile[];
};

export type AgentRuntimeSettings = {
  agent_id?: string;
  self_evolve: boolean;
  subagents_enabled: boolean;
  subagents_max_concurrency: number;
  subagents_max_generation_depth: number;
  subagents_max_children_per_agent: number;
  subagents_archive_after_minutes: number;
  subagents_max_retries: number;
  subagents_model_override: string;
  tools_enabled: boolean;
  tools_profile: string;
  tools_tool_call_prefix: string;
  tools_allow_json: string;
  tools_deny_json: string;
  tools_concurrent_allow_json: string;
  memory_enabled: boolean;
  memory_max_chunk_length: number;
  memory_max_results: number;
  memory_min_score: number;
  l0_recent_window_turns?: number;
  l0_recent_window_tokens?: number;
  l0_summary_threshold?: number;
  l0_summary_keep_turns?: number;
  l0_truncate_strategy?: string;
  l0_inject_l1?: boolean;
  l0_inject_l3?: boolean;
  l0_inject_l4?: boolean;
  l0_l3_max_chunks?: number;
  l0_l4_max_paths?: number;
  l0_snapshot_mode?: string;
  l1_enabled?: boolean;
  l1_budget_tokens?: number;
  l1_field_max_tokens?: number;
  l1_history_keep_revisions?: number;
  l1_default_schema_id?: string;
  l1_archive_on_idle_minutes?: number;
  l2_episode_enabled?: boolean;
  l2_episode_min_importance?: number;
  l2_index_enabled?: boolean;
  l2_index_embedding_model?: string;
  l2_recall_enabled?: boolean;
  l2_recall_max?: number;
  l2_retention_days?: number;
  l2_archive_after_days?: number;
  l3_enabled?: boolean;
  l3_recall_top_k?: number;
  l3_recall_min_score?: number;
  l3_recall_scopes_json?: string;
  l3_embedding_model?: string;
  l3_decay_interval_hours?: number;
  l3_archive_threshold?: number;
  l3_max_per_recall_chars?: number;
  l4_enabled?: boolean;
  l4_graph_inject_neighbors?: boolean;
  l4_graph_max_neighbors?: number;
  l4_graph_max_hops?: number;
  l4_identity_inject?: boolean;
  l4_strategy_inject?: boolean;
  evo_enabled?: boolean;
  evo_auto_apply?: boolean;
  evo_min_episodes?: number;
  evo_min_negative_feedback?: number;
  evo_throttle_hours?: number;
  evo_proposal_ttl_days?: number;
  evo_persona_max_chars?: number;
  evo_system_prompt_max_appends?: number;
  heartbeat_enabled: boolean;
  heartbeat_interval_minutes: number;
  evolution_self_evolve: boolean;
  evolution_skill_evolve: boolean;
  evolution_metrics_enabled: boolean;
  evolution_suggestions_enabled: boolean;
  guardrail_max_change_per_period: number;
  guardrail_min_data_points: number;
  guardrail_rollback_on_decline_percent: number;
  created_at?: string;
  updated_at?: string;
};

export type AgentPromptFile = {
  id?: string;
  agent_id?: string;
  name: string;
  body: string;
  sort_order: number;
  created_at?: string;
  updated_at?: string;
};

export type AgentListQuery = {
  keyword?: string;
  status?: string;
  provider?: string;
  category_id?: string;
  limit?: number;
  offset?: number;
};

export type AgentListResult = {
  items: Agent[];
  total: number;
  limit: number;
  offset: number;
};

function runtimeSettingsToWire(s: AgentRuntimeSettings): KratosRuntimeWire {
  return {
    agentId: s.agent_id,
    selfEvolve: s.self_evolve,
    subagentsEnabled: s.subagents_enabled,
    subagentsMaxConcurrency: s.subagents_max_concurrency,
    subagentsMaxGenerationDepth: s.subagents_max_generation_depth,
    subagentsMaxChildrenPerAgent: s.subagents_max_children_per_agent,
    subagentsArchiveAfterMinutes: s.subagents_archive_after_minutes,
    subagentsMaxRetries: s.subagents_max_retries,
    subagentsModelOverride: s.subagents_model_override,
    toolsEnabled: s.tools_enabled,
    toolsProfile: s.tools_profile,
    toolsToolCallPrefix: s.tools_tool_call_prefix,
    toolsAllowJson: s.tools_allow_json,
    toolsDenyJson: s.tools_deny_json,
    toolsConcurrentAllowJson: s.tools_concurrent_allow_json,
    memoryEnabled: s.memory_enabled,
    memoryMaxChunkLength: s.memory_max_chunk_length,
    memoryMaxResults: s.memory_max_results,
    memoryMinScore: s.memory_min_score,
    heartbeatEnabled: s.heartbeat_enabled,
    heartbeatIntervalMinutes: s.heartbeat_interval_minutes,
    evolutionSelfEvolve: s.evolution_self_evolve,
    evolutionSkillEvolve: s.evolution_skill_evolve,
    evolutionMetricsEnabled: s.evolution_metrics_enabled,
    evolutionSuggestionsEnabled: s.evolution_suggestions_enabled,
    guardrailMaxChangePerPeriod: s.guardrail_max_change_per_period,
    guardrailMinDataPoints: s.guardrail_min_data_points,
    guardrailRollbackOnDeclinePercent: s.guardrail_rollback_on_decline_percent,
    l0RecentWindowTurns: s.l0_recent_window_turns,
    l0RecentWindowTokens: s.l0_recent_window_tokens,
    l0SummaryThreshold: s.l0_summary_threshold,
    l0SummaryKeepTurns: s.l0_summary_keep_turns,
    l0TruncateStrategy: s.l0_truncate_strategy,
    l0InjectL1: s.l0_inject_l1,
    l0InjectL3: s.l0_inject_l3,
    l0InjectL4: s.l0_inject_l4,
    l0L3MaxChunks: s.l0_l3_max_chunks,
    l0L4MaxPaths: s.l0_l4_max_paths,
    l0SnapshotMode: s.l0_snapshot_mode,
    l1Enabled: s.l1_enabled,
    l1BudgetTokens: s.l1_budget_tokens,
    l1FieldMaxTokens: s.l1_field_max_tokens,
    l1HistoryKeepRevisions: s.l1_history_keep_revisions,
    l1DefaultSchemaId: s.l1_default_schema_id,
    l1ArchiveOnIdleMinutes: s.l1_archive_on_idle_minutes,
    l2EpisodeEnabled: s.l2_episode_enabled,
    l2EpisodeMinImportance: s.l2_episode_min_importance,
    l2IndexEnabled: s.l2_index_enabled,
    l2IndexEmbeddingModel: s.l2_index_embedding_model,
    l2RecallEnabled: s.l2_recall_enabled,
    l2RecallMax: s.l2_recall_max,
    l2RetentionDays: s.l2_retention_days,
    l2ArchiveAfterDays: s.l2_archive_after_days,
    l3Enabled: s.l3_enabled,
    l3RecallTopK: s.l3_recall_top_k,
    l3RecallMinScore: s.l3_recall_min_score,
    l3RecallScopesJson: s.l3_recall_scopes_json,
    l3EmbeddingModel: s.l3_embedding_model,
    l3DecayIntervalHours: s.l3_decay_interval_hours,
    l3ArchiveThreshold: s.l3_archive_threshold,
    l3MaxPerRecallChars: s.l3_max_per_recall_chars,
    l4Enabled: s.l4_enabled,
    l4GraphInjectNeighbors: s.l4_graph_inject_neighbors,
    l4GraphMaxNeighbors: s.l4_graph_max_neighbors,
    l4GraphMaxHops: s.l4_graph_max_hops,
    l4IdentityInject: s.l4_identity_inject,
    l4StrategyInject: s.l4_strategy_inject,
    evoEnabled: s.evo_enabled,
    evoAutoApply: s.evo_auto_apply,
    evoMinEpisodes: s.evo_min_episodes,
    evoMinNegativeFeedback: s.evo_min_negative_feedback,
    evoThrottleHours: s.evo_throttle_hours,
    evoProposalTtlDays: s.evo_proposal_ttl_days,
    evoPersonaMaxChars: s.evo_persona_max_chars,
    evoSystemPromptMaxAppends: s.evo_system_prompt_max_appends,
    createdAt: s.created_at,
    updatedAt: s.updated_at
  };
}

function promptFileToWire(f: AgentPromptFile): KratosFileWire {
  return {
    id: f.id,
    agentId: f.agent_id,
    name: f.name,
    body: f.body,
    sortOrder: f.sort_order,
    createdAt: f.created_at,
    updatedAt: f.updated_at
  };
}

function partialAgentToWire(payload: Partial<Agent>): KratosAgentWire {
  const o: Partial<KratosAgentWire> = {};
  if (payload.id !== undefined) o.id = payload.id;
  if (payload.agent_key !== undefined) o.agentKey = payload.agent_key;
  if (payload.display_name !== undefined) o.displayName = payload.display_name;
  if (payload.provider !== undefined) o.provider = payload.provider;
  if (payload.model !== undefined) o.model = payload.model;
  if (payload.status !== undefined) o.status = payload.status;
  if (payload.is_default !== undefined) o.isDefault = payload.is_default;
  if (payload.is_favorite !== undefined) o.isFavorite = payload.is_favorite;
  if (payload.icon !== undefined) o.icon = payload.icon;
  if (payload.agent_description !== undefined) o.agentDescription = payload.agent_description;
  if (payload.category_position_id !== undefined) o.categoryPositionId = payload.category_position_id;
  if (payload.system_prompt_mode !== undefined) o.systemPromptMode = payload.system_prompt_mode;
  if (payload.context_window !== undefined) o.contextWindow = payload.context_window;
  if (payload.budget_monthly_cents !== undefined) o.budgetMonthlyCents = payload.budget_monthly_cents;
  if (payload.config_json !== undefined) o.configJson = payload.config_json;
  if (payload.created_at !== undefined) o.createdAt = payload.created_at;
  if (payload.updated_at !== undefined) o.updatedAt = payload.updated_at;
  if (payload.deleted_at !== undefined) o.deletedAt = payload.deleted_at;
  if (payload.settings !== undefined) o.settings = runtimeSettingsToWire(payload.settings);
  if (payload.files !== undefined) o.files = payload.files.map(promptFileToWire);
  return o as KratosAgentWire;
}

/** Agent 目录：Kratos `agent/v1`（与 {@link createAgentService} 一致）。 */
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
    items: (res.items ?? []) as unknown as Agent[],
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
  return data as unknown as Agent;
}

export async function getAgent(id: string): Promise<Agent> {
  const svc = createAgentService();
  const data = await svc.GetAgent({ id });
  return data as unknown as Agent;
}

export async function updateAgent(id: string, payload: Partial<Agent>): Promise<Agent> {
  const svc = createAgentService();
  const data = await svc.UpdateAgent({
    id,
    agent: partialAgentToWire(payload)
  });
  return data as unknown as Agent;
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

export { useAgentsPage, type CreateAgentForm } from "./useAgentsPage";
