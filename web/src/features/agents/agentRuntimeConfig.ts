/** Agent runtime form defaults and shared helpers (settings ↔ config_json). */

import type { EvolutionKey } from '../../components/agents/agentUi';

export type AgentRuntimeConfigForm = ReturnType<typeof defaultAgentRuntimeConfig>;

export type AgentAdvancedSettingsForm = ReturnType<typeof defaultAgentAdvancedSettings>;

export function defaultAgentAdvancedSettings() {
  return {
    channel_id: '',
    chat_id: '',
    workspace: '',
    reasoning_mode: 'provider_default',
    reasoning_level: 'off',
    context_compaction_enabled: false,
    session_summary_enabled: false,
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
      stored_result_runes: 4000,
      stored_summary_runes: 240,
      model_override: '',
    },
    tools: {
      enabled: true,
      profile: 'coding',
      tool_call_prefix: '',
      allow: [] as string[],
      deny: [] as string[],
      concurrent_allow: [] as string[],
      retry: {
        enabled: true,
        max_attempts: 2,
        initial_interval_ms: 500,
        backoff_factor: 2.0,
        max_interval_ms: 5000,
        jitter: true,
      },
      parallel_enabled: true,
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
      compress_provider: '',
      compress_model: '',
      truncate_strategy: 'summary',
      inject_l1: true,
      inject_l3: true,
      inject_l4: true,
      l3_max_chunks: 5,
      l4_max_paths: 3,
      snapshot_mode: 'on_warning',
    },
    memoryL1: {
      enabled: true,
      budget_tokens: 8192,
      field_max_tokens: 2048,
      history_keep_revisions: 10,
      default_schema_id: '',
      archive_on_idle_minutes: 60,
    },
    memoryL2: {
      episode_enabled: true,
      episode_min_importance: 0.3,
      index_enabled: true,
      index_embedding_model: '',
      recall_enabled: false,
      recall_max: 3,
      retention_days: 90,
      archive_after_days: 30,
    },
    memoryL3: {
      enabled: true,
      recall_top_k: 5,
      recall_min_score: 0.35,
      recall_scopes: ['agent', 'user', 'team', 'workspace'] as string[],
      embedding_model: '',
      decay_interval_hours: 24,
      archive_threshold: 0.2,
      max_per_recall_chars: 1500,
      pii_policy: 'redact',
    },
    memoryWorker: {
      provider: '',
      model: '',
    },
    memoryL4: {
      enabled: true,
      graph_inject_neighbors: true,
      graph_max_neighbors: 6,
      graph_max_hops: 2,
      identity_inject: true,
      strategy_inject: false,
      decay_interval_hours: 168,
      decay_overrides_json: '',
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
    /** After-Turn 自动评估（US-5）：写入 config_json.evaluation，由后端 AfterTurnTrigger 消费。 */
    evaluation: {
      auto_after_turn: false,
      dataset_id: '',
      metrics: '',
      num_runs: 1,
      min_interval_sec: 300,
    },
    knowledge: {
      grounded_only: false,
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
    skill_load_mode: 'progressive',
    code_executor_type: 'local',
    intent_pass: {
      enabled: false,
    },
    spirit: {
      max_concurrent_teams: 3,
      max_team_concurrency: 2,
      team_timeout_seconds: 600,
      auto_archive_seconds: 3600,
      max_session_depth: 2,
      verification_truncate_chars: 2000,
      timeout_handler_db_timeout_seconds: 30,
    },
  };
}

export function parseJSONList(raw: string) {
  try {
    const parsed = JSON.parse(raw || '[]');
    return Array.isArray(parsed) ? parsed.map(String) : [];
  } catch {
    return [];
  }
}

const truncateStrategyLabels: Record<string, string> = {
  summary: '摘要优先',
  drop_oldest: '丢弃最旧',
  drop_tool_results: '丢弃工具结果',
  hybrid: '混合',
};

export const truncateStrategyOptions = ['summary', 'drop_oldest', 'drop_tool_results', 'hybrid'].map((value) => ({
  label: truncateStrategyLabels[value] ?? value,
  value,
}));

const snapshotModeLabels: Record<string, string> = {
  always: '始终',
  on_warning: '警告时',
  off: '关闭',
};

export const snapshotModeOptions = ['always', 'on_warning', 'off'].map((value) => ({
  label: snapshotModeLabels[value] ?? value,
  value,
}));

const memoryScopeLabels: Record<string, string> = {
  agent: 'Agent',
  user: '用户',
  team: '团队',
  workspace: '工作区',
  global: '全局',
};

export const memoryScopeOptions = ['agent', 'user', 'team', 'workspace', 'global'].map((value) => ({
  label: memoryScopeLabels[value] ?? value,
  value,
}));

export const piiPolicyOptions = [
  { label: 'redact · 脱敏写入（默认）', value: 'redact' },
  { label: 'block · 阻断写入', value: 'block' },
  { label: 'review · 人工审核', value: 'review' },
];

// 与后端 toolProfiles（internal/biz/agent_effective_tools.go）9 个 profile 对齐；
// minimal/safe 为 chat_only/read_only 的兼容别名，system_admin/spirit 为系统内部 profile。
export const toolProfileOptions = [
  { label: 'chat_only · 仅对话（无工具）', value: 'chat_only' },
  { label: 'read_only · 只读 + 时间', value: 'read_only' },
  { label: 'coding · 文件读写 + 网页 + 技能', value: 'coding' },
  { label: 'research · 网页 + 检索 + 技能', value: 'research' },
  { label: 'full · 全工具（高权限，慎用）', value: 'full' },
  { label: 'minimal · 同 chat_only（兼容别名）', value: 'minimal' },
  { label: 'safe · 同 read_only（兼容别名）', value: 'safe' },
  { label: 'system_admin · CLI 管理工具组（系统内部）', value: 'system_admin' },
  { label: 'spirit · 编排工具（系统内部）', value: 'spirit' },
];

/** 后端 expandToolGroup 支持的 12 个工具组（allow/deny/并行白名单可用 group:<name>）。 */
export const toolGroupOptions = [
  { label: 'group:filesystem · 文件系统', value: 'group:filesystem' },
  { label: 'group:web · 网页检索/抓取', value: 'group:web' },
  { label: 'group:memory · 记忆', value: 'group:memory' },
  { label: 'group:skill · 技能', value: 'group:skill' },
  { label: 'group:media · 媒体处理', value: 'group:media' },
  { label: 'group:runtime · 运行时/Shell', value: 'group:runtime' },
  { label: 'group:messaging · 消息通知', value: 'group:messaging' },
  { label: 'group:session · 会话管理', value: 'group:session' },
  { label: 'group:integration · 外部集成', value: 'group:integration' },
  { label: 'group:subagent · 子代理', value: 'group:subagent' },
  { label: 'group:browser · 浏览器', value: 'group:browser' },
  { label: 'group:cli_admin · CLI 管理（系统）', value: 'group:cli_admin' },
];
