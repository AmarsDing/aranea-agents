import { computed, reactive, ref, type ComputedRef, type Ref } from "vue";
import { useQuasar } from "quasar";
import type { PlatformResource, PlatformResourceName, ProviderConfig, ModelCategory, CapabilityChip } from "./types";
import { errorMessage, toNullableNumber, toNumber, getConfig, getCategories } from "./providerUtils";
import { usePlatformStore } from "../../stores/platform";
import type { ProviderHAForm } from "../../components/platform/ProviderHAConfig.vue";
import {
  PROVIDER_PRESETS,
  findModelPreset,
  findProviderPreset,
  type ProviderModelPreset
} from "../../config/providerPresets";
import {
  PROVIDER_RUNTIME_OVERLAY,
  PROVIDER_TYPE_OPTIONS,
  VARIANT_OPTIONS,
  runtimeProfileFor,
  usdPer1MToMicroPer1K,
  microPer1KToUsdPer1M
} from "../../config/providerRuntimeOverlay";
import { MODEL_CATEGORY_OPTIONS } from "../model-catalog/catalogCategories";
import { catalogProviderIdFor, ensureProviderMigrationMap } from "../model-catalog/providerMigration";
import {
  hasPricingConfigured,
} from "../usage/pricingWarning";
import { useProviderCatalog } from "./useProviderCatalog";
import { useProviderCredentials } from "./useProviderCredentials";

export function useProviderWizard(deps: {
  editingId: Ref<string>;
  dialogOpen: Ref<boolean>;
  saving: Ref<boolean>;
  resource: ComputedRef<PlatformResourceName>;
  isProviderResource: ComputedRef<boolean>;
  rows: Ref<PlatformResource[]>;
}) {
  const platformStore = usePlatformStore();
  const $q = useQuasar();

  const providerForm = reactive({
    provider_type: "openai",
    variant: "openai",
    model_api_id: "",
    provider_code: "",
    provider_display_name: "",
    model_display_name: "",
    api_base_url: "",
    api_key: "",
    api_key_set: false,
    secret_id: "",
    secret_key: "",
    aws_region: "",
    enabled: true,
    model_category: [] as ModelCategory[],
    model_size_label: "",
    context_window_k: null as number | null,
    max_output_tokens: 4096,
    model_rating: 60,
    input_price_micro_usd_per_1k: 0,
    output_price_micro_usd_per_1k: 0,
    cached_input_price_micro_usd_per_1k: 0,
    reasoning_price_micro_usd_per_1k: 0,
    embedding_price_micro_usd_per_1k: 0,
    input_price_usd_per_1m: 0,
    output_price_usd_per_1m: 0,
    cache_read_usd_per_1m: 0,
    cache_write_usd_per_1m: 0,
    reasoning_price_usd_per_1m: 0,
    embedding_price_usd_per_1m: 0,
    capability_chips: [] as CapabilityChip[],
    catalog_managed: false,
    catalog_source: "",
    raw_metadata_json: "",
    metadata_source: "",
    sort_order: 0,
    description: "",
    enable_token_tailoring: true,
    optimize_for_cache: false,
    reasoning_backfill: true,
    show_tool_call_delta: false,
    keep_alive_minutes: 5,
    rate_limit_rpm: 0
  });

  const providerHAForm = reactive<ProviderHAForm>({
    haMode: "",
    haCandidates: [],
    haHedgeDelayMs: 100
  });

  const providerPresetKey = ref("");
  const providerStep = ref(1);
  const providerAddMode = ref<"catalog" | "custom">("catalog");
  const providerCreateInspectFingerprint = ref("");
  const providerEditIdentityAtOpen = ref("");
  const checkingModel = ref(false);

  const categoryOptions: ModelCategory[] = MODEL_CATEGORY_OPTIONS;
  const providerTypeOptions = PROVIDER_TYPE_OPTIONS;
  const providerTypeFilterOptions = PROVIDER_TYPE_OPTIONS;
  const variantOptions = VARIANT_OPTIONS;

  const currentProviderPreset = computed(() => findProviderPreset(providerPresetKey.value || providerForm.provider_code));

  const catalog = useProviderCatalog({
    providerForm,
    providerAddMode,
    providerCreateInspectFingerprint,
    editingId: deps.editingId,
    dialogOpen: deps.dialogOpen,
    isProviderResource: deps.isProviderResource,
    currentProviderPreset,
  });

  const credentials = useProviderCredentials({
    providerForm,
    providerHAForm,
    editingId: deps.editingId,
    isProviderResource: deps.isProviderResource,
  });

  const currentAuthType = computed(() => {
    if (providerAddMode.value === "catalog" && catalog.catalogProviderId.value) {
      return runtimeProfileFor(catalog.catalogProviderId.value).authType;
    }
    return currentProviderPreset.value?.authType || runtimeProfileFor(providerForm.provider_code).authType || "api_key";
  });

  const isLocalProviderModel = computed(() => {
    if (providerForm.provider_type === "ollama") return true;
    const raw = providerForm.api_base_url.trim();
    if (!raw) return false;
    return /^(https?|wss?):\/\/(localhost|127\.0\.0\.1|\[::1\])([/:?#]|$)/i.test(raw);
  });

  const hasInspectApiKey = computed(() => {
    if (providerForm.api_key.trim()) return true;
    if (providerForm.secret_id.trim() && providerForm.secret_key.trim()) return true;
    if (providerForm.aws_region.trim()) return true;
    if (deps.editingId.value && providerForm.api_key_set) return true;
    return false;
  });

  const canInspectProviderModel = computed(() => {
    if (!providerForm.provider_code.trim() || !providerForm.model_api_id.trim()) return false;
    if (isLocalProviderModel.value) return true;
    return hasInspectApiKey.value;
  });

  const apiKeyFieldHint = computed(() => {
    const parts: string[] = [];
    if (deps.editingId.value && providerForm.api_key_set && !providerForm.api_key.trim()) {
      parts.push("已保存密钥，点击眼睛图标查看");
    } else if (deps.editingId.value) {
      parts.push("留空表示不修改");
    }
    if (!isLocalProviderModel.value) parts.push("远程 Provider 检查模型前需填写密钥");
    return parts.join("；") || undefined;
  });

  const apiKeyMaskedPlaceholder = computed(() => {
    if (deps.editingId.value && providerForm.api_key_set && !providerForm.api_key.trim() && !credentials.showApiKey.value) {
      return "••••••••••••";
    }
    return undefined;
  });

  const secretKeyMaskedPlaceholder = computed(() => {
    if (deps.editingId.value && providerForm.secret_id.trim() && !providerForm.secret_key.trim() && !credentials.showSecretKey.value) {
      return "••••••••••••";
    }
    return undefined;
  });

  const providerPresetOptions = computed(() =>
    PROVIDER_PRESETS.map((preset) => ({
      label: preset.label,
      value: preset.key,
      caption: `${preset.apiBaseUrl || "手动配置"} · ${metadataLabel(preset.metadataApi)}`
    }))
  );

  const providerRuntimeLocked = computed(() => {
    const code = catalog.activeCatalogProviderId.value;
    if (!code) return providerAddMode.value === "catalog";
    if (providerAddMode.value === "catalog" && catalog.catalogProviderId.value) return true;
    return catalog.catalogModels.value.length > 0 || Boolean(PROVIDER_RUNTIME_OVERLAY[code]);
  });

  const providerRuntimeSummary = computed(() => {
    const rt = runtimeProfileFor(catalog.activeCatalogProviderId.value || providerForm.provider_code.trim());
    const typeLabel =
      PROVIDER_TYPE_OPTIONS.find((o) => o.value === rt.providerType)?.label || rt.providerType;
    if (rt.providerType === "openai" && rt.variant) {
      const variantLabel = VARIANT_OPTIONS.find((o) => o.value === rt.variant)?.label || rt.variant;
      return `${typeLabel} · ${variantLabel}`;
    }
    return typeLabel;
  });

  const dialogTitle = computed(() => {
    if (!deps.isProviderResource.value) return deps.editingId.value ? "编辑资源" : "新增资源";
    return deps.editingId.value ? "编辑Provider" : "添加Provider";
  });

  const dialogSubtitle = computed(() => {
    if (!deps.isProviderResource.value) return "Key 和 Name 为必填，其他字段按模块需要填写。";
    if (!deps.editingId.value) {
      return providerAddMode.value === "catalog"
        ? "从 models.dev 目录选择 Provider 与模型，规格与定价自动回填；远程 Provider 建议检查连通性。"
        : "配置 LLM Provider 连接。自定义模式需先点击「检查」并通过验证后再创建。";
    }
    return "配置 LLM Provider 连接";
  });

  function effectiveMicroPrice(usdPer1M: number): number {
    return usdPer1MToMicroPer1K(usdPer1M);
  }

  const showPricingWarning = computed(
    () =>
      deps.isProviderResource.value &&
      !hasPricingConfigured({
        inputPrice: effectiveMicroPrice(providerForm.input_price_usd_per_1m),
        outputPrice: effectiveMicroPrice(providerForm.output_price_usd_per_1m),
        inputPriceCached: effectiveMicroPrice(providerForm.cache_read_usd_per_1m),
        outputPriceReasoning: effectiveMicroPrice(providerForm.reasoning_price_usd_per_1m),
      })
  );

  const providerIdentityChanged = computed(() => {
    if (!deps.editingId.value || !deps.isProviderResource.value) return false;
    const cur = `${providerForm.provider_code.trim()}\0${providerForm.model_api_id.trim()}`;
    return cur !== providerEditIdentityAtOpen.value;
  });

  const providerRuntimeBindingPreview = computed(() => {
    const code = providerForm.provider_code.trim();
    const model = providerForm.model_api_id.trim();
    if (!code || !model) return "";
    return `Agent / 运行时将使用：${code} / ${model}`;
  });

  function providerCreateInspectFingerprintValue(): string {
    return [
      providerForm.provider_code.trim(),
      providerForm.model_api_id.trim(),
      providerForm.api_base_url.trim(),
      providerForm.api_key.trim(),
      providerForm.provider_type.trim(),
      providerForm.variant.trim(),
      providerForm.secret_id.trim(),
      providerForm.secret_key.trim(),
      providerForm.aws_region.trim()
    ].join("\0");
  }

  const canSubmitNewProviderModel = computed(() => {
    if (!deps.isProviderResource.value) return true;
    if (deps.editingId.value && !providerIdentityChanged.value) return true;
    if (isLocalProviderModel.value) return true;
    if (
      catalog.activeCatalogProviderId.value &&
      providerForm.model_api_id.trim() &&
      catalog.catalogModels.value.some((m) => m.id === providerForm.model_api_id.trim())
    ) {
      return true;
    }
    if (
      providerAddMode.value === "catalog" &&
      catalog.catalogProviderId.value &&
      providerForm.catalog_managed &&
      providerForm.model_api_id.trim()
    ) {
      return true;
    }
    const saved = providerCreateInspectFingerprint.value;
    if (!saved) return false;
    return saved === providerCreateInspectFingerprintValue();
  });

  function setProviderAddMode(mode: "catalog" | "custom") {
    providerAddMode.value = mode;
    providerCreateInspectFingerprint.value = "";
    if (mode === "custom") {
      catalog.catalogProviderId.value = "";
      catalog.catalogModels.value = [];
      providerForm.catalog_managed = false;
      providerForm.catalog_source = "custom";
      providerForm.metadata_source = "custom";
      providerForm.capability_chips = [];
      return;
    }
    providerForm.catalog_source = "models.dev";
    void catalog.ensureCatalogLoaded();
  }

  async function applyProviderPreset(key: string) {
    const preset = findProviderPreset(key);
    if (!preset) return;
    providerPresetKey.value = preset.key;
    providerForm.provider_code = preset.providerCode;
    providerForm.provider_display_name = preset.label;
    providerForm.api_base_url = preset.apiBaseUrl;
    catalog.applyProviderRuntimeFields(preset.providerCode);
    catalog.catalogProviderId.value = preset.providerCode;
    catalog.catalogModelFilterLocal.value = "";
    await catalog.loadCatalogModels(preset.providerCode, "", true);
    if (catalog.catalogModels.value.length > 0) {
      const keep = catalog.catalogModels.value.find((m) => m.id === providerForm.model_api_id);
      const pick = keep ?? catalog.catalogModels.value[0];
      if (pick?.id) {
        providerForm.model_api_id = pick.id;
        catalog.applyCatalogModel(pick.id);
      }
      return;
    }
    if (!providerForm.model_api_id && preset.models[0]) {
      providerForm.model_api_id = preset.models[0].id;
      applyModelPreset(preset.models[0].id);
    } else {
      applyModelPreset(providerForm.model_api_id);
    }
  }

  function applyModelPreset(modelId: string) {
    const preset = findModelPreset(providerPresetKey.value || providerForm.provider_code, modelId);
    if (!preset) return;
    applyModelPresetValues(preset, false);
  }

  function applyModelPresetValues(preset: ProviderModelPreset, overwrite = false) {
    providerForm.model_display_name = overwrite || !providerForm.model_display_name ? preset.label : providerForm.model_display_name;
    providerForm.model_size_label = overwrite || !providerForm.model_size_label ? preset.sizeLabel || providerForm.model_size_label : providerForm.model_size_label;
    providerForm.context_window_k = overwrite || !providerForm.context_window_k ? preset.contextWindowK ?? providerForm.context_window_k : providerForm.context_window_k;
    providerForm.max_output_tokens = overwrite || !providerForm.max_output_tokens ? preset.maxOutputTokens ?? providerForm.max_output_tokens : providerForm.max_output_tokens;
    if (overwrite || !providerForm.input_price_usd_per_1m) {
      providerForm.input_price_usd_per_1m = microPer1KToUsdPer1M(preset.inputPriceMicroUsdPer1K ?? 0);
    }
    if (overwrite || !providerForm.output_price_usd_per_1m) {
      providerForm.output_price_usd_per_1m = microPer1KToUsdPer1M(preset.outputPriceMicroUsdPer1K ?? 0);
    }
    if (overwrite || !providerForm.cache_read_usd_per_1m) {
      providerForm.cache_read_usd_per_1m = 0;
    }
    if (overwrite || !providerForm.reasoning_price_usd_per_1m) {
      providerForm.reasoning_price_usd_per_1m = 0;
    }
    if (overwrite || !providerForm.embedding_price_usd_per_1m) {
      providerForm.embedding_price_usd_per_1m = 0;
    }
  }

  function setCustomModelValue(value: string, done: (value: string, mode?: "add" | "add-unique" | "toggle") => void) {
    done(value, "add-unique");
    providerForm.model_api_id = value;
  }

  async function inspectCurrentProviderModel() {
    const code = providerForm.provider_code.trim();
    const model = providerForm.model_api_id.trim();
    if (!code || !model) {
      $q.notify({ type: "negative", message: "请先填写 Provider 名称和模型ID" });
      return;
    }
    if (!canInspectProviderModel.value) {
      $q.notify({ type: "warning", message: "非本地模型需填写 API 密钥后才能检查" });
      return;
    }
    checkingModel.value = true;
    try {
      const result = await platformStore.inspectModel({
        resource_id: deps.editingId.value,
        provider_code: code,
        provider_type: providerForm.provider_type,
        model_api_id: model,
        api_base_url: providerForm.api_base_url.trim(),
        api_key: providerForm.api_key.trim(),
        variant: providerForm.variant,
        secret_id: providerForm.secret_id.trim(),
        secret_key: providerForm.secret_key.trim(),
        aws_region: providerForm.aws_region.trim()
      });
      if (!result.ok) {
        if (!deps.editingId.value) {
          providerCreateInspectFingerprint.value = "";
        }
        const preset = findModelPreset(providerPresetKey.value || code, model);
        const catalogModel = catalog.catalogModels.value.find((item) => item.id === model);
        if (catalogModel && catalog.catalogProviderId.value) {
          catalog.applyCatalogModel(catalogModel.id || model);
          providerForm.catalog_managed = true;
          if (!deps.editingId.value) {
            providerCreateInspectFingerprint.value = providerCreateInspectFingerprintValue();
          }
          $q.notify({ type: "warning", message: `${result.message || "未获取到模型参数"}；已使用 models.dev 目录参数回填` });
          return;
        }
        if (preset) {
          applyModelPresetValues(preset, true);
          providerForm.metadata_source = `${currentProviderPreset.value?.key || code}-preset`;
          providerForm.raw_metadata_json = JSON.stringify({ source: "frontend-provider-preset", provider: providerPresetKey.value || code, model });
          $q.notify({ type: "warning", message: `${result.message || "未获取到模型参数"}；已使用前端预设参数回填` });
          return;
        }
        $q.notify({ type: "warning", message: result.message || "未获取到模型参数，也没有匹配的预设参数" });
        return;
      }
      if (!deps.editingId.value) {
        providerCreateInspectFingerprint.value = providerCreateInspectFingerprintValue();
      }
      if (catalog.catalogProviderId.value && providerForm.model_api_id.trim()) {
        const cm = catalog.catalogModels.value.find((item) => item.id === providerForm.model_api_id.trim());
        if (cm && !providerForm.input_price_usd_per_1m && !providerForm.output_price_usd_per_1m) {
          catalog.applyCatalogModel(cm.id || providerForm.model_api_id.trim());
        }
      }
      if (result.enable_token_tailoring) providerForm.enable_token_tailoring = true;
      if (!providerForm.model_display_name.trim()) {
        providerForm.model_display_name = result.model_display_name || model;
      }
      if (result.model_size_label && !providerForm.model_size_label.trim()) {
        providerForm.model_size_label = result.model_size_label;
      }
      if (result.raw_metadata_json && !providerForm.raw_metadata_json.trim()) {
        providerForm.raw_metadata_json = result.raw_metadata_json;
      }
      $q.notify({ type: "positive", message: result.message || "已验证 Provider 连通性" });
    } catch (error) {
      if (!deps.editingId.value) {
        providerCreateInspectFingerprint.value = "";
      }
      $q.notify({ type: "negative", message: errorMessage(error) });
    } finally {
      checkingModel.value = false;
    }
  }

  function normalizeProviderType(raw: string | undefined): string {
    const v = (raw || "").trim().toLowerCase();
    if (v === "anthropic") return "anthropic";
    if (v === "gemini" || v === "google gemini") return "gemini";
    if (v === "ollama") return "ollama";
    if (v === "hunyuan") return "hunyuan";
    if (v === "huggingface") return "huggingface";
    if (v === "bedrock") return "bedrock";
    return "openai";
  }

  function isProviderCodeValid(value: string) {
    return /^[a-z0-9-]+$/.test(value);
  }

  function providerCodeRule(value: string) {
    return isProviderCodeValid(value) || "仅支持小写字母、数字、连字符";
  }

  function metadataLabel(value: string) {
    if (value === "full") return "可查询参数";
    if (value === "partial") return "可验证模型";
    if (value === "limited") return "有限查询";
    return "手动维护";
  }

  function buildProviderPayload() {
    const editingRow = deps.editingId.value ? deps.rows.value.find((row) => row.id === deps.editingId.value) : undefined;
    const existingConfig = editingRow ? getConfig(editingRow) : {};
    const nextApiKey = providerForm.api_key.trim();
    const config: ProviderConfig = {
      provider_type: providerForm.provider_type,
      variant: providerForm.provider_type === "openai" ? providerForm.variant : undefined,
      provider_display_name: providerForm.provider_display_name.trim(),
      api_base_url: providerForm.api_base_url.trim(),
      api_key_set: Boolean(nextApiKey) || providerForm.api_key_set,
      model_category: providerForm.model_category,
      model_size_label: providerForm.model_size_label.trim(),
      context_window_k: providerForm.context_window_k,
      max_output_tokens: providerForm.max_output_tokens,
      tokens_per_second: existingConfig.tokens_per_second,
      model_hotness_score: existingConfig.model_hotness_score,
      usage_call_count_30d: existingConfig.usage_call_count_30d,
      usage_total_tokens_30d: existingConfig.usage_total_tokens_30d,
      usage_cost_micro_usd_30d: existingConfig.usage_cost_micro_usd_30d,
      success_rate_30d: existingConfig.success_rate_30d,
      avg_latency_ms_30d: existingConfig.avg_latency_ms_30d,
      cost: {
        input_usd_per_1m: providerForm.input_price_usd_per_1m,
        output_usd_per_1m: providerForm.output_price_usd_per_1m,
        cache_read_usd_per_1m: providerForm.cache_read_usd_per_1m,
        cache_write_usd_per_1m: providerForm.cache_write_usd_per_1m,
        reasoning_usd_per_1m: providerForm.reasoning_price_usd_per_1m,
        embedding_usd_per_1m: providerForm.embedding_price_usd_per_1m,
      },
      capability_chips: providerForm.capability_chips,
      catalog_source: providerForm.catalog_source || (providerAddMode.value === "custom" ? "custom" : ""),
      catalog_managed: providerForm.catalog_managed,
      raw_metadata_json: providerForm.raw_metadata_json,
      metadata_source: providerForm.metadata_source,
      last_used_at: existingConfig.last_used_at,
      model_rating: providerForm.model_rating,
      ha_mode: providerHAForm.haMode || undefined,
      ha_candidates: providerHAForm.haCandidates
        .filter((c) => c.name.trim())
        .map((c) => ({
          name: c.name.trim(),
          provider_type: c.providerType,
          base_url: c.baseUrl.trim(),
          api_key: c.apiKey.trim() || undefined
        })),
      ha_hedge_delay_ms: providerHAForm.haMode === "hedge" ? providerHAForm.haHedgeDelayMs : undefined,
      enable_token_tailoring: providerForm.enable_token_tailoring,
      optimize_for_cache: providerForm.optimize_for_cache,
      reasoning_content_backfill: providerForm.reasoning_backfill,
      show_tool_call_delta: providerForm.show_tool_call_delta,
      keep_alive_minutes: providerForm.keep_alive_minutes,
      rate_limit_rpm: providerForm.rate_limit_rpm || undefined
    };
    if (nextApiKey) {
      config.api_key = nextApiKey;
      config.api_key_set = true;
    }
    if (providerForm.secret_id.trim()) {
      config.secret_id = providerForm.secret_id.trim();
    }
    if (providerForm.secret_key.trim()) {
      config.secret_key = providerForm.secret_key.trim();
    }
    if (providerForm.aws_region.trim()) {
      config.aws_region = providerForm.aws_region.trim();
    }

    const code = providerForm.provider_code.trim();
    const model = providerForm.model_api_id.trim();
    return {
      key: `${code}:${model}`,
      name: providerForm.model_display_name.trim() || model,
      description: providerForm.description.trim(),
      enabled: providerForm.enabled,
      sort_order: providerForm.sort_order,
      provider: code,
      model,
      config_json: JSON.stringify(config),
      metadata_json: JSON.stringify({ model_rating: providerForm.model_rating })
    };
  }

  async function saveProviderRow() {
    const code = providerForm.provider_code.trim();
    const model = providerForm.model_api_id.trim();
    if (!code || !model || !isProviderCodeValid(code)) {
      $q.notify({ type: "negative", message: "Provider 名称和模型ID必填，名称仅支持小写字母、数字、连字符" });
      return;
    }
    if (!canSubmitNewProviderModel.value) {
      $q.notify({
        type: "warning",
        message:
          providerAddMode.value === "catalog"
            ? "请从目录选择 Provider 与模型"
            : deps.editingId.value && providerIdentityChanged.value
              ? "修改 Provider ID 或模型 ID 后请先点击「检查」"
              : "请先点击「检查」并通过验证后再创建"
      });
      return;
    }

    if (deps.editingId.value && !providerIdentityChanged.value) {
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

  function resetProviderForm() {
    Object.assign(providerForm, {
      provider_type: "openai",
      variant: "openai",
      model_api_id: "",
      provider_code: "",
      provider_display_name: "",
      model_display_name: "",
      api_base_url: "",
      api_key: "",
      api_key_set: false,
      secret_id: "",
      secret_key: "",
      aws_region: "",
      enabled: true,
      model_category: [],
      model_size_label: "",
      context_window_k: null,
      max_output_tokens: 4096,
      model_rating: 60,
      input_price_micro_usd_per_1k: 0,
      output_price_micro_usd_per_1k: 0,
      cached_input_price_micro_usd_per_1k: 0,
      reasoning_price_micro_usd_per_1k: 0,
      embedding_price_micro_usd_per_1k: 0,
      input_price_usd_per_1m: 0,
      output_price_usd_per_1m: 0,
      cache_read_usd_per_1m: 0,
      cache_write_usd_per_1m: 0,
      reasoning_price_usd_per_1m: 0,
      embedding_price_usd_per_1m: 0,
      capability_chips: [],
      catalog_managed: false,
      catalog_source: "",
      raw_metadata_json: "",
      metadata_source: "",
      sort_order: 0,
      description: "",
      enable_token_tailoring: true,
      optimize_for_cache: false,
      reasoning_backfill: true,
      show_tool_call_delta: false,
      keep_alive_minutes: 5,
      rate_limit_rpm: 0
    });
    Object.assign(providerHAForm, { haMode: "", haCandidates: [], haHedgeDelayMs: 100 });
    credentials.showApiKey.value = false;
    credentials.showSecretKey.value = false;
    credentials.credentialsLoadedFromServer.value = false;
    credentials.revealingCredentials.value = false;
    providerPresetKey.value = "";
    providerCreateInspectFingerprint.value = "";
    providerEditIdentityAtOpen.value = "";
    providerStep.value = 1;
    providerAddMode.value = "catalog";
    catalog.catalogProviderId.value = "";
    catalog.catalogModels.value = [];
  }

  async function populateProviderForm(row: PlatformResource) {
    const config = getConfig(row);
    const isCatalogManaged =
      config.catalog_managed === true ||
      config.catalog_source === "models.dev" ||
      config.metadata_source === "models.dev";
    providerAddMode.value = isCatalogManaged ? "catalog" : "custom";
    providerPresetKey.value = findProviderPreset(row.provider)?.key || "";
    await ensureProviderMigrationMap();
    const resolvedCatalogId = catalogProviderIdFor(row.provider);
    catalog.catalogProviderId.value = isCatalogManaged ? resolvedCatalogId : "";
    Object.assign(providerForm, {
      provider_type: normalizeProviderType(config.provider_type),
      variant: config.variant || "openai",
      model_api_id: row.model,
      provider_code: row.provider,
      provider_display_name: config.provider_display_name || row.provider,
      model_display_name: row.name,
      api_base_url: config.api_base_url || "",
      api_key: "",
      api_key_set: Boolean(config.api_key_set),
      secret_id: config.secret_id || "",
      secret_key: "",
      aws_region: config.aws_region || "",
      enabled: row.enabled,
      model_category: getCategories(row),
      model_size_label: config.model_size_label || "",
      context_window_k: toNullableNumber(config.context_window_k),
      max_output_tokens: toNumber(config.max_output_tokens, 4096),
      model_rating: toNumber(config.model_rating, 60),
      input_price_micro_usd_per_1k: toNumber(config.input_price_micro_usd_per_1k, 0),
      output_price_micro_usd_per_1k: toNumber(config.output_price_micro_usd_per_1k, 0),
      cached_input_price_micro_usd_per_1k: toNumber(config.cached_input_price_micro_usd_per_1k, 0),
      reasoning_price_micro_usd_per_1k: toNumber(config.reasoning_price_micro_usd_per_1k, 0),
      embedding_price_micro_usd_per_1k: toNumber(config.embedding_price_micro_usd_per_1k, 0),
      capability_chips: Array.isArray(config.capability_chips) ? config.capability_chips : [],
      catalog_managed: Boolean(config.catalog_managed),
      catalog_source: config.catalog_source || config.metadata_source || "",
      raw_metadata_json: config.raw_metadata_json || "",
      metadata_source: config.metadata_source || "",
      sort_order: row.sort_order,
      description: row.description,
      enable_token_tailoring: config.enable_token_tailoring !== false,
      optimize_for_cache: Boolean(config.optimize_for_cache),
      reasoning_backfill: config.reasoning_content_backfill !== false,
      show_tool_call_delta: Boolean(config.show_tool_call_delta),
      keep_alive_minutes: toNumber(config.keep_alive_minutes, 5),
      rate_limit_rpm: toNumber(config.rate_limit_rpm, 0)
    });
    credentials.loadUsdPricingFromConfig(config);
    if (resolvedCatalogId) {
      catalog.catalogProviderId.value = resolvedCatalogId;
      await catalog.ensureCatalogLoaded();
      await catalog.loadCatalogModels(resolvedCatalogId, "", true);
      catalog.applyProviderRuntimeFields(resolvedCatalogId);
    }
    Object.assign(providerHAForm, {
      haMode: (config.ha_mode || "") as ProviderHAForm["haMode"],
      haCandidates: (config.ha_candidates || []).map((c) => ({
        name: c.name || "",
        providerType: c.provider_type || "openai",
        baseUrl: c.base_url || "",
        apiKey: c.api_key || ""
      })),
      haHedgeDelayMs: toNumber(config.ha_hedge_delay_ms, 100)
    });
    providerCreateInspectFingerprint.value = "";
    if (row.provider && row.model) {
      providerEditIdentityAtOpen.value = `${row.provider}\0${row.model}`;
    } else {
      providerEditIdentityAtOpen.value = "";
    }
    providerStep.value = 1;
  }

  return {
    providerForm,
    providerHAForm,
    providerStep,
    providerAddMode,
    providerPresetKey,
    catalogProviderId: catalog.catalogProviderId,
    catalogProviderSearch: catalog.catalogProviderSearch,
    catalogProviders: catalog.catalogProviders,
    catalogModels: catalog.catalogModels,
    catalogModelFilterLocal: catalog.catalogModelFilterLocal,
    catalogModelTotal: catalog.catalogModelTotal,
    catalogLoading: catalog.catalogLoading,
    catalogModelsLoading: catalog.catalogModelsLoading,
    providerCreateInspectFingerprint,
    providerEditIdentityAtOpen,
    checkingModel,
    showApiKey: credentials.showApiKey,
    showSecretKey: credentials.showSecretKey,
    revealingCredentials: credentials.revealingCredentials,
    credentialsLoadedFromServer: credentials.credentialsLoadedFromServer,
    categoryOptions,
    providerTypeOptions,
    providerTypeFilterOptions,
    variantOptions,
    currentAuthType,
    isLocalProviderModel,
    canInspectProviderModel,
    apiKeyFieldHint,
    apiKeyMaskedPlaceholder,
    secretKeyMaskedPlaceholder,
    providerPresetOptions,
    currentProviderPreset,
    catalogProviderOptions: catalog.catalogProviderOptions,
    catalogProviderHint: catalog.catalogProviderHint,
    catalogProviderDocUrl: catalog.catalogProviderDocUrl,
    catalogModelsHint: catalog.catalogModelsHint,
    selectedCatalogModelLabel: catalog.selectedCatalogModelLabel,
    activeCatalogProviderId: catalog.activeCatalogProviderId,
    useCatalogModelPicker: catalog.useCatalogModelPicker,
    providerRuntimeLocked,
    providerRuntimeSummary,
    providerModelOptions: catalog.providerModelOptions,
    dialogTitle,
    dialogSubtitle,
    showPricingWarning,
    providerIdentityChanged,
    providerRuntimeBindingPreview,
    canSubmitNewProviderModel,
    catalogPricingMissing: catalog.catalogPricingMissing,
    ensureCatalogLoaded: catalog.ensureCatalogLoaded,
    reloadCatalogProviders: catalog.reloadCatalogProviders,
    loadCatalogModels: catalog.loadCatalogModels,
    filterCatalogModelsLocal: catalog.filterCatalogModelsLocal,
    applyCatalogProvider: catalog.applyCatalogProvider,
    applyCatalogModel: catalog.applyCatalogModel,
    setProviderAddMode,
    applyProviderPreset,
    applyModelPreset,
    setCustomModelValue,
    inspectCurrentProviderModel,
    saveProviderRow,
    buildProviderPayload,
    normalizeProviderType,
    toggleApiKeyVisibility: credentials.toggleApiKeyVisibility,
    toggleSecretKeyVisibility: credentials.toggleSecretKeyVisibility,
    resetProviderForm,
    populateProviderForm,
    providerCodeRule,
    metadataLabel,
  };
}
