import type {
  Agent,
  AgentPromptFile,
  AgentRuntimeSettings,
  A2AProxyConfig
} from "./types";
import type {
  Agent as KratosAgentWire,
  AgentRuntimeSettings as KratosRuntimeWire,
  AgentPromptFile as KratosFileWire
} from "../../services/kratos/agent/v1/index";

function asWireRecord(v: unknown): Record<string, unknown> {
  return v !== null && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : {};
}

function pickStr(w: Record<string, unknown>, camel: string, snake: string, fallback = ""): string {
  const v = w[camel] ?? w[snake];
  if (v === null || v === undefined) return fallback;
  return String(v);
}

function pickStrOpt(w: Record<string, unknown>, camel: string, snake: string): string | undefined {
  const v = w[camel] ?? w[snake];
  if (v === null || v === undefined) return undefined;
  const s = String(v);
  return s === "" ? undefined : s;
}

function pickNum(w: Record<string, unknown>, camel: string, snake: string, fallback: number): number {
  const v = w[camel] ?? w[snake];
  if (v === null || v === undefined) return fallback;
  const n = Number(v);
  return Number.isFinite(n) ? n : fallback;
}

function pickNumOpt(w: Record<string, unknown>, camel: string, snake: string): number | undefined {
  const v = w[camel] ?? w[snake];
  if (v === null || v === undefined) return undefined;
  const n = Number(v);
  return Number.isFinite(n) ? n : undefined;
}

function pickBool(w: Record<string, unknown>, camel: string, snake: string, fallback: boolean): boolean {
  const v = w[camel] ?? w[snake];
  if (typeof v === "boolean") return v;
  return fallback;
}

function pickBoolOpt(w: Record<string, unknown>, camel: string, snake: string): boolean | undefined {
  const v = w[camel] ?? w[snake];
  if (typeof v === "boolean") return v;
  return undefined;
}

export function normalizePromptFileFromWire(raw: unknown): AgentPromptFile {
  const w = asWireRecord(raw);
  return {
    id: pickStrOpt(w, "id", "id"),
    agent_id: pickStrOpt(w, "agentId", "agent_id"),
    name: pickStr(w, "name", "name"),
    body: pickStr(w, "body", "body"),
    sort_order: pickNum(w, "sortOrder", "sort_order", 0),
    created_at: pickStrOpt(w, "createdAt", "created_at"),
    updated_at: pickStrOpt(w, "updatedAt", "updated_at")
  };
}

export function normalizeRuntimeSettingsFromWire(raw: unknown): AgentRuntimeSettings | undefined {
  if (raw === null || raw === undefined) return undefined;
  const w = asWireRecord(raw);
  if (Object.keys(w).length === 0) return undefined;
  return {
    agent_id: pickStrOpt(w, "agentId", "agent_id"),
    self_evolve: pickBool(w, "selfEvolve", "self_evolve", true),
    subagents_enabled: pickBool(w, "subagentsEnabled", "subagents_enabled", true),
    subagents_max_concurrency: pickNum(w, "subagentsMaxConcurrency", "subagents_max_concurrency", 20),
    subagents_max_generation_depth: pickNum(w, "subagentsMaxGenerationDepth", "subagents_max_generation_depth", 1),
    subagents_max_children_per_agent: pickNum(w, "subagentsMaxChildrenPerAgent", "subagents_max_children_per_agent", 5),
    subagents_archive_after_minutes: pickNum(w, "subagentsArchiveAfterMinutes", "subagents_archive_after_minutes", 60),
    subagents_max_retries: pickNum(w, "subagentsMaxRetries", "subagents_max_retries", 2),
    subagents_model_override: pickStr(w, "subagentsModelOverride", "subagents_model_override", ""),
    tools_enabled: pickBool(w, "toolsEnabled", "tools_enabled", true),
    tools_profile: pickStr(w, "toolsProfile", "tools_profile", "coding"),
    tools_tool_call_prefix: pickStr(w, "toolsToolCallPrefix", "tools_tool_call_prefix", ""),
    tools_allow_json: pickStr(w, "toolsAllowJson", "tools_allow_json", "[]"),
    tools_deny_json: pickStr(w, "toolsDenyJson", "tools_deny_json", "[]"),
    tools_concurrent_allow_json: pickStr(w, "toolsConcurrentAllowJson", "tools_concurrent_allow_json", "[]"),
    memory_enabled: pickBool(w, "memoryEnabled", "memory_enabled", true),
    memory_max_chunk_length: pickNum(w, "memoryMaxChunkLength", "memory_max_chunk_length", 1000),
    memory_max_results: pickNum(w, "memoryMaxResults", "memory_max_results", 6),
    memory_min_score: pickNum(w, "memoryMinScore", "memory_min_score", 0.35),
    l0_recent_window_turns: pickNumOpt(w, "l0RecentWindowTurns", "l0_recent_window_turns"),
    l0_recent_window_tokens: pickNumOpt(w, "l0RecentWindowTokens", "l0_recent_window_tokens"),
    l0_summary_threshold: pickNumOpt(w, "l0SummaryThreshold", "l0_summary_threshold"),
    l0_summary_keep_turns: pickNumOpt(w, "l0SummaryKeepTurns", "l0_summary_keep_turns"),
    l0_compress_provider: pickStrOpt(w, "l0CompressProvider", "l0_compress_provider"),
    l0_compress_model: pickStrOpt(w, "l0CompressModel", "l0_compress_model"),
    memory_worker_provider: pickStrOpt(w, "memoryWorkerProvider", "memory_worker_provider"),
    memory_worker_model: pickStrOpt(w, "memoryWorkerModel", "memory_worker_model"),
    l0_truncate_strategy: pickStrOpt(w, "l0TruncateStrategy", "l0_truncate_strategy"),
    l0_inject_l1: pickBoolOpt(w, "l0InjectL1", "l0_inject_l1"),
    l0_inject_l3: pickBoolOpt(w, "l0InjectL3", "l0_inject_l3"),
    l0_inject_l4: pickBoolOpt(w, "l0InjectL4", "l0_inject_l4"),
    l0_l3_max_chunks: pickNumOpt(w, "l0L3MaxChunks", "l0_l3_max_chunks"),
    l0_l4_max_paths: pickNumOpt(w, "l0L4MaxPaths", "l0_l4_max_paths"),
    l0_snapshot_mode: pickStrOpt(w, "l0SnapshotMode", "l0_snapshot_mode"),
    l1_enabled: pickBoolOpt(w, "l1Enabled", "l1_enabled"),
    l1_budget_tokens: pickNumOpt(w, "l1BudgetTokens", "l1_budget_tokens"),
    l1_field_max_tokens: pickNumOpt(w, "l1FieldMaxTokens", "l1_field_max_tokens"),
    l1_history_keep_revisions: pickNumOpt(w, "l1HistoryKeepRevisions", "l1_history_keep_revisions"),
    l1_default_schema_id: pickStrOpt(w, "l1DefaultSchemaId", "l1_default_schema_id"),
    l1_archive_on_idle_minutes: pickNumOpt(w, "l1ArchiveOnIdleMinutes", "l1_archive_on_idle_minutes"),
    l2_episode_enabled: pickBoolOpt(w, "l2EpisodeEnabled", "l2_episode_enabled"),
    l2_episode_min_importance: pickNumOpt(w, "l2EpisodeMinImportance", "l2_episode_min_importance"),
    l2_index_enabled: pickBoolOpt(w, "l2IndexEnabled", "l2_index_enabled"),
    l2_index_embedding_model: pickStrOpt(w, "l2IndexEmbeddingModel", "l2_index_embedding_model"),
    l2_recall_enabled: pickBoolOpt(w, "l2RecallEnabled", "l2_recall_enabled"),
    l2_recall_max: pickNumOpt(w, "l2RecallMax", "l2_recall_max"),
    l2_retention_days: pickNumOpt(w, "l2RetentionDays", "l2_retention_days"),
    l2_archive_after_days: pickNumOpt(w, "l2ArchiveAfterDays", "l2_archive_after_days"),
    l3_enabled: pickBoolOpt(w, "l3Enabled", "l3_enabled"),
    l3_recall_top_k: pickNumOpt(w, "l3RecallTopK", "l3_recall_top_k"),
    l3_recall_min_score: pickNumOpt(w, "l3RecallMinScore", "l3_recall_min_score"),
    l3_recall_scopes_json: pickStrOpt(w, "l3RecallScopesJson", "l3_recall_scopes_json"),
    l3_embedding_model: pickStrOpt(w, "l3EmbeddingModel", "l3_embedding_model"),
    l3_decay_interval_hours: pickNumOpt(w, "l3DecayIntervalHours", "l3_decay_interval_hours"),
    l3_archive_threshold: pickNumOpt(w, "l3ArchiveThreshold", "l3_archive_threshold"),
    l3_max_per_recall_chars: pickNumOpt(w, "l3MaxPerRecallChars", "l3_max_per_recall_chars"),
    l4_enabled: pickBoolOpt(w, "l4Enabled", "l4_enabled"),
    l4_graph_inject_neighbors: pickBoolOpt(w, "l4GraphInjectNeighbors", "l4_graph_inject_neighbors"),
    l4_graph_max_neighbors: pickNumOpt(w, "l4GraphMaxNeighbors", "l4_graph_max_neighbors"),
    l4_graph_max_hops: pickNumOpt(w, "l4GraphMaxHops", "l4_graph_max_hops"),
    l4_identity_inject: pickBoolOpt(w, "l4IdentityInject", "l4_identity_inject"),
    l4_strategy_inject: pickBoolOpt(w, "l4StrategyInject", "l4_strategy_inject"),
    evo_enabled: pickBoolOpt(w, "evoEnabled", "evo_enabled"),
    evo_auto_apply: pickBoolOpt(w, "evoAutoApply", "evo_auto_apply"),
    evo_min_episodes: pickNumOpt(w, "evoMinEpisodes", "evo_min_episodes"),
    evo_min_negative_feedback: pickNumOpt(w, "evoMinNegativeFeedback", "evo_min_negative_feedback"),
    evo_throttle_hours: pickNumOpt(w, "evoThrottleHours", "evo_throttle_hours"),
    evo_proposal_ttl_days: pickNumOpt(w, "evoProposalTtlDays", "evo_proposal_ttl_days"),
    evo_persona_max_chars: pickNumOpt(w, "evoPersonaMaxChars", "evo_persona_max_chars"),
    evo_system_prompt_max_appends: pickNumOpt(w, "evoSystemPromptMaxAppends", "evo_system_prompt_max_appends"),
    heartbeat_enabled: pickBool(w, "heartbeatEnabled", "heartbeat_enabled", false),
    heartbeat_interval_minutes: pickNum(w, "heartbeatIntervalMinutes", "heartbeat_interval_minutes", 30),
    evolution_self_evolve: pickBool(w, "evolutionSelfEvolve", "evolution_self_evolve", true),
    evolution_skill_evolve: pickBool(w, "evolutionSkillEvolve", "evolution_skill_evolve", true),
    evolution_metrics_enabled: pickBool(w, "evolutionMetricsEnabled", "evolution_metrics_enabled", true),
    evolution_suggestions_enabled: pickBool(w, "evolutionSuggestionsEnabled", "evolution_suggestions_enabled", true),
    guardrail_max_change_per_period: pickNum(w, "guardrailMaxChangePerPeriod", "guardrail_max_change_per_period", 0.1),
    guardrail_min_data_points: pickNum(w, "guardrailMinDataPoints", "guardrail_min_data_points", 100),
    guardrail_rollback_on_decline_percent: pickNum(w, "guardrailRollbackOnDeclinePercent", "guardrail_rollback_on_decline_percent", 20),
    skill_runtime_json: pickStr(w, "skillRuntimeJson", "skill_runtime_json", "{}"),
    intent_pass_enabled: pickBool(w, "intentPassEnabled", "intent_pass_enabled", false),
    variables_json: pickStrOpt(w, "variablesJson", "variables_json"),
    model_instructions_json: pickStrOpt(w, "modelInstructionsJson", "model_instructions_json"),
    context_compaction_enabled: pickBoolOpt(w, "contextCompactionEnabled", "context_compaction_enabled"),
    session_summary_enabled: pickBoolOpt(w, "sessionSummaryEnabled", "session_summary_enabled"),
    skill_load_mode: pickStrOpt(w, "skillLoadMode", "skill_load_mode"),
    code_executor_type: pickStrOpt(w, "codeExecutorType", "code_executor_type"),
    output_schema_json: pickStrOpt(w, "outputSchemaJson", "output_schema_json"),
    model_selector: pickStrOpt(w, "modelSelector", "model_selector"),
    channel_id: pickStrOpt(w, "channelId", "channel_id"),
    chat_id: pickStrOpt(w, "chatId", "chat_id"),
    workspace: pickStrOpt(w, "workspace", "workspace"),
    reasoning_mode: pickStrOpt(w, "reasoningMode", "reasoning_mode"),
    reasoning_level: pickStrOpt(w, "reasoningLevel", "reasoning_level"),
    planner_kind: pickStrOpt(w, "plannerKind", "planner_kind"),
    planner_config_json: pickStrOpt(w, "plannerConfigJson", "planner_config_json"),
    ralph_loop_max_iterations: pickNumOpt(w, "ralphLoopMaxIterations", "ralph_loop_max_iterations"),
    ralph_loop_completion_promise: pickStrOpt(w, "ralphLoopCompletionPromise", "ralph_loop_completion_promise"),
    ralph_loop_verify_command: pickStrOpt(w, "ralphLoopVerifyCommand", "ralph_loop_verify_command"),
    ralph_loop_verify_timeout_seconds: pickNumOpt(w, "ralphLoopVerifyTimeoutSeconds", "ralph_loop_verify_timeout_seconds"),
    ralph_loop_promise_tag_open: pickStrOpt(w, "ralphLoopPromiseTagOpen", "ralph_loop_promise_tag_open"),
    ralph_loop_promise_tag_close: pickStrOpt(w, "ralphLoopPromiseTagClose", "ralph_loop_promise_tag_close"),
    ralph_loop_verify_work_dir: pickStrOpt(w, "ralphLoopVerifyWorkDir", "ralph_loop_verify_work_dir"),
    created_at: pickStrOpt(w, "createdAt", "created_at"),
    updated_at: pickStrOpt(w, "updatedAt", "updated_at")
  };
}

export function normalizeA2AProxyFromWire(raw: unknown): A2AProxyConfig | undefined {
  if (raw === null || raw === undefined) return undefined;
  const w = asWireRecord(raw);
  const remote = pickStr(w, "remoteUrl", "remote_url");
  if (!remote) return undefined;
  return {
    remote_url: remote,
    agent_card_url: pickStrOpt(w, "agentCardUrl", "agent_card_url"),
    enable_streaming: pickBoolOpt(w, "enableStreaming", "enable_streaming"),
    auth_type: pickStrOpt(w, "authType", "auth_type"),
    auth_config_json: pickStrOpt(w, "authConfigJson", "auth_config_json"),
    timeout_seconds: pickNumOpt(w, "timeoutSeconds", "timeout_seconds")
  };
}

export function a2aProxyToWire(cfg: A2AProxyConfig) {
  return {
    remoteUrl: cfg.remote_url,
    agentCardUrl: cfg.agent_card_url,
    enableStreaming: cfg.enable_streaming,
    authType: cfg.auth_type,
    authConfigJson: cfg.auth_config_json,
    timeoutSeconds: cfg.timeout_seconds
  };
}

export function normalizeAgentFromService(raw: unknown): Agent {
  const w = asWireRecord(raw);
  const filesRaw = w.files;
  let files: AgentPromptFile[] | undefined;
  if (Array.isArray(filesRaw)) {
    files = filesRaw.map((item) => normalizePromptFileFromWire(item));
  }
  return {
    id: pickStr(w, "id", "id"),
    agent_key: pickStr(w, "agentKey", "agent_key"),
    display_name: pickStr(w, "displayName", "display_name"),
    provider: pickStr(w, "provider", "provider"),
    model: pickStr(w, "model", "model"),
    status: pickStr(w, "status", "status", "active"),
    is_default: pickBool(w, "isDefault", "is_default", false),
    is_favorite: pickBool(w, "isFavorite", "is_favorite", false),
    icon: pickStr(w, "icon", "icon"),
    agent_description: pickStr(w, "agentDescription", "agent_description"),
    position_key: pickStrOpt(w, "positionKey", "position_key"),
    agent_variant: pickStrOpt(w, "agentVariant", "agent_variant"),
    variant_description: pickStrOpt(w, "variantDescription", "variant_description"),
    taxonomy_position_id: pickStr(w, "taxonomyPositionId", "taxonomy_position_id"),
    system_prompt_mode: pickStr(w, "systemPromptMode", "system_prompt_mode", "complete"),
    context_window: pickNum(w, "contextWindow", "context_window", 0),
    budget_monthly_cents: pickNum(w, "budgetMonthlyCents", "budget_monthly_cents", 0),
    config_json: pickStr(w, "configJson", "config_json"),
    created_by: pickStrOpt(w, "createdBy", "created_by"),
    created_at: pickStr(w, "createdAt", "created_at"),
    updated_at: pickStr(w, "updatedAt", "updated_at"),
    deleted_at: pickStr(w, "deletedAt", "deleted_at"),
    agent_kind: (pickStr(w, "agentKind", "agent_kind", "llm") || "llm") as Agent["agent_kind"],
    a2a_proxy_config: normalizeA2AProxyFromWire(w.a2aProxyConfig ?? w.a2a_proxy_config),
    a2a_endpoint_enabled: pickBool(w, "a2aEndpointEnabled", "a2a_endpoint_enabled", false),
    last_run_status: pickStrOpt(w, "lastRunStatus", "last_run_status"),
    last_run_at: pickStrOpt(w, "lastRunAt", "last_run_at"),
    pending_evolution_count: pickNum(w, "pendingEvolutionCount", "pending_evolution_count", 0),
    readonly: pickBool(w, "readonly", "readonly", false),
    source: pickStrOpt(w, "source", "source"),
    settings: normalizeRuntimeSettingsFromWire(w.settings),
    files
  };
}

export function runtimeSettingsToWire(s: AgentRuntimeSettings): KratosRuntimeWire {
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
    l0CompressProvider: s.l0_compress_provider,
    l0CompressModel: s.l0_compress_model,
    memoryWorkerProvider: s.memory_worker_provider,
    memoryWorkerModel: s.memory_worker_model,
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
    skillRuntimeJson: s.skill_runtime_json ?? "{}",
    intentPassEnabled: s.intent_pass_enabled ?? true,
    variablesJson: s.variables_json,
    modelInstructionsJson: s.model_instructions_json,
    contextCompactionEnabled: s.context_compaction_enabled,
    sessionSummaryEnabled: s.session_summary_enabled,
    skillLoadMode: s.skill_load_mode,
    codeExecutorType: s.code_executor_type,
    outputSchemaJson: s.output_schema_json,
    modelSelector: s.model_selector,
    toolsRetryEnabled: s.tools_retry_enabled,
    toolsRetryMaxAttempts: s.tools_retry_max_attempts,
    toolsRetryInitialIntervalMs: s.tools_retry_initial_interval_ms,
    toolsRetryBackoffFactor: s.tools_retry_backoff_factor,
    toolsRetryMaxIntervalMs: s.tools_retry_max_interval_ms,
    toolsRetryJitter: s.tools_retry_jitter,
    toolsParallelEnabled: s.tools_parallel_enabled,
    toolsStreamingEnabled: s.tools_streaming_enabled,
    channelId: s.channel_id,
    chatId: s.chat_id,
    workspace: s.workspace,
    reasoningMode: s.reasoning_mode,
    reasoningLevel: s.reasoning_level,
    plannerKind: s.planner_kind,
    plannerConfigJson: s.planner_config_json,
    ralphLoopMaxIterations: s.ralph_loop_max_iterations,
    ralphLoopCompletionPromise: s.ralph_loop_completion_promise,
    ralphLoopVerifyCommand: s.ralph_loop_verify_command,
    ralphLoopVerifyTimeoutSeconds: s.ralph_loop_verify_timeout_seconds,
    ralphLoopPromiseTagOpen: s.ralph_loop_promise_tag_open,
    ralphLoopPromiseTagClose: s.ralph_loop_promise_tag_close,
    ralphLoopVerifyWorkDir: s.ralph_loop_verify_work_dir,
    createdAt: s.created_at,
    updatedAt: s.updated_at,
  };
}

export function promptFileToWire(f: AgentPromptFile): KratosFileWire {
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

export function partialAgentToWire(payload: Partial<Agent>): KratosAgentWire {
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
  if (payload.position_key !== undefined) o.positionKey = payload.position_key;
  if (payload.agent_variant !== undefined) o.agentVariant = payload.agent_variant;
  if (payload.variant_description !== undefined) o.variantDescription = payload.variant_description;
  if (payload.taxonomy_position_id !== undefined) o.taxonomyPositionId = payload.taxonomy_position_id;
  if (payload.system_prompt_mode !== undefined) o.systemPromptMode = payload.system_prompt_mode;
  if (payload.context_window !== undefined) o.contextWindow = payload.context_window;
  if (payload.budget_monthly_cents !== undefined) o.budgetMonthlyCents = payload.budget_monthly_cents;
  if (payload.config_json !== undefined) o.configJson = payload.config_json;
  if (payload.created_at !== undefined) o.createdAt = payload.created_at;
  if (payload.updated_at !== undefined) o.updatedAt = payload.updated_at;
  if (payload.deleted_at !== undefined) o.deletedAt = payload.deleted_at;
  if (payload.settings !== undefined) o.settings = runtimeSettingsToWire(payload.settings);
  if (payload.files !== undefined) o.files = payload.files.map(promptFileToWire);
  if (payload.agent_kind !== undefined) o.agentKind = payload.agent_kind;
  if (payload.a2a_proxy_config !== undefined) o.a2aProxyConfig = a2aProxyToWire(payload.a2a_proxy_config);
  return o as KratosAgentWire;
}
