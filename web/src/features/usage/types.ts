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
  status?: string;
  limit?: number;
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
  agent_id: string;
  agent_key: string;
  session_id: string;
  message_id: string;
  provider_code: string;
  provider_type: string;
  provider_display_name: string;
  model_api_id: string;
  model_display_name: string;
  call_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  latency_ms: number;
  tokens_per_second: number;
  status: string;
  error_message: string;
  prompt_mode: string;
  max_output_tokens: number;
  context_window_k: number;
  stream_enabled: boolean;
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
};
