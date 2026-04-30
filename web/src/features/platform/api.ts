import { legacyRestApi as api } from "../../services";

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
};

export async function listPlatformResources(resource: PlatformResourceName): Promise<PlatformResource[]> {
  const { data } = await api.get(`/${resource}`);
  return data.items ?? [];
}

export async function listPlatformResourceTree(resource: "agent-categories"): Promise<PlatformResourceTreeNode[]> {
  const { data } = await api.get(`/${resource}/tree`);
  return data.items ?? [];
}

export async function createPlatformResource(
  resource: PlatformResourceName,
  payload: PlatformResourceInput
): Promise<PlatformResource> {
  const { data } = await api.post(`/${resource}`, payload);
  return data;
}

export async function updatePlatformResource(
  resource: PlatformResourceName,
  id: string,
  payload: Partial<PlatformResourceInput>
): Promise<PlatformResource> {
  const { data } = await api.patch(`/${resource}/${id}`, payload);
  return data;
}

export async function deletePlatformResource(resource: PlatformResourceName, id: string): Promise<void> {
  await api.delete(`/${resource}/${id}`);
}

export async function validateModel(provider: string, model: string): Promise<ValidateModelResult> {
  const { data } = await api.post("/agents/validate-model", { provider, model });
  return data;
}

export async function inspectProviderModel(payload: InspectProviderModelInput): Promise<InspectProviderModelResult> {
  const { data } = await api.post("/llm-provider-models/inspect", payload);
  return data;
}

/** @deprecated 请从 `features/avatar/api` 导入 */
export type { AvatarAsset } from "../avatar/api";
/** @deprecated 请从 `features/avatar/api` 导入 */
export { listAvatarAssets, uploadAvatarAsset } from "../avatar/api";
