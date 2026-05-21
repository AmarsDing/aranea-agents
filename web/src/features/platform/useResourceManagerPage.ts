import { computed, onMounted, reactive, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useQuasar, type QTableColumn } from "quasar";
import {
  createPlatformResource,
  deletePlatformResource,
  inspectProviderModel,
  revealProviderModelCredentials,
  listPlatformResources,
  updatePlatformResource,
  type PlatformResource,
  type PlatformResourceInput,
  type PlatformResourceName
} from "./api";
import ProviderHAConfig, { type ProviderHAForm } from "../../components/platform/ProviderHAConfig.vue";
import {
  PROVIDER_PRESETS,
  PROVIDER_TYPE_OPTIONS,
  VARIANT_OPTIONS,
  findModelPreset,
  findProviderPreset,
  type ProviderModelPreset
} from "../../config/providerPresets";
import {
  hasPricingConfigured,
  pricingWarningMessage,
} from "../usage/pricingWarning";

export function useResourceManagerPage() {
  const route = useRoute();
const $q = useQuasar();
const isDark = computed(() => $q.dark.isActive);

const rows = ref<PlatformResource[]>([]);
const loading = ref(false);
const saving = ref(false);
const checkingModel = ref(false);
const keyword = ref("");
const dialogOpen = ref(false);
const editingId = ref("");
const page = ref(1);
const rowsPerPage = ref(20);
const showApiKey = ref(false);
const showSecretKey = ref(false);
const revealingCredentials = ref(false);
const credentialsLoadedFromServer = ref(false);
const trendDialogOpen = ref(false);
const trendRow = ref<PlatformResource | null>(null);
const providerPresetKey = ref("");
const providerStep = ref(1);
const providerTypeFilter = ref<string[]>([]);
/** 新建 Provider：最近一次「检查」成功时的连接指纹；改动 code/model/base/key/type 后需重新检查 */
const providerCreateInspectFingerprint = ref("");

type ModelCategory = {
  value: string;
  label: string;
  tooltip: string;
};

type ProviderConfig = {
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
  raw_metadata_json?: string;
  metadata_source?: string;
  last_used_at?: string;
  model_rating?: number | string | null;
};

const categoryOptions: ModelCategory[] = [
  { value: "general", label: "通用对话", tooltip: "均衡，适合日常问答与轻任务" },
  { value: "reasoning", label: "推理 / 复杂问题", tooltip: "数学、逻辑、多步推导" },
  { value: "code", label: "代码", tooltip: "生成、解释、重构代码" },
  { value: "long_context", label: "长上下文", tooltip: "大文档、长会话摘要" },
  { value: "vision", label: "视觉 / 多模态", tooltip: "图像理解" },
  { value: "embedding", label: "向量嵌入", tooltip: "记忆、检索" },
  { value: "fast", label: "低延迟", tooltip: "优先响应速度" },
  { value: "creative", label: "创意写作", tooltip: "文案、故事、营销" }
];

const providerTypeOptions = PROVIDER_TYPE_OPTIONS;
const providerTypeFilterOptions = PROVIDER_TYPE_OPTIONS;
const variantOptions = VARIANT_OPTIONS;

const form = reactive<PlatformResourceInput>({
  key: "",
  name: "",
  description: "",
  enabled: true,
  sort_order: 0,
  parent_id: "",
  level: "",
  agent_id: "",
  provider: "",
  model: "",
  config_json: "{}",
  metadata_json: "{}"
});

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

const columns: QTableColumn<PlatformResource>[] = [
  { name: "name", field: "name", label: "Name", align: "left", sortable: true },
  { name: "key", field: "key", label: "Key", align: "left", sortable: true },
  { name: "provider", field: "provider", label: "Provider", align: "left" },
  { name: "model", field: "model", label: "Model", align: "left" },
  { name: "status", field: "status", label: "Status", align: "left" },
  { name: "actions", field: "id", label: "Actions", align: "right" }
];

const resource = computed(() => route.meta.resource as PlatformResourceName);
const isProviderResource = computed(() => resource.value === "llm-provider-models");
const pageTitle = computed(() => (route.meta.title as string) || "资源管理");
const pageSubtitle = computed(() => (route.meta.subtitle as string) || "管理平台资源、启用状态与运行配置。");
const currentAuthType = computed(() => currentProviderPreset.value?.authType || "api_key");

const filteredRows = computed(() => {
  let list = rows.value;
  if (isProviderResource.value && providerTypeFilter.value.length) {
    const allowed = new Set(providerTypeFilter.value.map((v) => v.toLowerCase()));
    list = list.filter((row) => allowed.has((getConfig(row).provider_type || "openai").toLowerCase()));
  }
  const q = keyword.value.trim().toLowerCase();
  if (!q) return list;
  return list.filter((row) =>
    [
      row.key,
      row.name,
      row.description,
      row.provider,
      row.model,
      row.agent_id,
      getConfig(row).provider_display_name,
      ...getCategories(row).map((category) => category.label)
    ].some((value) =>
      (value || "").toLowerCase().includes(q)
    )
  );
});
const pageCount = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / rowsPerPage.value)));
const pagedProviderRows = computed(() => {
  const start = (page.value - 1) * rowsPerPage.value;
  return filteredRows.value.slice(start, start + rowsPerPage.value);
});
const providerPresetOptions = computed(() =>
  PROVIDER_PRESETS.map((preset) => ({
    label: preset.label,
    value: preset.key,
    caption: `${preset.apiBaseUrl || "手动配置"} · ${metadataLabel(preset.metadataApi)}`
  }))
);
const currentProviderPreset = computed(() => findProviderPreset(providerPresetKey.value || providerForm.provider_code));

/** 本地 / 内网模型：检查列表通常可不填密钥（如 Ollama、localhost OpenAI 兼容） */
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
  if (editingId.value && providerForm.api_key_set) return true;
  return false;
});

const canInspectProviderModel = computed(() => {
  if (!providerForm.provider_code.trim() || !providerForm.model_api_id.trim()) return false;
  if (isLocalProviderModel.value) return true;
  return hasInspectApiKey.value;
});

const apiKeyFieldHint = computed(() => {
  const parts: string[] = [];
  if (editingId.value && providerForm.api_key_set && !providerForm.api_key.trim()) {
    parts.push("已保存密钥，点击眼睛图标查看");
  } else if (editingId.value) {
    parts.push("留空表示不修改");
  }
  if (!isLocalProviderModel.value) parts.push("远程 Provider 检查模型前需填写密钥");
  return parts.join("；") || undefined;
});

const apiKeyMaskedPlaceholder = computed(() => {
  if (editingId.value && providerForm.api_key_set && !providerForm.api_key.trim() && !showApiKey.value) {
    return "••••••••••••";
  }
  return undefined;
});

const secretKeyMaskedPlaceholder = computed(() => {
  if (editingId.value && providerForm.secret_id.trim() && !providerForm.secret_key.trim() && !showSecretKey.value) {
    return "••••••••••••";
  }
  return undefined;
});

function clearRevealedCredentialsFromForm() {
  if (credentialsLoadedFromServer.value) {
    providerForm.api_key = "";
    providerForm.secret_key = "";
    providerHAForm.haCandidates = providerHAForm.haCandidates.map((c) => ({ ...c, apiKey: "" }));
    credentialsLoadedFromServer.value = false;
  }
}

async function loadRevealedCredentials() {
  if (!editingId.value) return;
  revealingCredentials.value = true;
  try {
    const creds = await revealProviderModelCredentials(editingId.value);
    if (creds.api_key) providerForm.api_key = creds.api_key;
    if (creds.secret_key) providerForm.secret_key = creds.secret_key;
    for (const ha of creds.ha_candidates) {
      const idx = providerHAForm.haCandidates.findIndex((c) => c.name.trim() === ha.name.trim());
      if (idx >= 0 && ha.api_key) {
        providerHAForm.haCandidates[idx] = { ...providerHAForm.haCandidates[idx], apiKey: ha.api_key };
      }
    }
    credentialsLoadedFromServer.value = true;
  } catch (error) {
    $q.notify({ type: "negative", message: errorMessage(error) });
    throw error;
  } finally {
    revealingCredentials.value = false;
  }
}

async function toggleApiKeyVisibility() {
  if (!showApiKey.value && editingId.value && providerForm.api_key_set && !providerForm.api_key.trim()) {
    try {
      await loadRevealedCredentials();
      showApiKey.value = true;
    } catch {
      /* notified */
    }
    return;
  }
  if (showApiKey.value) {
    clearRevealedCredentialsFromForm();
    showApiKey.value = false;
    return;
  }
  showApiKey.value = true;
}

async function toggleSecretKeyVisibility() {
  if (!showSecretKey.value && editingId.value && providerForm.secret_id.trim() && !providerForm.secret_key.trim()) {
    try {
      await loadRevealedCredentials();
      showSecretKey.value = true;
    } catch {
      /* notified */
    }
    return;
  }
  if (showSecretKey.value) {
    if (credentialsLoadedFromServer.value) {
      providerForm.secret_key = "";
      if (!showApiKey.value) {
        providerForm.api_key = "";
        providerHAForm.haCandidates = providerHAForm.haCandidates.map((c) => ({ ...c, apiKey: "" }));
        credentialsLoadedFromServer.value = false;
      }
    }
    showSecretKey.value = false;
    return;
  }
  showSecretKey.value = true;
}

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

const showPricingWarning = computed(
  () =>
    isProviderResource.value &&
    !hasPricingConfigured({
      inputPrice: providerForm.input_price_micro_usd_per_1k,
      outputPrice: providerForm.output_price_micro_usd_per_1k,
      inputPriceCached: providerForm.cached_input_price_micro_usd_per_1k,
      outputPriceReasoning: providerForm.reasoning_price_micro_usd_per_1k,
    })
);

const canSubmitNewProviderModel = computed(() => {
  if (!isProviderResource.value) return true;
  if (editingId.value) return true;
  const saved = providerCreateInspectFingerprint.value;
  if (!saved) return false;
  return saved === providerCreateInspectFingerprintValue();
});

const providerModelOptions = computed(() =>
  (currentProviderPreset.value?.models ?? []).map((model) => ({
    label: model.label,
    value: model.id,
    caption: `${model.id}${model.contextWindowK ? ` · ${model.contextWindowK}K ctx` : ""}`
  }))
);
const dialogTitle = computed(() => {
  if (!isProviderResource.value) return editingId.value ? "编辑资源" : "新增资源";
  return editingId.value ? "编辑Provider" : "添加Provider";
});
const dialogSubtitle = computed(() => {
  if (!isProviderResource.value) return "Key 和 Name 为必填，其他字段按模块需要填写。";
  if (!editingId.value) return "配置 LLM Provider 连接。新建需先点击「检查」并通过验证后再创建。";
  return "配置 LLM Provider 连接";
});

onMounted(loadRows);

watch(resource, () => {
  keyword.value = "";
  page.value = 1;
  void loadRows();
});

watch(filteredRows, () => {
  if (page.value > pageCount.value) page.value = pageCount.value;
});

async function loadRows() {
  if (!resource.value) return;
  loading.value = true;
  try {
    rows.value = await listPlatformResources(resource.value);
  } finally {
    loading.value = false;
  }
}

function resetForm() {
  Object.assign(form, {
    key: "",
    name: "",
    description: "",
    enabled: true,
    sort_order: 0,
    parent_id: "",
    level: "",
    agent_id: "",
    provider: "",
    model: "",
    config_json: "{}",
    metadata_json: "{}"
  });
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
  showApiKey.value = false;
  showSecretKey.value = false;
  credentialsLoadedFromServer.value = false;
  revealingCredentials.value = false;
  providerPresetKey.value = "";
  providerCreateInspectFingerprint.value = "";
  providerStep.value = 1;
}

function openCreate() {
  editingId.value = "";
  resetForm();
  dialogOpen.value = true;
}

function openEdit(row: PlatformResource) {
  editingId.value = row.id;
  Object.assign(form, {
    key: row.key,
    name: row.name,
    description: row.description,
    enabled: row.enabled,
    sort_order: row.sort_order,
    parent_id: row.parent_id,
    level: row.level,
    agent_id: row.agent_id,
    provider: row.provider,
    model: row.model,
    config_json: row.config_json || "{}",
    metadata_json: row.metadata_json || "{}"
  });
  if (isProviderResource.value) {
    const config = getConfig(row);
    providerPresetKey.value = findProviderPreset(row.provider)?.key || "";
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
  }
  providerCreateInspectFingerprint.value = "";
  providerStep.value = 1;
  dialogOpen.value = true;
}

async function saveRow() {
  if (isProviderResource.value) {
    await saveProviderRow();
    return;
  }
  if (!form.key || !form.name) {
    $q.notify({ type: "negative", message: "Key 和 Name 必填" });
    return;
  }
  saving.value = true;
  try {
    if (editingId.value) {
      const updated = await updatePlatformResource(resource.value, editingId.value, form);
      rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
    } else {
      const created = await createPlatformResource(resource.value, form);
      rows.value = [created, ...rows.value];
    }
    dialogOpen.value = false;
    $q.notify({ type: "positive", message: "已保存" });
  } finally {
    saving.value = false;
  }
}

async function saveProviderRow() {
  const code = providerForm.provider_code.trim();
  const model = providerForm.model_api_id.trim();
  if (!code || !model || !isProviderCodeValid(code)) {
    $q.notify({ type: "negative", message: "Provider 名称和模型ID必填，名称仅支持小写字母、数字、连字符" });
    return;
  }
  if (!editingId.value && !canSubmitNewProviderModel.value) {
    $q.notify({ type: "warning", message: "请先点击「检查」并通过验证后再创建" });
    return;
  }

  const payload = buildProviderPayload();
  saving.value = true;
  try {
    if (editingId.value) {
      const updated = await updatePlatformResource(resource.value, editingId.value, payload);
      rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
    } else {
      const created = await createPlatformResource(resource.value, payload);
      rows.value = [created, ...rows.value];
    }
    dialogOpen.value = false;
    $q.notify({ type: "positive", message: "已保存" });
  } finally {
    saving.value = false;
  }
}

function applyProviderPreset(key: string) {
  const preset = findProviderPreset(key);
  if (!preset) return;
  providerPresetKey.value = preset.key;
  providerForm.provider_type = preset.providerType;
  providerForm.variant = preset.variant || "openai";
  providerForm.provider_code = preset.providerCode;
  providerForm.provider_display_name = preset.label;
  providerForm.api_base_url = preset.apiBaseUrl;
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
  providerForm.input_price_micro_usd_per_1k = overwrite || !providerForm.input_price_micro_usd_per_1k ? preset.inputPriceMicroUsdPer1K ?? providerForm.input_price_micro_usd_per_1k : providerForm.input_price_micro_usd_per_1k;
  providerForm.output_price_micro_usd_per_1k = overwrite || !providerForm.output_price_micro_usd_per_1k ? preset.outputPriceMicroUsdPer1K ?? providerForm.output_price_micro_usd_per_1k : providerForm.output_price_micro_usd_per_1k;
  providerForm.cached_input_price_micro_usd_per_1k = overwrite || !providerForm.cached_input_price_micro_usd_per_1k ? preset.cachedInputPriceMicroUsdPer1K ?? providerForm.cached_input_price_micro_usd_per_1k : providerForm.cached_input_price_micro_usd_per_1k;
  providerForm.reasoning_price_micro_usd_per_1k = overwrite || !providerForm.reasoning_price_micro_usd_per_1k ? preset.reasoningPriceMicroUsdPer1K ?? providerForm.reasoning_price_micro_usd_per_1k : providerForm.reasoning_price_micro_usd_per_1k;
  providerForm.embedding_price_micro_usd_per_1k = overwrite || !providerForm.embedding_price_micro_usd_per_1k ? preset.embeddingPriceMicroUsdPer1K ?? providerForm.embedding_price_micro_usd_per_1k : providerForm.embedding_price_micro_usd_per_1k;
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
    const result = await inspectProviderModel({
      resource_id: editingId.value,
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
      if (!editingId.value) {
        providerCreateInspectFingerprint.value = "";
      }
      const preset = findModelPreset(providerPresetKey.value || code, model);
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
    if (!editingId.value) {
      providerCreateInspectFingerprint.value = providerCreateInspectFingerprintValue();
    }
    providerForm.provider_type = result.provider_type || providerForm.provider_type;
    if (result.variant) providerForm.variant = result.variant;
    if (result.enable_token_tailoring) providerForm.enable_token_tailoring = true;
    providerForm.model_display_name = result.model_display_name || providerForm.model_display_name || model;
    providerForm.model_size_label = result.model_size_label || providerForm.model_size_label;
    providerForm.context_window_k = result.context_window_k || providerForm.context_window_k;
    providerForm.max_output_tokens = result.max_output_tokens || providerForm.max_output_tokens;
    providerForm.input_price_micro_usd_per_1k = result.input_price_micro_usd_per_1k || 0;
    providerForm.output_price_micro_usd_per_1k = result.output_price_micro_usd_per_1k || 0;
    providerForm.cached_input_price_micro_usd_per_1k = result.cached_input_price_micro_usd_per_1k || 0;
    providerForm.reasoning_price_micro_usd_per_1k = result.reasoning_price_micro_usd_per_1k || 0;
    providerForm.embedding_price_micro_usd_per_1k = result.embedding_price_micro_usd_per_1k || 0;
    providerForm.raw_metadata_json = result.raw_metadata_json || "";
    providerForm.metadata_source = result.source || "";
    $q.notify({ type: "positive", message: result.message || "已获取模型参数" });
  } catch (error) {
    if (!editingId.value) {
      providerCreateInspectFingerprint.value = "";
    }
    $q.notify({ type: "negative", message: errorMessage(error) });
  } finally {
    checkingModel.value = false;
  }
}

async function toggleEnabled(row: PlatformResource, enabled: boolean) {
  saving.value = true;
  try {
    const updated = await updatePlatformResource(resource.value, row.id, { ...row, enabled });
    rows.value = rows.value.map((item) => (item.id === updated.id ? updated : item));
  } finally {
    saving.value = false;
  }
}

function confirmRemoveRow(row: PlatformResource) {
  $q.dialog({
    title: "确认删除",
    message: `确定删除「${row.name}」吗？`,
    cancel: true,
    persistent: true
  }).onOk(() => {
    void removeRow(row);
  });
}

function openTrend(row: PlatformResource) {
  trendRow.value = row;
  trendDialogOpen.value = true;
}

async function removeRow(row: PlatformResource) {
  await deletePlatformResource(resource.value, row.id);
  rows.value = rows.value.filter((item) => item.id !== row.id);
  $q.notify({ type: "positive", message: "已删除" });
}

function buildProviderPayload(): PlatformResourceInput {
  const editingRow = editingId.value ? rows.value.find((row) => row.id === editingId.value) : undefined;
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
    input_price_micro_usd_per_1k: providerForm.input_price_micro_usd_per_1k,
    output_price_micro_usd_per_1k: providerForm.output_price_micro_usd_per_1k,
    cached_input_price_micro_usd_per_1k: providerForm.cached_input_price_micro_usd_per_1k,
    reasoning_price_micro_usd_per_1k: providerForm.reasoning_price_micro_usd_per_1k,
    embedding_price_micro_usd_per_1k: providerForm.embedding_price_micro_usd_per_1k,
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

function getConfig(row: PlatformResource): ProviderConfig {
  if (!row.config_json) return {};
  try {
    const value = JSON.parse(row.config_json) as ProviderConfig;
    return value && typeof value === "object" ? value : {};
  } catch {
    return {};
  }
}

function getCategories(row: PlatformResource): ModelCategory[] {
  const categories = getConfig(row).model_category;
  if (!Array.isArray(categories)) return [];
  return categories.filter((category) => category?.value && category?.label && category?.tooltip);
}

function toNullableNumber(value: unknown) {
  if (value === "" || value === null || value === undefined) return null;
  const numberValue = Number(value);
  return Number.isFinite(numberValue) ? numberValue : null;
}

function toNumber(value: unknown, fallback: number) {
  const numberValue = toNullableNumber(value);
  return numberValue === null ? fallback : numberValue;
}

function providerCodeRule(value: string) {
  return isProviderCodeValid(value) || "仅支持小写字母、数字、连字符";
}

function isProviderCodeValid(value: string) {
  return /^[a-z0-9-]+$/.test(value);
}

function metadataLabel(value: string) {
  if (value === "full") return "可查询参数";
  if (value === "partial") return "可验证模型";
  if (value === "limited") return "有限查询";
  return "手动维护";
}

function errorMessage(error: unknown) {
  if (typeof error === "object" && error && "response" in error) {
    const response = (error as { response?: { data?: { error?: string } } }).response;
    if (response?.data?.error) return response.data.error;
  }
  return error instanceof Error ? error.message : "模型检查失败";
}

  return {
    isDark,
    rows,
    loading,
    saving,
    checkingModel,
    keyword,
    dialogOpen,
    editingId,
    page,
    rowsPerPage,
    showApiKey,
    showSecretKey,
    revealingCredentials,
    trendDialogOpen,
    trendRow,
    providerPresetKey,
    providerStep,
    providerTypeFilter,
    categoryOptions,
    providerTypeFilterOptions,
    variantOptions,
    form,
    providerForm,
    providerHAForm,
    columns,
    isProviderResource,
    pageTitle,
    pageSubtitle,
    filteredRows,
    pageCount,
    pagedProviderRows,
    providerPresetOptions,
    currentAuthType,
    isLocalProviderModel,
    canInspectProviderModel,
    apiKeyFieldHint,
    apiKeyMaskedPlaceholder,
    secretKeyMaskedPlaceholder,
    showPricingWarning,
    canSubmitNewProviderModel,
    providerModelOptions,
    dialogTitle,
    dialogSubtitle,
    pricingWarningMessage,
    openCreate,
    openEdit,
    saveRow,
    applyProviderPreset,
    setCustomModelValue,
    inspectCurrentProviderModel,
    toggleApiKeyVisibility,
    toggleSecretKeyVisibility,
    toggleEnabled,
    confirmRemoveRow,
    openTrend,
    providerCodeRule,
    getCategories,
    metadataLabel,
  };
}

