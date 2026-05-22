/** 平台资源 UI / Kratos 映射类型（与 `platform/api` 响应字段对齐）。 */

export type PlatformResourceName =
  | "avatar-assets"
  | "agent-categories"
  | "llm-provider-models"
  | "hooks"
  | "channels"
  | "mcp-servers"
  | "skills"
  | "cron-tasks"
  | "monitor-events"
  | "monitor-traces";

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
  config_json: string;
  metadata_json: string;
  created_at: string;
  updated_at: string;
  deleted_at: string;
};

export type PlatformResourceTreeNode = PlatformResource & {
  children?: PlatformResourceTreeNode[];
};

export type PlatformResourceInput = Partial<Omit<PlatformResource, "id" | "resource" | "created_at" | "updated_at" | "deleted_at">> & {
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

