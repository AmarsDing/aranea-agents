/** 记忆中心 UI / `memory/v1` 映射后的 snake_case 形状（与网关 JSON 对齐）。 */

export type L0AssemblySegmentStats = {
  token_estimate: number;
  message_count: number;
  field_count?: number;
  fact_count?: number;
  entity_count?: number;
  result_count?: number;
  turn_count?: number;
  from_turn?: number;
  to_turn?: number;
};

export type L0AssemblySegmentsMap = Record<string, L0AssemblySegmentStats>;

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
  pii_types: string[];
  quality_score: number;
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
  valid_from?: string;
  valid_to?: string;
};

export type GraphNeighborhood = {
  center: MemoryEntity;
  hops: number;
  entities: MemoryEntity[];
  relations: MemoryRelation[];
  query_at?: string;
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

export type CascadeAffectedEntity = {
  entity_id: string;
  entity_name: string;
  entity_type: string;
  relation_type: string;
  hops: number;
};

export type CascadeProposal = {
  id: string;
  agent_id: string;
  trigger_entity_id: string;
  trigger_entity_name: string;
  trigger_attribute: string;
  old_value: string;
  new_value: string;
  affected_entities?: CascadeAffectedEntity[];
  status: string;
  risk_level: string;
  rationale: string;
  workspace_id?: string;
  reviewed_by?: string;
  reviewed_at?: string;
  expires_at?: string;
  created_at: string;
  updated_at?: string;
};

export type CascadeFactDiff = {
  fact_id: string;
  before_statement: string;
  after_statement: string;
  scope: string;
};

export type CascadeEntityRename = {
  entity_id: string;
  entity_type: string;
  old_name: string;
  new_name: string;
};

export type CascadePreview = {
  affected_entities_count: number;
  affected_facts_count: number;
  fact_diffs: CascadeFactDiff[];
  entity_renames: CascadeEntityRename[];
};

export type CascadeSagaStep = {
  id: string;
  proposal_id: string;
  step_index: number;
  step_name: string;
  state: string;
  is_critical: boolean;
  attempts: number;
  started_at: string;
  finished_at: string;
  payload_json: string;
  result_json: string;
  error: string;
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

export type MemoryRecallScoreBreakdown = {
  keyword: number;
  vector: number;
  importance: number;
  recency: number;
  cross_encoder: number;
  quality_score: number;
  total: number;
};

export type MemoryRecallHit = {
  layer: string;
  id: string;
  title?: string;
  summary?: string;
  statement?: string;
  scores: MemoryRecallScoreBreakdown;
};

export type CompositeSearchHit = {
  layer: string;
  id: string;
  text: string;
  score: number;
};

export type MemoryWorkerStatus = {
  jobs_done: number;
  jobs_dead: number;
  llm_fallback_total: number;
  avg_extraction_seconds: number;
  episode_backfill_total: number;
  queue_high?: MemoryWorkerQueueStats;
  queue_normal?: MemoryWorkerQueueStats;
  queue_low?: MemoryWorkerQueueStats;
  fact_index_stale_count?: number;
  fact_index_disabled_count?: number;
  db_available?: boolean;
  dead_letter_pending?: number;
  oldest_pending_age_ms?: number;
};

export type MemoryWorkerQueueStats = {
  capacity: number;
  in_flight: number;
  dropped_total?: number;
  debounced_total?: number;
};

export type MemoryDeadLetterEntry = {
  id: number;
  session_id: string;
  app_name: string;
  drop_reason: string;
  priority: number;
  attempts: number;
  state: string;
  last_error: string;
  enqueued_at: string;
  failed_at: string;
};

export type MemoryPlatformSettings = {
  policy_strict: boolean;
  episode_backfill_disabled: boolean;
  env_policy_strict_override: boolean;
  env_episode_backfill_disabled_override: boolean;
};
