import { useQuasar } from "quasar";
import type { PlatformResource, ProviderConfig, ModelCategory, CapabilityChip } from "./types";
import { getConfig } from "./providerUtils";
import { usePlatformStore } from "../../stores/platform";
import type { Ref, ComputedRef } from "vue";
import type { ProviderHAForm } from "../../components/platform/ProviderHAConfig.vue";

type ProviderForm = {
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

export function useProviderSave(deps: {
  editingId: Ref<string>;
  dialogOpen: Ref<boolean>;
  saving: Ref<boolean>;
  resource: ComputedRef<string>;
  isProviderResource: ComputedRef<boolean>;
  rows: Ref<PlatformResource[]>;
  providerForm: ProviderForm;
  providerHAForm: ProviderHAForm;
  providerAddMode: Ref<"catalog" | "custom">;
  canSubmitNewProviderModel: ComputedRef<boolean>;
  providerIdentityChanged: ComputedRef<boolean>;
  isProviderCodeValid: (value: string) => boolean;
}) {
  const platformStore = usePlatformStore();
  const $q = useQuasar();

  function buildProviderPayload() {
    const editingRow = deps.editingId.value ? deps.rows.value.find((row) => row.id === deps.editingId.value) : undefined;
    const existingConfig = editingRow ? getConfig(editingRow) : {};
    const nextApiKey = deps.providerForm.api_key.trim();
    const config: ProviderConfig = {
      provider_type: deps.providerForm.provider_type,
      variant: deps.providerForm.provider_type === "openai" ? deps.providerForm.variant : undefined,
      provider_display_name: deps.providerForm.provider_display_name.trim(),
      api_base_url: deps.providerForm.api_base_url.trim(),
      api_key_set: Boolean(nextApiKey) || deps.providerForm.api_key_set,
      model_category: deps.providerForm.model_category,
      model_size_label: deps.providerForm.model_size_label.trim(),
      context_window_k: deps.providerForm.context_window_k,
      max_output_tokens: deps.providerForm.max_output_tokens,
      tokens_per_second: existingConfig.tokens_per_second,
      model_hotness_score: existingConfig.model_hotness_score,
      usage_call_count_30d: existingConfig.usage_call_count_30d,
      usage_total_tokens_30d: existingConfig.usage_total_tokens_30d,
      usage_cost_micro_usd_30d: existingConfig.usage_cost_micro_usd_30d,
      success_rate_30d: existingConfig.success_rate_30d,
      avg_latency_ms_30d: existingConfig.avg_latency_ms_30d,
      cost: {
        input_usd_per_1m: deps.providerForm.input_price_usd_per_1m,
        output_usd_per_1m: deps.providerForm.output_price_usd_per_1m,
        cache_read_usd_per_1m: deps.providerForm.cache_read_usd_per_1m,
        cache_write_usd_per_1m: deps.providerForm.cache_write_usd_per_1m,
        reasoning_usd_per_1m: deps.providerForm.reasoning_price_usd_per_1m,
        embedding_usd_per_1m: deps.providerForm.embedding_price_usd_per_1m,
      },
      capability_chips: deps.providerForm.capability_chips,
      catalog_source: deps.providerForm.catalog_source || (deps.providerAddMode.value === "custom" ? "custom" : ""),
      catalog_managed: deps.providerForm.catalog_managed,
      raw_metadata_json: deps.providerForm.raw_metadata_json,
      metadata_source: deps.providerForm.metadata_source,
      last_used_at: existingConfig.last_used_at,
      model_rating: deps.providerForm.model_rating,
      ha_mode: deps.providerHAForm.haMode || undefined,
      ha_candidates: deps.providerHAForm.haCandidates
        .filter((c) => c.name.trim())
        .map((c) => ({
          name: c.name.trim(),
          provider_type: c.providerType,
          base_url: c.baseUrl.trim(),
          api_key: c.apiKey.trim() || undefined
        })),
      ha_hedge_delay_ms: deps.providerHAForm.haMode === "hedge" ? deps.providerHAForm.haHedgeDelayMs : undefined,
      enable_token_tailoring: deps.providerForm.enable_token_tailoring,
      optimize_for_cache: deps.providerForm.optimize_for_cache,
      reasoning_content_backfill: deps.providerForm.reasoning_backfill,
      show_tool_call_delta: deps.providerForm.show_tool_call_delta,
      keep_alive_minutes: deps.providerForm.keep_alive_minutes,
      rate_limit_rpm: deps.providerForm.rate_limit_rpm || undefined
    };
    if (nextApiKey) {
      config.api_key = nextApiKey;
      config.api_key_set = true;
    }
    if (deps.providerForm.secret_id.trim()) {
      config.secret_id = deps.providerForm.secret_id.trim();
    }
    if (deps.providerForm.secret_key.trim()) {
      config.secret_key = deps.providerForm.secret_key.trim();
    }
    if (deps.providerForm.aws_region.trim()) {
      config.aws_region = deps.providerForm.aws_region.trim();
    }

    const code = deps.providerForm.provider_code.trim();
    const model = deps.providerForm.model_api_id.trim();
    return {
      key: `${code}:${model}`,
      name: deps.providerForm.model_display_name.trim() || model,
      description: deps.providerForm.description.trim(),
      enabled: deps.providerForm.enabled,
      sort_order: deps.providerForm.sort_order,
      provider: code,
      model,
      config_json: JSON.stringify(config),
      metadata_json: JSON.stringify({ model_rating: deps.providerForm.model_rating })
    };
  }

  async function saveProviderRow() {
    const code = deps.providerForm.provider_code.trim();
    const model = deps.providerForm.model_api_id.trim();
    if (!code || !model || !deps.isProviderCodeValid(code)) {
      $q.notify({ type: "negative", message: "Provider 名称和模型ID必填，名称仅支持小写字母、数字、连字符" });
      return;
    }
    if (!deps.canSubmitNewProviderModel.value) {
      $q.notify({
        type: "warning",
        message:
          deps.providerAddMode.value === "catalog"
            ? "请从目录选择 Provider 与模型"
            : deps.editingId.value && deps.providerIdentityChanged.value
              ? "修改 Provider ID 或模型 ID 后请先点击「检查」"
              : "请先点击「检查」并通过验证后再创建"
      });
      return;
    }

    if (deps.editingId.value && !deps.providerIdentityChanged.value) {
      const pre = await platformStore.checkModel(code, model);
      if (!pre.ok) {
        $q.notify({
          type: "negative",
          message: pre.message || "目录中无已启用的 provider/model，请启用本条或修正 Provider ID / 模型 ID"
        });
        return;
      }
    }

    const payload = buildProviderPayload();
    deps.saving.value = true;
    try {
      if (deps.editingId.value) {
        const updated = await platformStore.editResource(deps.resource.value, deps.editingId.value, payload);
        deps.rows.value = deps.rows.value.map((row) => (row.id === updated.id ? updated : row));
      } else {
        const created = await platformStore.addResource(deps.resource.value, payload);
        deps.rows.value = [created, ...deps.rows.value];
      }
      deps.dialogOpen.value = false;
      const post = await platformStore.checkModel(code, model);
      if (!post.ok) {
        $q.notify({
          type: "warning",
          message: post.message || "已保存，但运行时校验未通过，请确认已启用且 Provider ID 正确"
        });
      } else {
        $q.notify({ type: "positive", message: "已保存" });
      }
    } finally {
      deps.saving.value = false;
    }
  }

  return { buildProviderPayload, saveProviderRow };
}
