/** Agent runtime form defaults and shared helpers (settings ↔ config_json). */

import type { EvolutionKey } from "../../components/agents/agentUi";

export type AgentRuntimeConfigForm = ReturnType<typeof defaultAgentRuntimeConfig>;

export type AgentAdvancedSettingsForm = ReturnType<typeof defaultAgentAdvancedSettings>;

export function defaultAgentAdvancedSettings() {
  return {
    channel_id: "",
    chat_id: "",
    workspace: "",
    reasoning_mode: "provider_default",
    reasoning_level: "off",
    context_compaction_enabled: false,
    session_summary_enabled: false,
    truncate_strategy: "sliding",
    recent_window_turns: 20,
    recent_window_tokens: 0,
    summary_keep_turns: 4,
  };
}

export function defaultAgentRuntimeConfig() {
  return {
    self_evolve: true,
    subagents: {
      enabled: true,
      max_concurrency: 20,
      max_generation_depth: 1,
      max_children_per_agent: 5,
      archive_after_minutes: 60,
      max_retries: 2,
      model_override: "",
    },
    tools: {
      enabled: true,
      profile: "chat_only",
      tool_call_prefix: "",
      allow: [] as string[],
      deny: [] as string[],
      concurrent_allow: [] as string[],
      retry: {
        enabled: false,
        max_attempts: 2,
        initial_interval_ms: 500,
        backoff_factor: 2.0,
        max_interval_ms: 5000,
        jitter: true,
      },
      parallel_enabled: false,
      streaming_enabled: false,
    },
    memory: {
      enabled: true,
      max_chunk_length: 1000,
      max_results: 6,
      min_score: 0.35,
    },
    memoryL0: {
      recent_window_turns: 12,
      recent_window_tokens: 0,
      summary_threshold: 0.6,
      summary_keep_turns: 4,
      truncate_strategy: "summary",
      inject_l1: true,
      inject_l3: true,
      inject_l4: false,
      l3_max_chunks: 5,
      l4_max_paths: 3,
      snapshot_mode: "on_warning",
    },
    memoryL1: {
      enabled: true,
      budget_tokens: 8192,
      field_max_tokens: 2048,
      history_keep_revisions: 10,
      default_schema_id: "",
      archive_on_idle_minutes: 60,
    },
    memoryL2: {
      episode_enabled: true,
      episode_min_importance: 0.3,
      index_enabled: true,
      index_embedding_model: "",
      recall_enabled: false,
      recall_max: 3,
      retention_days: 90,
      archive_after_days: 30,
    },
    memoryL3: {
      enabled: true,
      recall_top_k: 5,
      recall_min_score: 0.55,
      recall_scopes: ["agent", "user", "team", "workspace"] as string[],
      embedding_model: "",
      decay_interval_hours: 24,
      archive_threshold: 0.2,
      max_per_recall_chars: 1500,
    },
    memoryL4: {
      enabled: true,
      graph_inject_neighbors: true,
      graph_max_neighbors: 6,
      graph_max_hops: 2,
      identity_inject: true,
      strategy_inject: false,
    },
    evolutionSettings: {
      enabled: false,
      auto_apply: false,
      min_episodes: 20,
      min_negative_feedback: 3,
      throttle_hours: 24,
      proposal_ttl_days: 14,
      persona_max_chars: 1500,
      system_prompt_max_appends: 5,
    },
    heartbeat: {
      enabled: false,
      interval_minutes: 30,
    },
    evolution: {
      self_evolve: true,
      skill_evolve: true,
      evolution_metrics_enabled: true,
      evolution_suggestions_enabled: true,
    } as Record<EvolutionKey, boolean>,
    evolution_guardrails: {
      max_change_per_period: 0.1,
      min_data_points: 100,
      rollback_on_decline_percent: 20,
    },
    skillRuntime: {
      intent_routing_enabled: true,
      intent_max_paths: 3,
      max_skills_in_toolset: 32,
      allowed_slugs: [] as string[],
      denied_slugs: [] as string[],
      allowed_tags: [] as string[],
    },
    code_executor_type: "local",
    intent_pass: {
      enabled: true,
    },
  };
}

export function parseJSONList(raw: string) {
  try {
    const parsed = JSON.parse(raw || "[]");
    return Array.isArray(parsed) ? parsed.map(String) : [];
  } catch {
    return [];
  }
}

export const truncateStrategyOptions = ["summary", "drop_oldest", "drop_tool_results", "hybrid"].map((value) => ({
  label: value,
  value,
}));

export const snapshotModeOptions = ["always", "on_warning", "off"].map((value) => ({ label: value, value }));

export const memoryScopeOptions = ["agent", "user", "team", "workspace", "global"].map((value) => ({
  label: value,
  value,
}));

export const toolProfileOptions = [
  { label: "chat_only · 仅对话（无工具）", value: "chat_only" },
  { label: "read_only · 只读 + 时间", value: "read_only" },
  { label: "coding · 文件读写 + 网页 + 技能", value: "coding" },
  { label: "research · 网页 + 检索 + 技能", value: "research" },
  { label: "full · 全工具（高权限，慎用）", value: "full" },
];
