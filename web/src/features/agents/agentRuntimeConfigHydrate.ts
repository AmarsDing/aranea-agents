import type { Agent, AgentRuntimeSettings } from "./types";
import {
  defaultAgentAdvancedSettings,
  defaultAgentRuntimeConfig,
  parseJSONList,
  type AgentAdvancedSettingsForm,
  type AgentRuntimeConfigForm,
} from "./agentRuntimeConfig";
import { parseSkillRuntimeForm } from "./agentSkillRuntimeConfig";

type AgentFileLike = { name: string; body: string; id?: string };

export function hydrateRuntimeFromConfigJson(
  config: AgentRuntimeConfigForm,
  raw: string,
  files?: AgentFileLike[],
) {
  try {
    const parsed = JSON.parse(raw || "{}");
    Object.assign(config, {
      ...config,
      ...parsed,
      subagents: { ...config.subagents, ...(parsed.subagents || {}) },
      tools: {
        ...config.tools,
        ...(parsed.tools || {}),
        retry: { ...config.tools.retry, ...((parsed.tools || {}).retry || {}) },
      },
      memory: { ...config.memory, ...(parsed.memory || {}) },
      memoryL0: { ...config.memoryL0, ...(parsed.memoryL0 || {}) },
      memoryL1: { ...config.memoryL1, ...(parsed.memoryL1 || {}) },
      memoryL2: { ...config.memoryL2, ...(parsed.memoryL2 || {}) },
      memoryL3: { ...config.memoryL3, ...(parsed.memoryL3 || {}) },
      memoryL4: { ...config.memoryL4, ...(parsed.memoryL4 || {}) },
      evolutionSettings: { ...config.evolutionSettings, ...(parsed.evolutionSettings || {}) },
      heartbeat: { ...config.heartbeat, ...(parsed.heartbeat || {}) },
      evolution: { ...config.evolution, ...(parsed.evolution || {}), self_evolve: parsed.self_evolve ?? config.self_evolve },
      evolution_guardrails: { ...config.evolution_guardrails, ...(parsed.evolution_guardrails || {}) },
      skillRuntime: { ...config.skillRuntime, ...(parsed.skillRuntime || {}) },
      intent_pass: { ...config.intent_pass, ...(parsed.intent_pass || {}) },
    });
    if (Array.isArray(parsed.files) && files) {
      for (const saved of parsed.files) {
        const file = files.find((item) => item.name === saved.name);
        if (file) file.body = saved.body;
      }
    }
  } catch {
    // Legacy config can be plain text; keep defaults.
  }
}

export function hydrateAdvancedFromSettings(
  advanced: AgentAdvancedSettingsForm,
  settings: AgentRuntimeSettings,
) {
  Object.assign(advanced, {
    channel_id: settings.channel_id || "",
    chat_id: settings.chat_id || "",
    workspace: settings.workspace || "",
    reasoning_mode: settings.reasoning_mode || "provider_default",
    reasoning_level: settings.reasoning_level || "off",
    context_compaction_enabled: settings.context_compaction_enabled ?? false,
    session_summary_enabled: settings.session_summary_enabled ?? false,
    truncate_strategy: settings.l0_truncate_strategy || "sliding",
    recent_window_turns: settings.l0_recent_window_turns ?? 20,
    recent_window_tokens: settings.l0_recent_window_tokens ?? 0,
    summary_keep_turns: settings.l0_summary_keep_turns ?? 4,
  });
}

export function hydrateRuntimeFromSettings(
  config: AgentRuntimeConfigForm,
  advanced: AgentAdvancedSettingsForm,
  settings: AgentRuntimeSettings,
) {
  Object.assign(config, {
    ...config,
    self_evolve: settings.self_evolve,
    subagents: {
      enabled: settings.subagents_enabled,
      max_concurrency: settings.subagents_max_concurrency,
      max_generation_depth: settings.subagents_max_generation_depth,
      max_children_per_agent: settings.subagents_max_children_per_agent,
      archive_after_minutes: settings.subagents_archive_after_minutes,
      max_retries: settings.subagents_max_retries,
      model_override: settings.subagents_model_override,
    },
    tools: {
      enabled: settings.tools_enabled,
      profile: settings.tools_profile,
      tool_call_prefix: settings.tools_tool_call_prefix,
      allow: parseJSONList(settings.tools_allow_json),
      deny: parseJSONList(settings.tools_deny_json),
      concurrent_allow: parseJSONList(settings.tools_concurrent_allow_json),
      retry: {
        enabled: settings.tools_retry_enabled ?? config.tools.retry.enabled,
        max_attempts: settings.tools_retry_max_attempts ?? config.tools.retry.max_attempts,
        initial_interval_ms: settings.tools_retry_initial_interval_ms ?? config.tools.retry.initial_interval_ms,
        backoff_factor: settings.tools_retry_backoff_factor ?? config.tools.retry.backoff_factor,
        max_interval_ms: settings.tools_retry_max_interval_ms ?? config.tools.retry.max_interval_ms,
        jitter: settings.tools_retry_jitter ?? config.tools.retry.jitter,
      },
      parallel_enabled: settings.tools_parallel_enabled ?? config.tools.parallel_enabled,
      streaming_enabled: settings.tools_streaming_enabled ?? config.tools.streaming_enabled,
    },
    memory: {
      enabled: settings.memory_enabled,
      max_chunk_length: settings.memory_max_chunk_length,
      max_results: settings.memory_max_results,
      min_score: settings.memory_min_score,
    },
    memoryL0: {
      recent_window_turns: settings.l0_recent_window_turns ?? config.memoryL0.recent_window_turns,
      recent_window_tokens: settings.l0_recent_window_tokens ?? config.memoryL0.recent_window_tokens,
      summary_threshold: settings.l0_summary_threshold ?? config.memoryL0.summary_threshold,
      summary_keep_turns: settings.l0_summary_keep_turns ?? config.memoryL0.summary_keep_turns,
      truncate_strategy: settings.l0_truncate_strategy || config.memoryL0.truncate_strategy,
      inject_l1: settings.l0_inject_l1 ?? config.memoryL0.inject_l1,
      inject_l3: settings.l0_inject_l3 ?? config.memoryL0.inject_l3,
      inject_l4: settings.l0_inject_l4 ?? config.memoryL0.inject_l4,
      l3_max_chunks: settings.l0_l3_max_chunks ?? config.memoryL0.l3_max_chunks,
      l4_max_paths: settings.l0_l4_max_paths ?? config.memoryL0.l4_max_paths,
      snapshot_mode: settings.l0_snapshot_mode || config.memoryL0.snapshot_mode,
    },
    memoryL1: {
      enabled: settings.l1_enabled ?? config.memoryL1.enabled,
      budget_tokens: settings.l1_budget_tokens ?? config.memoryL1.budget_tokens,
      field_max_tokens: settings.l1_field_max_tokens ?? config.memoryL1.field_max_tokens,
      history_keep_revisions: settings.l1_history_keep_revisions ?? config.memoryL1.history_keep_revisions,
      default_schema_id: settings.l1_default_schema_id || config.memoryL1.default_schema_id,
      archive_on_idle_minutes: settings.l1_archive_on_idle_minutes ?? config.memoryL1.archive_on_idle_minutes,
    },
    memoryL2: {
      episode_enabled: settings.l2_episode_enabled ?? config.memoryL2.episode_enabled,
      episode_min_importance: settings.l2_episode_min_importance ?? config.memoryL2.episode_min_importance,
      index_enabled: settings.l2_index_enabled ?? config.memoryL2.index_enabled,
      index_embedding_model: settings.l2_index_embedding_model || config.memoryL2.index_embedding_model,
      recall_enabled: settings.l2_recall_enabled ?? config.memoryL2.recall_enabled,
      recall_max: settings.l2_recall_max ?? config.memoryL2.recall_max,
      retention_days: settings.l2_retention_days ?? config.memoryL2.retention_days,
      archive_after_days: settings.l2_archive_after_days ?? config.memoryL2.archive_after_days,
    },
    memoryL3: {
      enabled: settings.l3_enabled ?? config.memoryL3.enabled,
      recall_top_k: settings.l3_recall_top_k ?? config.memoryL3.recall_top_k,
      recall_min_score: settings.l3_recall_min_score ?? config.memoryL3.recall_min_score,
      recall_scopes: parseJSONList(settings.l3_recall_scopes_json || JSON.stringify(config.memoryL3.recall_scopes)),
      embedding_model: settings.l3_embedding_model || config.memoryL3.embedding_model,
      decay_interval_hours: settings.l3_decay_interval_hours ?? config.memoryL3.decay_interval_hours,
      archive_threshold: settings.l3_archive_threshold ?? config.memoryL3.archive_threshold,
      max_per_recall_chars: settings.l3_max_per_recall_chars ?? config.memoryL3.max_per_recall_chars,
    },
    memoryL4: {
      enabled: settings.l4_enabled ?? config.memoryL4.enabled,
      graph_inject_neighbors: settings.l4_graph_inject_neighbors ?? config.memoryL4.graph_inject_neighbors,
      graph_max_neighbors: settings.l4_graph_max_neighbors ?? config.memoryL4.graph_max_neighbors,
      graph_max_hops: settings.l4_graph_max_hops ?? config.memoryL4.graph_max_hops,
      identity_inject: settings.l4_identity_inject ?? config.memoryL4.identity_inject,
      strategy_inject: settings.l4_strategy_inject ?? config.memoryL4.strategy_inject,
    },
    evolutionSettings: {
      enabled: settings.evo_enabled ?? config.evolutionSettings.enabled,
      auto_apply: settings.evo_auto_apply ?? config.evolutionSettings.auto_apply,
      min_episodes: settings.evo_min_episodes ?? config.evolutionSettings.min_episodes,
      min_negative_feedback: settings.evo_min_negative_feedback ?? config.evolutionSettings.min_negative_feedback,
      throttle_hours: settings.evo_throttle_hours ?? config.evolutionSettings.throttle_hours,
      proposal_ttl_days: settings.evo_proposal_ttl_days ?? config.evolutionSettings.proposal_ttl_days,
      persona_max_chars: settings.evo_persona_max_chars ?? config.evolutionSettings.persona_max_chars,
      system_prompt_max_appends: settings.evo_system_prompt_max_appends ?? config.evolutionSettings.system_prompt_max_appends,
    },
    heartbeat: {
      enabled: settings.heartbeat_enabled,
      interval_minutes: settings.heartbeat_interval_minutes,
    },
    evolution: {
      self_evolve: settings.evolution_self_evolve,
      skill_evolve: settings.evolution_skill_evolve,
      evolution_metrics_enabled: settings.evolution_metrics_enabled,
      evolution_suggestions_enabled: settings.evolution_suggestions_enabled,
    },
    evolution_guardrails: {
      max_change_per_period: settings.guardrail_max_change_per_period,
      min_data_points: settings.guardrail_min_data_points,
      rollback_on_decline_percent: settings.guardrail_rollback_on_decline_percent,
    },
    skillRuntime: parseSkillRuntimeForm(settings.skill_runtime_json),
    code_executor_type: settings.code_executor_type || "local",
    intent_pass: {
      enabled: settings.intent_pass_enabled !== false,
    },
  });
  hydrateAdvancedFromSettings(advanced, settings);
}

export function hydrateAgentRuntime(
  config: AgentRuntimeConfigForm,
  advanced: AgentAdvancedSettingsForm,
  agent: Agent,
  files?: AgentFileLike[],
) {
  if (agent.settings) {
    hydrateRuntimeFromSettings(config, advanced, agent.settings);
    return "settings" as const;
  }
  hydrateRuntimeFromConfigJson(config, agent.config_json, files);
  return "config_json" as const;
}

export function applyAdvancedSaveToRuntime(
  config: AgentRuntimeConfigForm,
  advanced: AgentAdvancedSettingsForm,
  payload: AgentAdvancedSettingsForm,
) {
  Object.assign(advanced, payload);
  config.memoryL0.recent_window_turns = payload.recent_window_turns;
  config.memoryL0.recent_window_tokens = payload.recent_window_tokens;
  config.memoryL0.summary_keep_turns = payload.summary_keep_turns;
  config.memoryL0.truncate_strategy = payload.truncate_strategy;
}

/** Reset config/advanced to defaults (e.g. new agent). */
export function resetAgentRuntimeForm(
  config: AgentRuntimeConfigForm,
  advanced: AgentAdvancedSettingsForm,
) {
  Object.assign(config, defaultAgentRuntimeConfig());
  Object.assign(advanced, defaultAgentAdvancedSettings());
}
