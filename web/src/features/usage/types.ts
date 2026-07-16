/**
 * 模型用量 UI / `usage/v1` 映射后的 snake_case 形状（与网关 JSON `*_micro_*` 命名对齐）。
 * 勿再经由单一巨型遗留 facade —— 已迁入各 `features/<domain>/api`。
 */

export type ModelUsageQuery = {
  range?: string;
  start_date?: string;
  end_date?: string;
  provider_code?: string;
  model_api_id?: string;
  agent_id?: string;
  team_id?: string;
  usage_kind?: string;
  status?: string;
  limit?: number;
  offset?: number;
  /** "" | "day" | "hour" — hour uses model_token_usage_hourly */
  granularity?: string;
};

export type ModelUsageEventsResult = {
  items: ModelTokenUsageEvent[];
  total: number;
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
  /** RFC3339; ingest defaults filled server-side when omitted */
  date_key?: string;
  hour_key?: string;
  workspace_id?: string;
  user_id?: string;
  team_id?: string;
  agent_id: string;
  agent_key: string;
  session_id: string;
  message_id: string;
  request_id?: string;
  provider_code: string;
  provider_type: string;
  provider_display_name: string;
  model_api_id: string;
  model_display_name: string;
  model_category_json?: string;
  usage_kind?: string;
  call_count: number;
  input_tokens: number;
  output_tokens: number;
  cached_input_tokens?: number;
  reasoning_tokens?: number;
  embedding_tokens?: number;
  total_tokens: number;
  input_price_micro_usd_per_1k?: number;
  output_price_micro_usd_per_1k?: number;
  cached_input_price_micro_usd_per_1k?: number;
  reasoning_price_micro_usd_per_1k?: number;
  embedding_price_micro_usd_per_1k?: number;
  input_cost_micro_usd?: number;
  output_cost_micro_usd?: number;
  cached_input_cost_micro_usd?: number;
  reasoning_cost_micro_usd?: number;
  embedding_cost_micro_usd?: number;
  total_cost_micro_usd: number;
  latency_ms: number;
  time_to_first_token_ms?: number;
  tokens_per_second: number;
  status: string;
  error_code?: string;
  error_message: string;
  retry_count?: number;
  prompt_mode: string;
  max_output_tokens: number;
  context_window_k: number;
  stream_enabled: boolean;
  metadata_json?: string;
  created_at?: string;
};

export type UsageQuotaDashboard = {
  configured_count: number;
  total_cap_micro_usd: number;
  total_spent_micro_usd: number;
  max_utilization_ratio: number;
};

export type BudgetAlert = {
  id: string;
  scope_type: string;
  scope_id: string;
  alert_ratio: number;
  enabled: boolean;
  last_fired_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type UsageModelInsight = {
  provider_code: string;
  model_api_id: string;
  model_display_name: string;
  call_count: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  avg_latency_ms: number;
  avg_tokens_per_second: number;
  success_rate: number;
  flags: string[];
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
  quota_dashboard?: UsageQuotaDashboard;
  inefficient_models?: UsageModelInsight[];
};

// AllModelsBreakdownQuery 是全模型消耗总览表的查询参数（snake_case）。
// 与 ModelUsageQuery 不同：支持服务端分页/搜索/动态排序，专为前端 QTable 分页 UI 设计。
export type AllModelsBreakdownQuery = {
  range?: string; // today | 7d | 30d | month
  start_date?: string; // YYYY-MM-DD 显式覆盖
  end_date?: string; // YYYY-MM-DD 显式覆盖
  provider_code?: string; // 可选 provider 过滤（精确匹配）
  search?: string; // 可选 LIKE 搜索（provider_code + model_api_id）
  sort_field?: string; // call_count | total_tokens | total_cost_micro_usd | success_rate | avg_latency_ms
  sort_dir?: string; // asc | desc
  page?: number; // 1-based
  page_size?: number; // 默认 20，最大 100
};

export type AllModelsBreakdownResult = {
  items: ModelUsageBreakdownRow[];
  total: number;
  page: number;
  page_size: number;
};
