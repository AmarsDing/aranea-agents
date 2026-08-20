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
  // FR-12.6 three-stage counters: recalled into result set / injected into
  // prompt / cited by the assistant reply. use_count above is legacy.
  recalled_count: number;
  injected_count: number;
  cited_count: number;
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
  // Filter by ORIGINATING agent across all scopes — keeps the L3 browse tab
  // consistent with the panorama card count (agent_id aggregation).
  agent_id?: string;
  limit?: number;
  offset?: number;
};

export type MemoryFactListResult = {
  items: MemoryFact[];
  total: number;
  limit: number;
  offset: number;
  /** 全量筛选集（忽略 status）下的活跃/归档计数，用于知识面板统计行。 */
  active_count?: number;
  archived_count?: number;
  /** 与返回 items 同口径（含 status 过滤）的总数，用于服务端分页。 */
  filtered_count?: number;
};

/** ReviewMemoryFact 治理动作（memory.md §9.4）。refine 需携带替换字段。 */
export type FactReviewAction = 'confirm' | 'reject' | 'archive' | 'dispute' | 'deprecate' | 'refine';

export type FactReviewPayload = {
  fact_id: string;
  action: FactReviewAction;
  statement?: string;
  details_markdown?: string;
  fact_kind?: string;
  tags_json?: string;
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

export type ActivationPathStep = {
  from_node_id: string;
  to_node_id: string;
  edge_weight: number;
  relation_type: string;
};

export type SpreadingActivationResult = {
  node_id: string;
  activation: number;
  hop_count: number;
  activation_path: ActivationPathStep[];
};

export type SpreadingActivationResponse = {
  center_id: string;
  hops: number;
  top_k: number;
  items: SpreadingActivationResult[];
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

// --- 记忆中心重设计：层级全景 + 跨层关联图谱 ---

/** 单层记忆统计卡片（L0~L4）。 */
export type MemoryLayerStat = {
  layer: string;
  item_count: number;
  today_added: number;
  recall_hits: number;
  health: string;
  /** JSON 字符串：各层头条指标（如 context_usage_pct / compress_status）。 */
  headline_json: string;
};

/** 待办行动项（如冲突事实、待审 PII）。 */
export type MemoryActionItem = {
  kind: string;
  count: number;
  target_tab: string;
};

/** 最近记忆动态事件（沉淀⬇ / 召回⬆）。 */
export type MemoryActivityItem = {
  ts: string;
  kind: string;
  layer_from: string;
  layer_to: string;
  summary: string;
};

/** 层级全景响应。 */
export type MemoryLayerOverview = {
  layers: MemoryLayerStat[];
  action_items: MemoryActionItem[];
  activity_feed: MemoryActivityItem[];
};

/** 跨层统一图谱节点（L4 实体 / L3 事实 / L2 情景）。 */
export type UnifiedGraphNode = {
  id: string;
  layer: string;
  kind: string;
  label: string;
  weight: number;
  /** JSON 字符串：节点附加元信息。 */
  meta_json: string;
};

/** 跨层统一图谱边。 */
export type UnifiedGraphEdge = {
  source: string;
  target: string;
  /** entity_relation / entity_fact / fact_link / fact_source / fact_conflict */
  type: string;
  label: string;
  weight: number;
  polarity: string;
};

/** 跨层关联图谱查询参数。 */
export type UnifiedMemoryGraphQuery = {
  focus?: string;
  hops?: number;
  min_weight?: number;
  layers?: string[];
};

/** 跨层关联图谱响应。 */
export type UnifiedMemoryGraph = {
  focus: string;
  nodes: UnifiedGraphNode[];
  edges: UnifiedGraphEdge[];
  node_count: number;
  edge_count: number;
  filtered_edge_count: number;
  empty_reason: string;
};

/** L2 情景记忆条目（浏览 Tab 时间线卡片）。 */
export type MemoryEpisode = {
  id: string;
  session_id: string;
  agent_id: string;
  /** 情景类型（如 task / chat）。 */
  episode_kind: string;
  title: string;
  outcome_summary: string;
  importance: number;
  /** pending | consolidated */
  consolidation_status: string;
  consolidated_l3_count: number;
  ended_at: string;
  created_at: string;
};

/** L2 情景分页结果。 */
export type MemoryEpisodeListResult = {
  items: MemoryEpisode[];
  total: number;
};
