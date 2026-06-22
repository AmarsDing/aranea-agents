import { computed, ref, watch, type ComputedRef, type Ref } from 'vue';
import { useQuasar } from 'quasar';
import type { ProviderForm } from './types';
import { errorMessage } from './providerUtils';
import type { ProviderPreset } from '../../config/providerPresets';
import { runtimeProfileFor } from '../../config/providerRuntimeOverlay';
import { applyCatalogModelFields, applyCatalogProviderFields, catalogModelToCost } from '../model-catalog/applyCatalog';
// TECH-DEBT: direct API calls; move to store — issue #catalog-bypass
// useProviderCatalog 直接调用 listCatalogProviders / listCatalogModels，
// 绕过了 Store 层。长期应将 catalog 状态和加载逻辑移至专用 Store，
// composable 只组合 Store。当前保留是因为 catalog 数据仅在向导对话框内使用，
// 不需要跨页面共享，且重构涉及 Store 拆分设计。
import { listCatalogModels, listCatalogProviders } from '../model-catalog/api';
import { ensureProviderMigrationMap } from '../model-catalog/providerMigration';
import type { CatalogModelSummary, CatalogProviderSummary } from '../../services/kratos/model_catalog/v1/index';

export function useProviderCatalog(deps: {
  providerForm: ProviderForm;
  providerAddMode: Ref<'catalog' | 'custom'>;
  providerCreateInspectFingerprint: Ref<string>;
  editingId: Ref<string>;
  dialogOpen: Ref<boolean>;
  isProviderResource: ComputedRef<boolean>;
  currentProviderPreset: ComputedRef<ProviderPreset | undefined>;
}) {
  const $q = useQuasar();

  const catalogProviderId = ref('');
  const catalogProviderSearch = ref('');
  const catalogProviders = ref<CatalogProviderSummary[]>([]);
  const catalogModels = ref<CatalogModelSummary[]>([]);
  const catalogModelFilterLocal = ref('');
  const catalogModelTotal = ref(0);
  const catalogLoading = ref(false);
  const catalogModelsLoading = ref(false);

  const catalogProviderOptions = computed(() =>
    catalogProviders.value.map((p) => ({
      label: p.name || p.id || '',
      value: p.id || '',
      caption: `${p.modelCount ?? 0} 模型 · ${p.api || '—'}`,
    })),
  );

  const selectedCatalogProvider = computed(
    () => catalogProviders.value.find((p) => p.id === catalogProviderId.value) ?? null,
  );

  const catalogProviderHint = computed(() => {
    const p = selectedCatalogProvider.value;
    if (!p) return '';
    const env = (p.env ?? []).filter(Boolean);
    if (!env.length) return '';
    return `环境变量: ${env.join(', ')}`;
  });

  const catalogProviderDocUrl = computed(() => {
    const doc = selectedCatalogProvider.value?.doc?.trim();
    if (!doc) return '';
    if (/^https?:\/\//i.test(doc)) return doc;
    return `https://${doc}`;
  });

  const activeCatalogProviderId = computed(() => catalogProviderId.value || deps.providerForm.provider_code.trim());

  const useCatalogModelPicker = computed(
    () => deps.providerAddMode.value === 'catalog' || catalogModels.value.length > 0,
  );

  const catalogModelsHint = computed(() => {
    const code = activeCatalogProviderId.value;
    if (!code) return '';
    const name = selectedCatalogProvider.value?.name || deps.providerForm.provider_display_name.trim() || code;
    if (catalogModelsLoading.value) return `正在加载 ${name} 的模型…`;
    if (catalogModelTotal.value <= 0) {
      if (deps.providerAddMode.value === 'catalog') return `${name}：目录中无模型（请检查 catalog 同步）`;
      return '';
    }
    return `${name}：${catalogModelTotal.value} 个模型可选`;
  });

  const selectedCatalogModelLabel = computed(() => {
    const id = deps.providerForm.model_api_id.trim();
    if (!id) return '';
    const model = catalogModels.value.find((m) => m.id === id);
    return model?.name || id;
  });

  const selectedCatalogModel = computed(() => {
    const id = deps.providerForm.model_api_id.trim();
    if (!id) return undefined;
    return catalogModels.value.find((m) => m.id === id);
  });

  function mapCatalogModelsToOptions(models: CatalogModelSummary[]) {
    const q = catalogModelFilterLocal.value.trim().toLowerCase();
    let list = models;
    if (q) {
      list = list.filter((model) =>
        [model.id, model.name, model.family, model.knowledge].some((v) => (v || '').toLowerCase().includes(q)),
      );
    }
    return list.map((model) => ({
      label: model.name || model.id || '',
      value: model.id || '',
      caption: [
        model.id,
        model.contextTokens ? `${Math.round(model.contextTokens / 1000)}K ctx` : '',
        model.inputUsdPer1m ? `$${model.inputUsdPer1m}/M in` : '',
        model.status && model.status !== 'active' ? model.status : '',
      ]
        .filter(Boolean)
        .join(' · '),
    }));
  }

  const providerModelOptions = computed(() => {
    if (useCatalogModelPicker.value && catalogModels.value.length > 0) {
      return mapCatalogModelsToOptions(catalogModels.value);
    }
    if (deps.providerAddMode.value === 'catalog') return [];
    return (deps.currentProviderPreset.value?.models ?? []).map((model) => ({
      label: model.label,
      value: model.id,
      caption: `${model.id}${model.contextWindowK ? ` · ${model.contextWindowK}K ctx` : ''}`,
    }));
  });

  function catalogModelHasPricing(model: CatalogModelSummary | undefined): boolean {
    if (!model) return false;
    const cost = catalogModelToCost(model);
    return (
      cost.input_usd_per_1m > 0 ||
      cost.output_usd_per_1m > 0 ||
      cost.cache_read_usd_per_1m > 0 ||
      cost.cache_write_usd_per_1m > 0 ||
      cost.reasoning_usd_per_1m > 0
    );
  }

  const catalogPricingMissing = computed(
    () =>
      deps.isProviderResource.value &&
      deps.providerAddMode.value === 'catalog' &&
      Boolean(deps.providerForm.model_api_id.trim()) &&
      !catalogModelHasPricing(selectedCatalogModel.value),
  );

  watch(deps.dialogOpen, (open) => {
    if (open && deps.isProviderResource.value) {
      void ensureProviderMigrationMap().then(() => ensureCatalogLoaded());
    }
  });

  watch(catalogModels, (models) => {
    if (!deps.dialogOpen.value || !useCatalogModelPicker.value || !models.length) return;
    const id = deps.providerForm.model_api_id.trim();
    if (id && models.some((m) => m.id === id)) {
      applyCatalogModel(id);
    }
  });

  async function ensureCatalogLoaded(q = '') {
    catalogLoading.value = true;
    try {
      const res = await listCatalogProviders(q, 200, 0);
      catalogProviders.value = res.items;
    } catch (error) {
      $q.notify({ type: 'warning', message: errorMessage(error) || '模型目录未加载，请先在系统设置同步 catalog' });
    } finally {
      catalogLoading.value = false;
    }
  }

  async function reloadCatalogProviders() {
    await ensureCatalogLoaded(catalogProviderSearch.value.trim());
  }

  function applyProviderRuntimeFields(providerId: string) {
    const rt = runtimeProfileFor(providerId);
    deps.providerForm.provider_type = rt.providerType;
    deps.providerForm.variant = rt.variant || 'openai';
  }

  async function loadCatalogModels(providerId: string, q = '', includeDeprecated = false) {
    if (!providerId) {
      catalogModels.value = [];
      catalogModelTotal.value = 0;
      return;
    }
    catalogModelsLoading.value = true;
    try {
      const res = await listCatalogModels(providerId, q, includeDeprecated, 500, 0);
      catalogModels.value = res.items;
      catalogModelTotal.value = res.total;
    } catch (error) {
      catalogModels.value = [];
      catalogModelTotal.value = 0;
      $q.notify({ type: 'warning', message: errorMessage(error) || '加载目录模型失败' });
    } finally {
      catalogModelsLoading.value = false;
    }
  }

  function filterCatalogModelsLocal(val: string, update: (fn: () => void) => void) {
    catalogModelFilterLocal.value = val;
    update(() => {});
  }

  function applyCatalogModel(modelId: string) {
    const providerId = catalogProviderId.value || deps.providerForm.provider_code.trim();
    const model = catalogModels.value.find((item) => item.id === modelId);
    if (!model || !providerId) return;
    const fields = applyCatalogModelFields(providerId, model, true);
    const { reasoning_backfill, model_category, ...formFields } = fields;
    Object.assign(deps.providerForm, formFields);
    if (model_category?.length) {
      deps.providerForm.model_category = model_category;
    }
    if (reasoning_backfill !== undefined) {
      deps.providerForm.reasoning_backfill = reasoning_backfill;
    }
    deps.providerForm.catalog_source = 'models.dev';
    deps.providerForm.metadata_source = 'models.dev';
  }

  async function applyCatalogProvider(providerId: string) {
    if (!providerId) return;
    catalogProviderId.value = providerId;
    catalogModelFilterLocal.value = '';
    const summary = catalogProviders.value.find((item) => item.id === providerId);
    const fields = applyCatalogProviderFields(providerId, summary?.name || providerId, summary?.api);
    Object.assign(deps.providerForm, fields);
    applyProviderRuntimeFields(providerId);
    deps.providerForm.catalog_managed = true;
    deps.providerForm.catalog_source = 'models.dev';
    deps.providerForm.metadata_source = 'models.dev';
    deps.providerCreateInspectFingerprint.value = '';
    await loadCatalogModels(providerId, '', true);
    if (!catalogModels.value.length) {
      $q.notify({ type: 'warning', message: `${summary?.name || providerId} 在目录中无可用模型` });
      deps.providerForm.model_api_id = '';
      return;
    }
    const keepModel = catalogModels.value.find((m) => m.id === deps.providerForm.model_api_id);
    const pick = keepModel ?? catalogModels.value[0];
    if (pick?.id) {
      deps.providerForm.model_api_id = pick.id;
      applyCatalogModel(pick.id);
    }
  }

  return {
    catalogProviderId,
    catalogProviderSearch,
    catalogProviders,
    catalogModels,
    catalogModelFilterLocal,
    catalogModelTotal,
    catalogLoading,
    catalogModelsLoading,
    catalogProviderOptions,
    catalogProviderHint,
    catalogProviderDocUrl,
    catalogModelsHint,
    selectedCatalogModelLabel,
    selectedCatalogModel,
    activeCatalogProviderId,
    useCatalogModelPicker,
    providerModelOptions,
    catalogPricingMissing,
    ensureCatalogLoaded,
    reloadCatalogProviders,
    loadCatalogModels,
    filterCatalogModelsLocal,
    applyCatalogProvider,
    applyCatalogModel,
    mapCatalogModelsToOptions,
    catalogModelHasPricing,
    applyProviderRuntimeFields,
  };
}
