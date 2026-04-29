import { createAgentService } from "./index";
import { legacyRestApi as api } from "./axiosHandler";
import { getBackendBaseURL } from "../config/runtime";
import type {
  Agent as KratosAgentWire,
  AgentRuntimeSettings as KratosRuntimeWire,
  AgentPromptFile as KratosFileWire,
  CreateAgentRequest as KratosCreateAgentRequest
} from "./kratos/agent/v1/index";

const kratosAgent = createAgentService();

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

function legacyRuntimeSettingsToKratos(s: AgentRuntimeSettings): KratosRuntimeWire {
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

function legacyPromptFileToKratos(f: AgentPromptFile): KratosFileWire {
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

function legacyPartialAgentToKratos(payload: Partial<Agent>): KratosAgentWire {
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
  if (payload.settings !== undefined) o.settings = legacyRuntimeSettingsToKratos(payload.settings);
  if (payload.files !== undefined) o.files = payload.files.map(legacyPromptFileToKratos);
  return o as KratosAgentWire;
}

/** 会话 HTTP 实现位于 `features/session/api.ts`；此处仅为 `@/api/client` 兼容 re-export。 */
export type {
  Session,
  SessionSearchQuery,
  SessionListResult,
  SessionTimelineItem,
  SessionTimelineSummary,
  SessionTimeline
} from "../features/session/api";

export type Team = {
  id: string;
  team_key: string;
  display_name: string;
  status: string;
  is_default: boolean;
  definition_json: string;
  adk_app_name: string;
  created_at: string;
  updated_at: string;
  deleted_at: string;
};

export type TeamDefinitionMember = {
  agent_id: string;
  role: "coordinator" | "worker" | "synthesizer" | "critic" | "generator" | string;
  name: string;
  enabled: boolean;
  sort_order: number;
};

export type TeamDefinitionGraphNode = {
  id: string;
  type: "start" | "agent" | "join" | "end" | string;
  label: string;
  agent_id?: string;
  role?: string;
  x?: number;
  y?: number;
};

export type TeamDefinitionGraphEdge = {
  id: string;
  source: string;
  target: string;
  label?: string;
  condition?: string;
};

export type TeamDefinition = {
  version: number;
  description?: string;
  mode: "sequential" | "parallel" | "coordinator" | "critic_loop" | "adaptive" | string;
  max_concurrency?: number;
  timeout_seconds?: number;
  members: TeamDefinitionMember[];
  a2a?: {
    enabled?: boolean;
    envelope_version?: string;
    message_format?: "markdown_json" | "plain" | string;
    include_trace?: boolean;
    max_payload_chars?: number;
  };
  graph?: {
    version?: number;
    layout?: "linear" | "parallel" | "loop" | "coordinator" | string;
    nodes: TeamDefinitionGraphNode[];
    edges: TeamDefinitionGraphEdge[];
  };
  synthesizer_agent_id?: string;
  critic_loop?: {
    max_iterations?: number;
    score_threshold?: number;
  };
};

export type TeamRun = {
  id: string;
  team_id: string;
  session_id: string;
  message_id: string;
  mode: string;
  status: string;
  input_preview: string;
  output_preview: string;
  token_in: number;
  token_out: number;
  cost_micro_usd: number;
  duration_ms: number;
  error_message: string;
  topology_json: string;
  started_at: string;
  finished_at: string;
  created_at: string;
  updated_at: string;
};

export type TeamRunStep = {
  id: string;
  run_id: string;
  team_id: string;
  agent_id: string;
  agent_key: string;
  agent_name: string;
  role: string;
  sort_order: number;
  status: string;
  input_preview: string;
  output_preview: string;
  token_in: number;
  token_out: number;
  cost_micro_usd: number;
  duration_ms: number;
  error_message: string;
  started_at: string;
  finished_at: string;
  created_at: string;
};

export type TeamRunEvent = {
  type: string;
  team_id: string;
  run_id: string;
  run?: TeamRun;
  step?: TeamRunStep;
};

export type Message = {
  id: string;
  session_id: string;
  parent_message_id: string;
  turn_index: number;
  role: string;
  content_markdown: string;
  model_name: string;
  token_in: number;
  token_out: number;
  latency_ms: number;
  status: string;
  attachments_count: number;
  options_json: string;
  error_message: string;
  created_at: string;
};

export type ModelUsageQuery = {
  range?: string;
  start_date?: string;
  end_date?: string;
  provider_code?: string;
  model_api_id?: string;
  agent_id?: string;
  status?: string;
  limit?: number;
};

export type ModelUsageSummary = {
  call_count: number;
  request_count: number;
  success_count: number;
  failed_count: number;
  cancelled_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  avg_latency_ms: number;
  avg_tokens_per_second: number;
  success_rate: number;
};

export type ModelUsageTrendPoint = {
  date_key: string;
  call_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  success_count: number;
  failed_count: number;
  cancelled_count: number;
  avg_latency_ms: number;
  avg_tokens_per_second: number;
};

export type ModelUsageBreakdownRow = {
  provider_code: string;
  model_api_id: string;
  model_display_name: string;
  agent_id: string;
  agent_key: string;
  call_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  avg_latency_ms: number;
  avg_tokens_per_second: number;
  success_rate: number;
};

export type ModelTokenUsageEvent = {
  id: string;
  occurred_at: string;
  agent_id: string;
  agent_key: string;
  session_id: string;
  message_id: string;
  provider_code: string;
  provider_type: string;
  provider_display_name: string;
  model_api_id: string;
  model_display_name: string;
  call_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  latency_ms: number;
  tokens_per_second: number;
  status: string;
  error_message: string;
  prompt_mode: string;
  max_output_tokens: number;
  context_window_k: number;
  stream_enabled: boolean;
};

export type ModelUsageOverview = {
  today: ModelUsageSummary;
  yesterday: ModelUsageSummary;
  month: ModelUsageSummary;
  range: ModelUsageSummary;
  trends: ModelUsageTrendPoint[];
  top_models: ModelUsageBreakdownRow[];
  top_agents: ModelUsageBreakdownRow[];
  anomalies: ModelTokenUsageEvent[];
};

export type ChatOption = {
  type: string;
  key: string;
  label: string;
  enabled: boolean;
  sort_order: number;
  metadata_json: string;
};

export type SendMessageOptions = {
  dialog_mode?: string;
  provider?: string;
  model?: string;
  attachments?: Array<{ id: string }>;
};

export type SendMessageResult = {
  user_message: Message;
  agent_message: Message;
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

export async function listAgents(query: AgentListQuery = {}): Promise<Agent[]> {
  const result = await listAgentsPaged(query);
  return result.items;
}

export async function listAgentsPaged(query: AgentListQuery = {}): Promise<AgentListResult> {
  const res = await kratosAgent.ListAgents({
    keyword: query.keyword,
    status: query.status,
    provider: query.provider,
    categoryId: query.category_id,
    limit: query.limit,
    offset: query.offset
  });
  return {
    items: (res.items ?? []) as Agent[],
    total: res.total ?? res.items?.length ?? 0,
    limit: res.limit ?? query.limit ?? 24,
    offset: res.offset ?? query.offset ?? 0
  };
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
    settings: payload.settings ? legacyRuntimeSettingsToKratos(payload.settings) : undefined,
    files: payload.files?.map(legacyPromptFileToKratos)
  };
  const data = await kratosAgent.CreateAgent(req);
  return data as Agent;
}

export async function getAgent(id: string): Promise<Agent> {
  const data = await kratosAgent.GetAgent({ id });
  return data as Agent;
}

export function subscribeTeamRunEvents(teamID: string, onEvent: (event: TeamRunEvent) => void, onError?: (error: Event) => void): EventSource {
  const query = new URLSearchParams({ team_id: teamID });
  const source = new EventSource(`${getBackendBaseURL()}/team-run-events?${query.toString()}`);
  for (const eventName of ["run_started", "step_finished", "run_finished"]) {
    source.addEventListener(eventName, (event) => {
      onEvent(JSON.parse((event as MessageEvent).data) as TeamRunEvent);
    });
  }
  source.onerror = (event) => {
    onError?.(event);
  };
  return source;
}

export async function updateAgent(id: string, payload: Partial<Agent>): Promise<Agent> {
  const data = await kratosAgent.UpdateAgent({
    id,
    agent: legacyPartialAgentToKratos(payload)
  });
  return data as Agent;
}

export async function getAgentPromptPreview(id: string, mode?: string): Promise<string> {
  const res = await kratosAgent.GetAgentPromptPreview({ id, mode });
  return res.preview ?? "";
}

export async function deleteAgent(id: string): Promise<void> {
  await kratosAgent.DeleteAgent({ id });
}

/** @deprecated 优先 `import { … } from "@/features/session/api"`；此处保留供 `export * from clientLegacy`。 */
export {
  archiveSession,
  clearAgentSessions,
  createSession,
  deleteSession,
  getSession,
  getSessionTimeline,
  listSessions,
  listTeamSessions,
  searchSessions,
  updateSessionTitle
} from "../features/session/api";

export async function listMessages(sessionID: string): Promise<Message[]> {
  const { data } = await api.get("/chat/messages", { params: { session_id: sessionID } });
  return data.items ?? [];
}

export async function sendMessage(payload: {
  session_id: string;
  agent_key?: string;
  team_id?: string;
  content: string;
  options?: SendMessageOptions;
}): Promise<SendMessageResult> {
  const { data } = await api.post("/chat/messages", payload);
  return data;
}

export type SendMessageStreamCallbacks = {
  signal?: AbortSignal;
  onUserMessage?: (message: Message) => void;
  onDelta?: (content: string) => void;
  onDone?: (message: Message) => void;
  onToolEvent?: (event: ToolUseEvent) => void;
  onMemberMessageStart?: (message: Message) => void;
  onMemberDelta?: (messageID: string, content: string) => void;
  onMemberMessageDone?: (message: Message) => void;
};

export type ToolUseEvent = {
  id: string;
  phase: "before" | "after" | string;
  status: "running" | "success" | "error" | "failed" | "blocked" | string;
  agent_id: string;
  agent_key: string;
  agent_name: string;
  agent_icon: string;
  tool_name: string;
  tool_label: string;
  arguments?: Record<string, unknown>;
  result?: Record<string, unknown>;
  error?: string;
  occurred_at: string;
  duration_ms?: number;
  message_hint?: string;
};

export async function sendMessageStream(
  payload: {
    session_id: string;
    agent_key?: string;
    team_id?: string;
    content: string;
    options?: SendMessageOptions;
  },
  callbacks: SendMessageStreamCallbacks = {}
): Promise<void> {
  const response = await fetch(`${getBackendBaseURL()}/chat/messages/stream`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
    signal: callbacks.signal
  });
  if (!response.ok || !response.body) {
    throw new Error((await response.text()) || `stream request failed: ${response.status}`);
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const events = buffer.split(/\n\n/);
    buffer = events.pop() ?? "";
    for (const eventBlock of events) {
      handleStreamEvent(eventBlock, callbacks);
    }
  }
  if (buffer.trim()) {
    handleStreamEvent(buffer, callbacks);
  }
}

function handleStreamEvent(block: string, callbacks: SendMessageStreamCallbacks) {
  const lines = block.split(/\r?\n/);
  const event = lines.find((line) => line.startsWith("event:"))?.replace(/^event:\s*/, "").trim();
  const data = lines
    .filter((line) => line.startsWith("data:"))
    .map((line) => line.replace(/^data:\s*/, ""))
    .join("\n");
  if (!event || !data) return;
  const parsed = JSON.parse(data);
  if (event === "user_message") {
    callbacks.onUserMessage?.(parsed as Message);
  } else if (event === "delta") {
    callbacks.onDelta?.(String(parsed.content ?? ""));
  } else if (event === "done") {
    callbacks.onDone?.(parsed.agent_message as Message);
  } else if (event === "tool_event") {
    callbacks.onToolEvent?.(parsed as ToolUseEvent);
  } else if (event === "member_message_start") {
    callbacks.onMemberMessageStart?.(parsed as Message);
  } else if (event === "member_delta") {
    callbacks.onMemberDelta?.(String(parsed.message_id ?? ""), String(parsed.content ?? ""));
  } else if (event === "member_message_done") {
    callbacks.onMemberMessageDone?.(parsed.agent_message as Message);
  } else if (event === "error") {
    throw new Error(String(parsed.message ?? "stream failed"));
  }
}

export async function listChatOptions(type?: string): Promise<ChatOption[]> {
  const { data } = await api.get("/chat/options", { params: type ? { type } : undefined });
  return data.items ?? [];
}

export async function getModelUsageOverview(query: ModelUsageQuery = {}): Promise<ModelUsageOverview> {
  const { data } = await api.get("/model-usage/overview", { params: cleanModelUsageQuery(query) });
  return data;
}

export async function listModelUsageTrends(query: ModelUsageQuery = {}): Promise<ModelUsageTrendPoint[]> {
  const { data } = await api.get("/model-usage/trends", { params: cleanModelUsageQuery(query) });
  return data.items ?? [];
}

export async function listModelUsageEvents(query: ModelUsageQuery = {}): Promise<ModelTokenUsageEvent[]> {
  const { data } = await api.get("/model-usage/events", { params: cleanModelUsageQuery(query) });
  return data.items ?? [];
}

function cleanModelUsageQuery(query: ModelUsageQuery) {
  return Object.fromEntries(
    Object.entries(query).filter(([, value]) => value !== "" && value !== undefined && value !== null)
  );
}

export type AuditLog = {
  id: string;
  action: string;
  resource: string;
  resource_id: string;
  request_id: string;
  detail: string;
  created_at: string;
};

export async function listAuditLogs(): Promise<AuditLog[]> {
  const { data } = await api.get("/monitor/audit");
  return data.items ?? [];
}

export type L0AssemblySegment = {
  section: string;
  role: string;
  source: string;
  tokens: number;
  preview: string;
  content?: string;
};

export type L0AssemblySnapshot = {
  id: string;
  session_id: string;
  run_id: string;
  turn_id: string;
  span_id: string;
  agent_id: string;
  team_id: string;
  provider: string;
  model: string;
  context_window_tokens: number;
  budget_tokens: number;
  recent_window_turns: number;
  recent_window_tokens: number;
  summary_token_estimate: number;
  l1_field_count: number;
  l1_token_estimate: number;
  l3_chunk_count: number;
  l3_token_estimate: number;
  l4_path_count: number;
  l4_token_estimate: number;
  prompt_token_estimate: number;
  prompt_token_actual: number;
  used_ratio: number;
  truncate_strategy: string;
  truncated_message_count: number;
  summarized_turn_from: number;
  summarized_turn_to: number;
  segments_json: string;
  warning_codes_json: string;
  metadata_json: string;
  created_at: string;
};

export type L1Task = {
  id: string;
  session_id: string;
  run_id: string;
  team_id: string;
  agent_id: string;
  task_key: string;
  task_title: string;
  task_goal: string;
  status: string;
  schema_version: number;
  budget_tokens: number;
  used_tokens: number;
  parent_task_id: string;
  shared_with_json?: string;
  started_at: string;
  ended_at: string;
  archived_at: string;
  metadata_json: string;
  created_at: string;
  updated_at: string;
};

export type L1Field = {
  id: string;
  task_id: string;
  session_id: string;
  agent_id: string;
  field_path: string;
  field_kind: string;
  visibility: string;
  pin_to_prompt: boolean;
  is_required: boolean;
  value_text: string;
  value_json: string;
  value_ref: string;
  preview: string;
  token_estimate: number;
  source: string;
  source_ref: string;
  ttl_seconds: number;
  expires_at: string;
  revision: number;
  last_read_at: string;
  read_count: number;
  metadata_json: string;
  created_at: string;
  updated_at: string;
};

export type MemoryFact = {
  id: string;
  scope_type: string;
  scope_id: string;
  workspace_id: string;
  user_id: string;
  team_id: string;
  agent_id: string;
  statement: string;
  details_markdown: string;
  fact_kind: string;
  tags_json: string;
  confidence: number;
  importance: number;
  use_count: number;
  hit_count: number;
  positive_feedback_count: number;
  negative_feedback_count: number;
  conflict_count: number;
  source_kind: string;
  source_episode_id: string;
  source_session_id: string;
  source_message_id: string;
  version: number;
  status: string;
  pii_flag: boolean;
  created_at: string;
  updated_at: string;
};

export type MemoryFactListQuery = {
  scope_type?: string;
  scope_id?: string;
  kind?: string;
  status?: string;
  keyword?: string;
  limit?: number;
  offset?: number;
};

export type MemoryFactListResult = {
  items: MemoryFact[];
  total: number;
  limit: number;
  offset: number;
};

export type MemoryEntity = {
  id: string;
  scope_type: string;
  scope_id: string;
  workspace_id?: string;
  user_id?: string;
  entity_type: string;
  name: string;
  name_normalized?: string;
  aliases?: string[];
  description?: string;
  importance: number;
  confidence: number;
  use_count: number;
  source_kind: string;
  status: string;
  created_at?: string;
  updated_at?: string;
};

export type MemoryRelation = {
  id: string;
  source_id: string;
  target_id: string;
  relation_type: string;
  weight: number;
  confidence: number;
  status: string;
};

export type GraphNeighborhood = {
  center: MemoryEntity;
  hops: number;
  entities: MemoryEntity[];
  relations: MemoryRelation[];
};

export type AgentIdentity = {
  agent_id: string;
  persona: string;
  values: string[];
  tone: string;
  domains: string[];
  user_expectations: string;
  current_phase: string;
  version: number;
};

export type AgentStrategyProfile = {
  agent_id: string;
  exploration: number;
  conciseness: number;
  caution: number;
  delegation: number;
  tool_preference: Record<string, number>;
  tool_blacklist: string[];
  provider_preference: Record<string, number>;
  model_preference: Record<string, number>;
  version: number;
};

export type EvolutionProposal = {
  id: string;
  agent_id: string;
  proposal_kind?: string;
  kind?: string;
  target_field: string;
  rationale: string;
  expected_impact: string;
  risk_level: string;
  status: string;
  created_at: string;
};

export type EvolutionEvent = {
  id: string;
  agent_id: string;
  event_kind?: string;
  kind?: string;
  target_field: string;
  reason: string;
  reverted: boolean;
  created_at: string;
};

export type EvolutionMetricsReport = {
  events_total: number;
  events_reverted: number;
  proposals_total: number;
  proposals_by_status: Record<string, number>;
  skill_stats: AgentSkillStat[];
};

export type AgentSkillStat = {
  agent_id: string;
  tool_key: string;
  invocations: number;
  successes: number;
  failures: number;
  preference_score: number;
  last_used_at: string;
};

export async function listL0Snapshots(sessionID: string, limit = 20): Promise<L0AssemblySnapshot[]> {
  const { data } = await api.get(`/sessions/${sessionID}/l0/snapshots`, { params: { limit } });
  return data.items ?? [];
}

export async function listL1Tasks(sessionID: string, params: { agent_id?: string; status?: string; include_ended?: boolean } = {}): Promise<L1Task[]> {
  const { data } = await api.get(`/sessions/${sessionID}/l1/tasks`, { params });
  return data.items ?? [];
}

export async function listL1Fields(sessionID: string, taskID: string, includeInternal = true): Promise<L1Field[]> {
  const { data } = await api.get(`/sessions/${sessionID}/l1/tasks/${taskID}/fields`, {
    params: { include_internal: includeInternal ? "true" : "false" }
  });
  return data.items ?? [];
}

export async function listMemoryFacts(query: MemoryFactListQuery = {}): Promise<MemoryFactListResult> {
  const { data } = await api.get("/memory/l3/facts", { params: query });
  const items = data.items ?? [];
  return {
    items,
    total: data.total ?? items.length,
    limit: data.limit ?? query.limit ?? items.length,
    offset: data.offset ?? query.offset ?? 0
  };
}

export async function listMemoryEntities(query: Record<string, string | number | undefined> = {}): Promise<{ items: MemoryEntity[]; total: number }> {
  const { data } = await api.get("/memory/l4/entities", { params: query });
  const items = data.items ?? [];
  return { items, total: data.total ?? items.length };
}

export async function getMemoryNeighborhood(centerID: string, params: { hops?: number; max_nodes?: number } = {}): Promise<GraphNeighborhood> {
  const { data } = await api.get(`/memory/l4/entities/${centerID}/neighborhood`, { params });
  return data;
}

export async function getAgentIdentity(agentID: string): Promise<AgentIdentity> {
  const { data } = await api.get(`/agents/${agentID}/identity`);
  return data;
}

export async function getAgentStrategy(agentID: string): Promise<AgentStrategyProfile> {
  const { data } = await api.get(`/agents/${agentID}/strategy`);
  return data;
}

export async function listEvolutionProposals(agentID: string, params: { status?: string; limit?: number } = {}): Promise<EvolutionProposal[]> {
  const { data } = await api.get(`/agents/${agentID}/evolution/proposals`, { params });
  return data.items ?? [];
}

export async function listEvolutionEvents(agentID: string, params: { limit?: number } = {}): Promise<EvolutionEvent[]> {
  const { data } = await api.get(`/agents/${agentID}/evolution/events`, { params });
  return data.items ?? [];
}

export async function getEvolutionMetrics(agentID: string, range = "30d"): Promise<EvolutionMetricsReport> {
  const { data } = await api.get(`/agents/${agentID}/evolution/metrics`, { params: { range } });
  return data;
}
