export type SkillTag = {
  name: string;
  source: "system" | "user" | string;
};

export type SkillVersionSummary = {
  id: string;
  version: string;
  validation_status: "pass" | "warn" | "block" | string;
  published_at: string;
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
  status: "draft" | "published" | "archived" | string;
  enabled: boolean;
  current_version: SkillVersionSummary | null;
  invoke_count: number;
  success_count: number;
  failure_count: number;
  usage_count_7d?: number;
  avg_duration_ms: number | null;
  last_agent_id?: string;
  last_agent_display_name?: string;
  last_invoked_at?: string;
  last_duration_ms: number | null;
  created_at: string;
  updated_at: string;
  permissions: SkillPermissions;
  filesystem_missing?: boolean;
  sync_origin?: "filesystem" | "import" | "manual" | string;
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
  status: "success" | "failure" | "pending" | string;
  duration_ms: number;
  started_at: string;
  ended_at?: string;
  input_preview?: string;
  input_hash?: string;
  output_preview?: string;
  error_code?: string;
  error_message?: string;
  permissions: SkillInvocationPermissions;
};

export type SkillRunQuery = {
  skill_id?: string;
  agent_id?: string;
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
  conflict_risk: "low" | "medium" | "high" | string;
  recommendation: "keep_separate" | "suggest_refine" | "block_duplicate" | string;
  confidence: number;
};

export type SkillImportCandidate = {
  candidate_id: string;
  name: string;
  slug: string;
  description: string;
  body_preview: string;
  target_dir: string;
  validation_status: "pass" | "warn" | "block" | string;
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
  status: "processing" | "completed" | "failed" | string;
  validation_status: "pass" | "warn" | "block" | string;
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
  action: "import_passed" | "skip_group" | "merge_group_with_ai" | "approve_risky_import" | "reject_risky_upload";
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

export type PaginatedResponse<T> = {
  items: T[];
  page: number;
  page_size: number;
  total: number;
};
