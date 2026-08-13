export type SkillTag = {
  name: string;
  source: 'system' | 'user' | string;
};

/** 字典标签 + 实时使用计数（source=orphan 表示使用中但未收录进字典）。 */
export type SkillTagInfo = {
  name: string;
  /** `:` 前缀维度（file_type/domain），无维度为空串。 */
  dimension: string;
  source: 'system' | 'user' | 'orphan' | string;
  used_count: number;
  created_at: string;
  updated_at: string;
};

export type SkillVersionSummary = {
  id: string;
  version: string;
  validation_status: 'pass' | 'warn' | 'block' | string;
  published_at: string;
};

export type SkillVersionDetail = {
  id: string;
  skill_id: string;
  version: string;
  status: string;
  content_markdown: string;
  validation_status: string;
  published_at: string;
  created_at: string;
  file_manifest_json?: string;
};

export type SkillPermissions = {
  can_edit: boolean;
  can_delete: boolean;
  can_toggle_enabled: boolean;
  can_duplicate: boolean;
};

export type Skill = {
  id: string;
  name: string;
  slug: string;
  description: string;
  tags: SkillTag[];
  extends_skill_id?: string;
  status: 'draft' | 'published' | 'archived' | string;
  enabled: boolean;
  current_version: SkillVersionSummary | null;
  invoke_count: number;
  success_count: number;
  failure_count: number;
  usage_count_7d: number;
  avg_duration_ms: number | null;
  last_agent_id?: string;
  last_agent_display_name?: string;
  last_invoked_at?: string;
  last_duration_ms: number | null;
  created_at: string;
  updated_at: string;
  permissions: SkillPermissions;
  filesystem_missing?: boolean;
  sync_origin?: 'filesystem' | 'import' | 'manual' | string;
  visibility?: string;
  default_config_json?: string;
};

export type SkillFilesystemHealth = {
  root_accessible: boolean;
  resolved_root: string;
  missing_count: number;
  pending_filesystem_count: number;
};

export type SkillListQuery = {
  search?: string;
  tags?: string[];
  enabled?: boolean | null;
  status?: string;
  filesystem_missing?: boolean | null;
  sync_origin?: string;
  /** 排序字段：tag（按首个标签名）| name（按名称）；空 = 默认按更新时间倒序。 */
  sort_by?: 'tag' | 'name' | '';
  /** 排序方向：asc | desc；空 = asc。 */
  sort_order?: 'asc' | 'desc' | '';
  page?: number;
  page_size?: number;
};

export type SkillInvocationPermissions = {
  can_view_detail: boolean;
};

export type SkillInvocation = {
  id: string;
  skill_id: string;
  skill_name: string;
  skill_version: string;
  agent_id: string;
  agent_display_name: string;
  user_id?: string;
  session_id?: string;
  status: 'success' | 'failure' | 'pending' | string;
  duration_ms: number;
  started_at: string;
  ended_at?: string;
  input_preview?: string;
  input_hash?: string;
  output_preview?: string;
  error_code?: string;
  error_message?: string;
  permissions: SkillInvocationPermissions;
  source?: string;
  activation_id?: string;
  message_id?: string;
};

export type SkillRunQuery = {
  skill_id?: string;
  agent_id?: string;
  session_id?: string;
  status?: string;
  from?: string;
  to?: string;
  page?: number;
  page_size?: number;
};

export type SkillImportIssue = {
  type: string;
  message: string;
};

export type SkillSimilarityMetrics = {
  similarity_score: number;
  name_similarity: number;
  description_similarity: number;
  body_similarity: number;
  trigger_similarity: number;
  tool_similarity: number;
  conflict_risk: 'low' | 'medium' | 'high' | string;
  recommendation: 'keep_separate' | 'suggest_refine' | 'block_duplicate' | string;
  confidence: number;
};

export type SkillImportCandidate = {
  candidate_id: string;
  name: string;
  slug: string;
  description: string;
  body_preview: string;
  target_dir: string;
  validation_status: 'pass' | 'warn' | 'block' | string;
  status_icon: string;
  warnings: SkillImportIssue[];
  blocks: SkillImportIssue[];
};

export type SkillSimilaritySource = {
  id: string;
  name: string;
  slug: string;
  description: string;
  version: string;
  body_preview: string;
};

export type SkillConflictGroup = {
  group_id: string;
  highest_similarity_score: number;
  metrics: SkillSimilarityMetrics;
  reason: string;
  evidence: string[];
  candidate_ids: string[];
  existing_skills: SkillSimilaritySource[];
  can_refine: boolean;
};

export type SkillImportJob = {
  job_id: string;
  status: 'processing' | 'completed' | 'failed' | string;
  validation_status: 'pass' | 'warn' | 'block' | string;
  storage_root: string;
  candidates: SkillImportCandidate[];
  conflict_groups: SkillConflictGroup[];
  message?: string;
};

export type SkillRefineResult = {
  merged_name: string;
  merged_description: string;
  merged_body: string;
  merged_tags: SkillTag[];
  source_candidate_ids: string[];
  source_existing_skill_ids: string[];
};

export type SkillImportDecision = {
  candidate_id?: string;
  group_id?: string;
  action:
    | 'import_passed'
    | 'skip_group'
    | 'merge_group_with_ai'
    | 'approve_risky_import'
    | 'reject_risky_upload'
    | 'keep_separate';
  merged_name?: string;
  merged_description?: string;
  merged_body?: string;
  merged_tags?: SkillTag[];
};

export type SkillImportApplyResult = {
  created_skill_ids: string[];
  skipped_candidate_ids: string[];
  message: string;
};

export type SkillFile = {
  path: string;
  name: string;
  language: string;
  size: number;
  updated_at: string;
};

export type SkillFileContent = {
  path: string;
  content: string;
  language: string;
};

export type SkillHealthDailyMetric = {
  date: string;
  invocations: number;
  successes: number;
  avg_duration_ms: number;
  routed_count: number;
  loaded_count: number;
};

export type SkillHealthMetric = {
  skill_id: string;
  total_invocations_7d: number;
  success_count_7d: number;
  success_rate_7d: number;
  p95_duration_ms_7d: number;
  total_invocations_30d: number;
  success_count_30d: number;
  success_rate_30d: number;
  p95_duration_ms_30d: number;
  route_hit_rate_7d: number;
  route_hit_rate_30d: number;
  /** 路由命中率分子/分母（去重轮次）；routed_count=0 表示无路由数据（区别于 0% 命中率）。 */
  routed_count_7d: number;
  loaded_count_7d: number;
  routed_count_30d: number;
  loaded_count_30d: number;
  daily_metrics: SkillHealthDailyMetric[];
};

export type PaginatedResponse<T> = {
  items: T[];
  page: number;
  page_size: number;
  total: number;
};

export type FailureTagCountView = {
  tag: string;
  count: number;
};

export type ExperienceReportListResult = {
  items: ExperienceReportView[];
  total: number;
  page: number;
  page_size: number;
  failureTagCounts: FailureTagCountView[];
  rootCauseReports: ExperienceReportView[];
};

// ── Experience Report (Skill Intelligence) ──────────────────────

export type ExperienceReportView = {
  id: string;
  tenantId: string;
  sessionId: string;
  invocationId: string;
  skillId: string;
  skillName: string;
  isSuccess: boolean;
  score: number;
  failureTags: string[];
  flowSummary: string;
  rootCauseAnalysis: string;
  suggestedFix: string;
  optimizationAdvice: string;
  selectionSnapshot: Record<string, unknown>;
  generatedSuggestionId: string;
  createdAt: string;
};

// ── Evolution Suggestion ────────────────────────────────────────

export type EvolutionSuggestionView = {
  id: string;
  skillId: string;
  type: string;
  status: string;
  triggerReason: string;
  sourceReportIds: string[];
  draftSkillBody: string;
  sandboxPassed: boolean | null; // null = not yet validated
  sandboxResult: Record<string, unknown>;
  preVerifyResult: Record<string, unknown>;
  parentVersionId: string;
  draftVersionId: string;
  evolutionReason: string;
  lifecycleStatus: string;
  approvedBy: string;
  rejectedBy: string;
  rejectionReason: string;
  resolvedAt: string;
  createdAt: string;
};

// ── Skill Catalog (Chat Integration) ────────────────────────────

export type SkillCatalogEntry = {
  slug: string;
  name: string;
  description: string;
  tags: string[];
};

export type SkillHint = {
  matched_skill: string;
  trigger: string;
  confidence: number;
};

// ── Unified Evolution Types ──

export type EvolutionTargetType = 'skill' | 'agent';
export type EvolutionActionType = 'create_skill' | 'improve_skill' | 'merge_skill' | 'evolve_agent';

export type SkillEvolutionView = {
  id: string;
  targetType: EvolutionTargetType;
  targetId: string;
  targetName: string;
  actionType: EvolutionActionType;
  triggerSource: string;
  triggerReason: string;
  status: 'pending' | 'approved' | 'rejected' | 'applied' | 'expired';
  priority: number;

  draftBody: string;
  draftName: string;
  mergeTargetId: string;

  lifecycleStatus: 'draft' | 'validating' | 'ready' | 'applied';
  sandboxPassed: boolean | null; // null = 无沙箱数据（如 agent 级提案），UI 显示 "—" 而非误报"未通过"
  sandboxResult: Record<string, unknown> | null;
  metadata: Record<string, unknown> | null;

  createdAt: string;
  approvedBy: string;
  appliedAt: string | null;
};

/** @deprecated Use SkillEvolutionView instead. Kept for backward compatibility. */
export type UnifiedEvolutionSuggestion = SkillEvolutionView;
