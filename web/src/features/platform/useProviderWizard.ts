import { computed, reactive, ref, type ComputedRef, type Ref } from 'vue';
import { useQuasar } from 'quasar';
import type { PlatformResource, PlatformResourceName, ModelCategory, CapabilityChip } from './types';
import { errorMessage, toNullableNumber, toNumber, getConfig, getCategories } from './providerUtils';
import { usePlatformStore } from '../../stores/platform';
import type { ProviderHAForm } from './types';
import {
  PROVIDER_RUNTIME_OVERLAY,
  PROVIDER_TYPE_OPTIONS,
  VARIANT_OPTIONS,
  runtimeProfileFor,
  microPer1KToUsdPer1M,
} from '../../config/providerRuntimeOverlay';
import { MODEL_CATEGORY_OPTIONS } from '../model-catalog/catalogCategories';
import { catalogProviderIdFor, ensureProviderMigrationMap } from '../model-catalog/providerMigration';
import { hasPricingConfigured } from '../usage/pricingWarning';
import { findProviderPreset, findModelPreset, type ProviderModelPreset } from '../../config/providerPresets';
import { useProviderCatalog } from './useProviderCatalog';
import { useProviderCredentials } from './useProviderCredentials';
import { useProviderInspect } from './useProviderInspect';
import { useProviderPresets } from './useProviderPresets';
import { useProviderSave } from './useProviderSave';

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
    provider_type: 'openai',
    variant: 'openai',
    model_api_id: '',
    provider_code: '',
    provider_display_name: '',
    model_display_name: '',
    api_base_url: '',
    api_key: '',
    api_key_set: false,
    secret_id: '',
    secret_key: '',
    aws_region: '',
    enabled: true,
    model_category: [] as ModelCategory[],
    model_size_label: '',
    context_window_k: null as number | null,
    max_output_tokens: 4096,
    model_rating: 60,
    input_price_usd_per_1m: 0,
    output_price_usd_per_1m: 0,
    cache_read_usd_per_1m: 0,
    cache_write_usd_per_1m: 0,
    reasoning_price_usd_per_1m: 0,
    embedding_price_usd_per_1m: 0,
    capability_chips: [] as CapabilityChip[],
    catalog_managed: false,
    catalog_source: '',
    raw_metadata_json: '',
    metadata_source: '',
    sort_order: 0,
    description: '',
    enable_token_tailoring: true,
    optimize_for_cache: false,
    reasoning_backfill: true,
    show_tool_call_delta: false,
    keep_alive_minutes: 5,
    rate_limit_rpm: 0,
  });

  const providerHAForm = reactive<ProviderHAForm>({
    haMode: '',
    haCandidates: [],
    haHedgeDelayMs: 100,
  });

  const providerStep = ref(1);
  const providerAddMode = ref<'catalog' | 'custom'>('catalog');
  const providerCreateInspectFingerprint = ref('');
  const providerEditIdentityAtOpen = ref('');

  const categoryOptions: ModelCategory[] = MODEL_CATEGORY_OPTIONS;
  const providerTypeOptions = PROVIDER_TYPE_OPTIONS;
  const providerTypeFilterOptions = PROVIDER_TYPE_OPTIONS;
  const haCandidateProviderTypeOptions = PROVIDER_TYPE_OPTIONS;
  const variantOptions = VARIANT_OPTIONS;

  function applyModelPresetValues(preset: ProviderModelPreset, overwrite = false) {
    const f = providerForm;
    f.model_display_name = overwrite || !f.model_display_name ? preset.label : f.model_display_name;
    f.model_size_label = overwrite || !f.model_size_label ? preset.sizeLabel || f.model_size_label : f.model_size_label;
    f.context_window_k =
      overwrite || !f.context_window_k ? (preset.contextWindowK ?? f.context_window_k) : f.context_window_k;
    f.max_output_tokens =
      overwrite || !f.max_output_tokens ? (preset.maxOutputTokens ?? f.max_output_tokens) : f.max_output_tokens;
    if (overwrite || !f.input_price_usd_per_1m) {
      f.input_price_usd_per_1m = microPer1KToUsdPer1M(preset.inputPriceMicroUsdPer1K ?? 0);
    }
    if (overwrite || !f.output_price_usd_per_1m) {
      f.output_price_usd_per_1m = microPer1KToUsdPer1M(preset.outputPriceMicroUsdPer1K ?? 0);
    }
    if (overwrite || !f.cache_read_usd_per_1m) {
      f.cache_read_usd_per_1m = 0;
    }
    if (overwrite || !f.reasoning_price_usd_per_1m) {
      f.reasoning_price_usd_per_1m = 0;
    }
    if (overwrite || !f.embedding_price_usd_per_1m) {
      f.embedding_price_usd_per_1m = 0;
    }
  }

  const presets = useProviderPresets({
    providerForm,
    applyModelPresetValues,
  });

  const catalog = useProviderCatalog({
    providerForm,
    providerAddMode,
    providerCreateInspectFingerprint,
    editingId: deps.editingId,
    dialogOpen: deps.dialogOpen,
    isProviderResource: deps.isProviderResource,
    currentProviderPreset: presets.currentProviderPreset,
  });

  const credentials = useProviderCredentials({
    providerForm,
    providerHAForm,
    editingId: deps.editingId,
    isProviderResource: deps.isProviderResource,
  });

  const inspect = useProviderInspect({
    providerForm,
    providerAddMode,
    providerCreateInspectFingerprint,
    providerEditIdentityAtOpen,
    editingId: deps.editingId,
    isProviderResource: deps.isProviderResource,
    currentProviderPreset: presets.currentProviderPreset,
    applyModelPresetValues,
    findCatalogModel: (modelId: string) => catalog.catalogModels.value.find((m) => m.id === modelId),
    applyCatalogModel: catalog.applyCatalogModel,
    catalogProviderId: catalog.catalogProviderId,
    catalogModels: catalog.catalogModels,
  });

  const currentAuthType = computed(() => {
    if (providerAddMode.value === 'catalog' && catalog.catalogProviderId.value) {
      return runtimeProfileFor(catalog.catalogProviderId.value).authType;
    }
    return (
      presets.currentProviderPreset.value?.authType ||
      runtimeProfileFor(providerForm.provider_code).authType ||
      'api_key'
    );
  });

  const apiKeyFieldHint = computed(() => {
    const parts: string[] = [];
    if (deps.editingId.value && providerForm.api_key_set && !providerForm.api_key.trim()) {
      parts.push('已保存密钥，点击眼睛图标查看');
    } else if (deps.editingId.value) {
      parts.push('留空表示不修改');
    }
    if (!inspect.isLocalProviderModel.value) parts.push('远程 Provider 检查模型前需填写密钥');
    return parts.join('；') || '';
  });

  const apiKeyMaskedPlaceholder = computed(() => {
    if (
      deps.editingId.value &&
      providerForm.api_key_set &&
      !providerForm.api_key.trim() &&
      !credentials.showApiKey.value
    ) {
      return '••••••••••••';
    }
    return '';
  });

  const secretKeyMaskedPlaceholder = computed(() => {
    if (
      deps.editingId.value &&
      providerForm.secret_id.trim() &&
      !providerForm.secret_key.trim() &&
      !credentials.showSecretKey.value
    ) {
      return '••••••••••••';
    }
    return '';
  });

  const providerRuntimeLocked = computed(() => {
    const code = catalog.activeCatalogProviderId.value;
    if (!code) return providerAddMode.value === 'catalog';
    if (providerAddMode.value === 'catalog' && catalog.catalogProviderId.value) return true;
    return catalog.catalogModels.value.length > 0 || Boolean(PROVIDER_RUNTIME_OVERLAY[code]);
  });

  const providerRuntimeSummary = computed(() => {
    const rt = runtimeProfileFor(catalog.activeCatalogProviderId.value || providerForm.provider_code.trim());
    const typeLabel = PROVIDER_TYPE_OPTIONS.find((o) => o.value === rt.providerType)?.label || rt.providerType;
    if (rt.providerType === 'openai' && rt.variant) {
      const variantLabel = VARIANT_OPTIONS.find((o) => o.value === rt.variant)?.label || rt.variant;
      return `${typeLabel} · ${variantLabel}`;
    }
    return typeLabel;
  });

  const dialogTitle = computed(() => {
    if (!deps.isProviderResource.value) return deps.editingId.value ? '编辑资源' : '新增资源';
    return deps.editingId.value ? '编辑Provider' : '添加Provider';
  });

  const dialogSubtitle = computed(() => {
    if (!deps.isProviderResource.value) return 'Key 和 Name 为必填，其他字段按模块需要填写。';
    if (!deps.editingId.value) {
      return providerAddMode.value === 'catalog'
        ? '从 models.dev 目录选择 Provider 与模型，规格与定价自动回填；远程 Provider 建议检查连通性。'
        : '配置 LLM Provider 连接。自定义模式需先点击「检查」并通过验证后再创建。';
    }
    return '配置 LLM Provider 连接';
  });

  const showPricingWarning = computed(
    () =>
      deps.isProviderResource.value &&
      !hasPricingConfigured({
        inputPrice: providerForm.input_price_usd_per_1m,
        outputPrice: providerForm.output_price_usd_per_1m,
        inputPriceCached: providerForm.cache_read_usd_per_1m,
        outputPriceReasoning: providerForm.reasoning_price_usd_per_1m,
        embeddingPrice: providerForm.embedding_price_usd_per_1m,
        cacheWritePrice: providerForm.cache_write_usd_per_1m,
      }),
  );

  const providerRuntimeBindingPreview = computed(() => {
    const code = providerForm.provider_code.trim();
    const model = providerForm.model_api_id.trim();
    if (!code || !model) return '';
    return `Agent / 运行时将使用：${code} / ${model}`;
  });

  function setProviderAddMode(mode: 'catalog' | 'custom') {
    providerAddMode.value = mode;
    providerCreateInspectFingerprint.value = '';
    if (mode === 'custom') {
      catalog.catalogProviderId.value = '';
      catalog.catalogModels.value = [];
      providerForm.catalog_managed = false;
      providerForm.catalog_source = 'custom';
      providerForm.metadata_source = 'custom';
      providerForm.capability_chips = [];
      return;
    }
    providerForm.catalog_source = 'models.dev';
    void catalog.ensureCatalogLoaded();
  }

  function normalizeProviderType(raw: string | undefined): string {
    const v = (raw || '').trim().toLowerCase();
    if (v === 'anthropic') return 'anthropic';
    if (v === 'gemini' || v === 'google gemini') return 'gemini';
    if (v === 'ollama') return 'ollama';
    if (v === 'hunyuan') return 'hunyuan';
    if (v === 'huggingface') return 'huggingface';
    if (v === 'bedrock') return 'bedrock';
    return 'openai';
  }

  function isProviderCodeValid(value: string) {
    return /^[a-z0-9-]+$/.test(value);
  }

  function providerCodeRule(value: string) {
    return isProviderCodeValid(value) || '仅支持小写字母、数字、连字符';
  }

  const save = useProviderSave({
    editingId: deps.editingId,
    dialogOpen: deps.dialogOpen,
    saving: deps.saving,
    resource: deps.resource,
    isProviderResource: deps.isProviderResource,
    rows: deps.rows,
    providerForm,
    providerHAForm,
    providerAddMode,
    canSubmitNewProviderModel: inspect.canSubmitNewProviderModel,
    providerIdentityChanged: inspect.providerIdentityChanged,
    isProviderCodeValid,
  });

  async function applyProviderPreset(key: string) {
    const preset = findProviderPreset(key);
    if (!preset) return;
    presets.providerPresetKey.value = preset.key;
    providerForm.provider_code = preset.providerCode;
    providerForm.provider_display_name = preset.label;
    providerForm.api_base_url = preset.apiBaseUrl;
    catalog.applyProviderRuntimeFields(preset.providerCode);
    catalog.catalogProviderId.value = preset.providerCode;
    catalog.catalogModelFilterLocal.value = '';
    await catalog.loadCatalogModels(preset.providerCode, '', true);
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
      presets.applyModelPreset(preset.models[0].id);
    } else {
      presets.applyModelPreset(providerForm.model_api_id);
    }
  }

  function resetProviderForm() {
    Object.assign(providerForm, {
      provider_type: 'openai',
      variant: 'openai',
      model_api_id: '',
      provider_code: '',
      provider_display_name: '',
      model_display_name: '',
      api_base_url: '',
      api_key: '',
      api_key_set: false,
      secret_id: '',
      secret_key: '',
      aws_region: '',
      enabled: true,
      model_category: [],
      model_size_label: '',
      context_window_k: null,
      max_output_tokens: 4096,
      model_rating: 60,
      input_price_usd_per_1m: 0,
      output_price_usd_per_1m: 0,
      cache_read_usd_per_1m: 0,
      cache_write_usd_per_1m: 0,
      reasoning_price_usd_per_1m: 0,
      embedding_price_usd_per_1m: 0,
      capability_chips: [],
      catalog_managed: false,
      catalog_source: '',
      raw_metadata_json: '',
      metadata_source: '',
      sort_order: 0,
      description: '',
      enable_token_tailoring: true,
      optimize_for_cache: false,
      reasoning_backfill: true,
      show_tool_call_delta: false,
      keep_alive_minutes: 5,
      rate_limit_rpm: 0,
    });
    Object.assign(providerHAForm, { haMode: '', haCandidates: [], haHedgeDelayMs: 100 });
    credentials.showApiKey.value = false;
    credentials.showSecretKey.value = false;
    credentials.credentialsLoadedFromServer.value = false;
    credentials.revealingCredentials.value = false;
    presets.providerPresetKey.value = '';
    providerCreateInspectFingerprint.value = '';
    providerEditIdentityAtOpen.value = '';
    providerStep.value = 1;
    providerAddMode.value = 'catalog';
    catalog.catalogProviderId.value = '';
    catalog.catalogModels.value = [];
  }

  async function populateProviderForm(row: PlatformResource) {
    const config = getConfig(row);
    const isCatalogManaged =
      config.catalog_managed === true ||
      config.catalog_source === 'models.dev' ||
      config.metadata_source === 'models.dev';
    providerAddMode.value = isCatalogManaged ? 'catalog' : 'custom';
    presets.providerPresetKey.value = findProviderPreset(row.provider)?.key || '';
    await ensureProviderMigrationMap();
    const resolvedCatalogId = catalogProviderIdFor(row.provider);
    catalog.catalogProviderId.value = isCatalogManaged ? resolvedCatalogId : '';
    Object.assign(providerForm, {
      provider_type: normalizeProviderType(config.provider_type),
      variant: config.variant || 'openai',
      model_api_id: row.model,
      provider_code: row.provider,
      provider_display_name: config.provider_display_name || row.provider,
      model_display_name: row.name,
      api_base_url: config.api_base_url || '',
      api_key: '',
      api_key_set: Boolean(config.api_key_set),
      secret_id: config.secret_id || '',
      secret_key: '',
      aws_region: config.aws_region || '',
      enabled: row.enabled,
      model_category: getCategories(row),
      model_size_label: config.model_size_label || '',
      context_window_k: toNullableNumber(config.context_window_k),
      max_output_tokens: toNumber(config.max_output_tokens, 4096),
      model_rating: toNumber(config.model_rating, 60),
      capability_chips: Array.isArray(config.capability_chips) ? config.capability_chips : [],
      catalog_managed: Boolean(config.catalog_managed),
      catalog_source: config.catalog_source || config.metadata_source || '',
      raw_metadata_json: config.raw_metadata_json || '',
      metadata_source: config.metadata_source || '',
      sort_order: row.sort_order,
      description: row.description,
      enable_token_tailoring: config.enable_token_tailoring !== false,
      optimize_for_cache: Boolean(config.optimize_for_cache),
      reasoning_backfill: config.reasoning_content_backfill !== false,
      show_tool_call_delta: Boolean(config.show_tool_call_delta),
      keep_alive_minutes: toNumber(config.keep_alive_minutes, 5),
      rate_limit_rpm: toNumber(config.rate_limit_rpm, 0),
    });
    credentials.loadUsdPricingFromConfig(config);
    if (resolvedCatalogId) {
      catalog.catalogProviderId.value = resolvedCatalogId;
      await catalog.ensureCatalogLoaded();
      await catalog.loadCatalogModels(resolvedCatalogId, '', true);
      catalog.applyProviderRuntimeFields(resolvedCatalogId);
    }
    Object.assign(providerHAForm, {
      haMode: (config.ha_mode || '') as ProviderHAForm['haMode'],
      haCandidates: (config.ha_candidates || []).map((c) => ({
        name: c.name || '',
        providerType: c.provider_type || 'openai',
        baseUrl: c.base_url || '',
        apiKey: c.api_key || '',
      })),
      haHedgeDelayMs: toNumber(config.ha_hedge_delay_ms, 100),
    });
    providerCreateInspectFingerprint.value = '';
    if (row.provider && row.model) {
      providerEditIdentityAtOpen.value = `${row.provider}\0${row.model}`;
    } else {
      providerEditIdentityAtOpen.value = '';
    }
    providerStep.value = 1;
  }

  function updateHAForm(val: ProviderHAForm) {
    Object.assign(providerHAForm, val);
  }

  return {
    providerForm,
    providerHAForm,
    providerStep,
    providerAddMode,
    providerPresetKey: presets.providerPresetKey,
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
    checkingModel: inspect.checkingModel,
    showApiKey: credentials.showApiKey,
    showSecretKey: credentials.showSecretKey,
    revealingCredentials: credentials.revealingCredentials,
    credentialsLoadedFromServer: credentials.credentialsLoadedFromServer,
    categoryOptions,
    providerTypeOptions,
    providerTypeFilterOptions,
    haCandidateProviderTypeOptions,
    variantOptions,
    currentAuthType,
    isLocalProviderModel: inspect.isLocalProviderModel,
    canInspectProviderModel: inspect.canInspectProviderModel,
    apiKeyFieldHint,
    apiKeyMaskedPlaceholder,
    secretKeyMaskedPlaceholder,
    providerPresetOptions: presets.providerPresetOptions,
    currentProviderPreset: presets.currentProviderPreset,
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
    providerIdentityChanged: inspect.providerIdentityChanged,
    providerRuntimeBindingPreview,
    canSubmitNewProviderModel: inspect.canSubmitNewProviderModel,
    catalogPricingMissing: catalog.catalogPricingMissing,
    ensureCatalogLoaded: catalog.ensureCatalogLoaded,
    reloadCatalogProviders: catalog.reloadCatalogProviders,
    loadCatalogModels: catalog.loadCatalogModels,
    filterCatalogModelsLocal: catalog.filterCatalogModelsLocal,
    applyCatalogProvider: catalog.applyCatalogProvider,
    applyCatalogModel: catalog.applyCatalogModel,
    setProviderAddMode,
    applyProviderPreset,
    applyModelPreset: presets.applyModelPreset,
    setCustomModelValue: presets.setCustomModelValue,
    inspectCurrentProviderModel: inspect.inspectCurrentProviderModel,
    saveProviderRow: save.saveProviderRow,
    buildProviderPayload: save.buildProviderPayload,
    normalizeProviderType,
    toggleApiKeyVisibility: credentials.toggleApiKeyVisibility,
    toggleSecretKeyVisibility: credentials.toggleSecretKeyVisibility,
    resetProviderForm,
    populateProviderForm,
    updateHAForm,
    providerCodeRule,
    metadataLabel: presets.metadataLabel,
  };
}
