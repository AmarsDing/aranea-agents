export type Session = {
  id: string;
  owner_type: string;
  agent_id: string;
  team_id: string;
  title: string;
  summary: string;
  context_used_ratio: number;
  max_context_used_ratio: number;
  context_status: string;
  dialog_mode: string;
  provider: string;
  model: string;
  status: string;
  message_count: number;
  run_count: number;
  model_call_count: number;
  tool_call_count: number;
  skill_call_count: number;
  mcp_call_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  last_message_at: string;
  created_at: string;
  updated_at: string;
  archived_at: string;
  deleted_at: string;
  metadata_json?: string;
  context_used_tokens?: number;
  last_context_window_tokens?: number;
};

export type SessionSearchQuery = {
  owner_type?: string;
  agent_id?: string;
  team_id?: string;
  status?: string;
  context_status?: string;
  keyword?: string;
  limit?: number;
  offset?: number;
  page?: number;
  page_size?: number;
};

export type SessionListResult = {
  items: Session[];
  total: number;
  limit: number;
  offset: number;
};

export type SessionTimelineItem = {
  id: string;
  kind: "message" | "tool" | "skill" | "mcp" | string;
  side: "left" | "right" | string;
  title: string;
  subtitle: string;
  actor_id: string;
  actor_name: string;
  status: string;
  occurred_at: string;
  duration_ms: number;
  content_markdown: string;
  preview: string;
  detail_json: string;
  tags: string[];
};

export type SessionTimelineSummary = {
  total: number;
  message_count: number;
  tool_count: number;
  skill_count: number;
  mcp_count: number;
};

export type SessionTimeline = {
  session_id: string;
  items: SessionTimelineItem[];
  summary: SessionTimelineSummary;
};

export type SessionTurn = {
  id: string;
  session_id: string;
  run_id: string;
  turn_index: number;
  user_message_id: string;
  assistant_message_id: string;
  owner_type: string;
  agent_id: string;
  team_id: string;
  status: string;
  started_at: string;
  ended_at: string;
  duration_ms: number;
  first_token_ms: number;
  model_call_count: number;
  tool_call_count: number;
  skill_call_count: number;
  mcp_call_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  final_provider: string;
  final_model: string;
  final_content_preview: string;
  error_code: string;
  error_message: string;
  metadata_json: string;
  created_at: string;
  updated_at: string;
};

export type SessionBatchScope = {
  owner_type?: string;
  agent_id?: string;
  team_id?: string;
  status?: string;
  context_status?: string;
  keyword?: string;
};

export type BatchPreviewResult = {
  matched: number;
  skipped_running: number;
  skipped_not_found: number;
  truncated: boolean;
  sample_ids: string[];
};

export type BatchOperationResult = {
  matched: number;
  processed: number;
  skipped_running: number;
  skipped_not_found: number;
  truncated: boolean;
  failed_ids: string[];
};

export type BulkProgress = {
  active: boolean;
  label: string;
  indeterminate?: boolean;
};

export type RetentionDialogMode = "archive" | "delete";
