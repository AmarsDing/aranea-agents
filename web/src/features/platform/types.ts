/** 平台资源 UI / Kratos 映射类型（与 `platform/api` 响应字段对齐）。 */

export type PlatformResourceName =
  | 'avatar-assets'
  | 'taxonomy-nodes'
  | 'taxonomy'
  | 'llm-provider-models'
  | 'hooks'
  | 'channels'
  | 'mcp-servers'
  | 'skills'
  | 'cron-tasks'
  | 'monitor-events'
  | 'monitor-traces';

export type PlatformResource = {
  id: string;
  resource: PlatformResourceName;
  key: string;
  name: string;
  description: string;
  status: string;
  enabled: boolean;
  sort_order: number;
  parent_id: string;
  level: string;
  agent_id: string;
  provider: string;
  model: string;
  is_system: boolean;
  config_json: string;
  metadata_json: string;
  capabilities?: {
    text?: boolean;
    vision?: boolean;
    image?: boolean;
    audio?: boolean;
    file?: boolean;
    tool_call?: boolean;
    cache?: boolean;
    thinking?: boolean;
    text_only?: boolean;
  };
  created_at: string;
  updated_at: string;
  deleted_at: string;
};

export type PlatformResourceTreeNode = PlatformResource & {
  children?: PlatformResourceTreeNode[];
  agent_count?: number;
};

export type PlatformResourceInput = Partial<
  Omit<PlatformResource, 'id' | 'resource' | 'created_at' | 'updated_at' | 'deleted_at'>
> & {
  key: string;
  name: string;
};

export type ValidateModelResult = {
  ok: boolean;
  message: string;
};

export type InspectProviderModelInput = {
  resource_id?: string;
  provider_code: string;
  provider_type: string;
  model_api_id: string;
  api_base_url: string;
  api_key?: string;
  variant?: string;
  secret_id?: string;
  secret_key?: string;
  aws_region?: string;
};

export type InspectProviderModelResult = {
  ok: boolean;
  message: string;
  provider_code: string;
  provider_type: string;
  model_api_id: string;
  model_display_name: string;
  model_size_label: string;
  context_window_k: number;
  max_output_tokens: number;
  input_price_micro_usd_per_1k: number;
  output_price_micro_usd_per_1k: number;
  cached_input_price_micro_usd_per_1k: number;
  reasoning_price_micro_usd_per_1k: number;
  embedding_price_micro_usd_per_1k: number;
  source: string;
  raw_metadata_json: string;
  variant?: string;
  enable_token_tailoring?: boolean;
  supports_cache?: boolean;
  supports_thinking?: boolean;
};

export type RevealProviderCredentialsResult = {
  api_key: string;
  secret_key: string;
  has_api_key: boolean;
  has_secret_key: boolean;
  ha_candidates: { name: string; api_key: string }[];
};

export type ModelCategory = {
  value: string;
  label: string;
  tooltip: string;
};

export type CapabilityChip = {
  key: string;
  label: string;
  source?: string;
};

export type ProviderConfig = {
  provider_type?: string;
  variant?: string;
  provider_display_name?: string;
  api_base_url?: string;
  api_key?: string;
  api_key_set?: boolean;
  secret_id?: string;
  secret_key?: string;
  aws_region?: string;
  ha_mode?: string;
  ha_candidates?: { name: string; provider_type: string; base_url: string; api_key?: string }[];
  ha_hedge_delay_ms?: number;
  enable_token_tailoring?: boolean;
  optimize_for_cache?: boolean;
  reasoning_content_backfill?: boolean;
  show_tool_call_delta?: boolean;
  keep_alive_minutes?: number;
  rate_limit_rpm?: number;
  model_category?: ModelCategory[];
  model_size_label?: string;
  context_window_k?: number | string | null;
  max_output_tokens?: number | string | null;
  tokens_per_second?: number | string | null;
  model_hotness_score?: number | string | null;
  usage_call_count_30d?: number | string | null;
  usage_total_tokens_30d?: number | string | null;
  usage_cost_micro_usd_30d?: number | string | null;
  success_rate_30d?: number | string | null;
  avg_latency_ms_30d?: number | string | null;
  input_price_micro_usd_per_1k?: number | string | null;
  output_price_micro_usd_per_1k?: number | string | null;
  cached_input_price_micro_usd_per_1k?: number | string | null;
  reasoning_price_micro_usd_per_1k?: number | string | null;
  embedding_price_micro_usd_per_1k?: number | string | null;
  cost?: {
    input_usd_per_1m?: number;
    output_usd_per_1m?: number;
    cache_read_usd_per_1m?: number;
    cache_write_usd_per_1m?: number;
    reasoning_usd_per_1m?: number;
    embedding_usd_per_1m?: number;
  };
  capability_chips?: CapabilityChip[];
  catalog_source?: string;
  catalog_managed?: boolean;
  raw_metadata_json?: string;
  metadata_source?: string;
  last_used_at?: string;
  model_rating?: number | string | null;
};

export type HACandidateForm = {
  name: string;
  providerType: string;
  baseUrl: string;
  apiKey: string;
};

export type ProviderHAForm = {
  haMode: '' | 'failover' | 'hedge';
  haCandidates: HACandidateForm[];
  haHedgeDelayMs: number;
};

export type ProviderForm = {
  provider_type: string;
  variant: string;
  model_api_id: string;
  provider_code: string;
  provider_display_name: string;
  model_display_name: string;
  api_base_url: string;
  api_key: string;
  api_key_set: boolean;
  secret_id: string;
  secret_key: string;
  aws_region: string;
  enabled: boolean;
  model_category: ModelCategory[];
  model_size_label: string;
  context_window_k: number | null;
  max_output_tokens: number;
  model_rating: number;
  input_price_usd_per_1m: number;
  output_price_usd_per_1m: number;
  cache_read_usd_per_1m: number;
  cache_write_usd_per_1m: number;
  reasoning_price_usd_per_1m: number;
  embedding_price_usd_per_1m: number;
  capability_chips: CapabilityChip[];
  catalog_managed: boolean;
  catalog_source: string;
  raw_metadata_json: string;
  metadata_source: string;
  sort_order: number;
  description: string;
  enable_token_tailoring: boolean;
  optimize_for_cache: boolean;
  reasoning_backfill: boolean;
  show_tool_call_delta: boolean;
  keep_alive_minutes: number;
  rate_limit_rpm: number;
};
