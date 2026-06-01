import { computed, onMounted, reactive, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useQuasar } from "quasar";
import type { PlatformResourceInput, PlatformResourceName } from "./types";
import { errorMessage, getCategories } from "./providerUtils";
import { usePlatformStore } from "../../stores/platform";
import { pricingWarningMessage } from "../usage/pricingWarning";
import { PLATFORM_RESOURCE_TABLE_COLUMNS } from "../../components/platform/providerModelUi";
import { useProviderList } from "./useProviderList";
import { useProviderWizard } from "./useProviderWizard";

const columns = PLATFORM_RESOURCE_TABLE_COLUMNS;

export function useResourceManagerPage() {
  const route = useRoute();
  const platformStore = usePlatformStore();
  const $q = useQuasar();
  const isDark = computed(() => $q.dark.isActive);

  const saving = ref(false);
  const dialogOpen = ref(false);
  const editingId = ref("");

  const resource = computed(() => route.meta.resource as PlatformResourceName);
  const isProviderResource = computed(() => resource.value === "llm-provider-models");
  const pageTitle = computed(() => (route.meta.title as string) || "资源管理");
  const pageSubtitle = computed(() => (route.meta.subtitle as string) || "管理平台资源、启用状态与运行配置。");

  const list = useProviderList({ resource, isProviderResource, saving });
  const wizard = useProviderWizard({ editingId, dialogOpen, saving, resource, isProviderResource, rows: list.rows });

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
    wizard.resetProviderForm();
  }

  function openCreate() {
    editingId.value = "";
    resetForm();
    dialogOpen.value = true;
    void wizard.ensureCatalogLoaded();
  }

  async function openEdit(row: typeof list.rows.value[number]) {
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
      try {
        await wizard.populateProviderForm(row);
      } catch (error) {
        $q.notify({ type: "warning", message: `加载目录数据失败：${errorMessage(error)}` });
      }
    }
    dialogOpen.value = true;
  }

  async function saveRow() {
    if (isProviderResource.value) {
      await wizard.saveProviderRow();
      return;
    }
    if (!form.key || !form.name) {
      $q.notify({ type: "negative", message: "Key 和 Name 必填" });
      return;
    }
    saving.value = true;
    try {
      if (editingId.value) {
        const updated = await platformStore.editResource(resource.value, editingId.value, form);
        list.rows.value = list.rows.value.map((row) => (row.id === updated.id ? updated : row));
      } else {
        const created = await platformStore.addResource(resource.value, form);
        list.rows.value = [created, ...list.rows.value];
      }
      dialogOpen.value = false;
      $q.notify({ type: "positive", message: "已保存" });
    } catch (error) {
      $q.notify({ type: "negative", message: errorMessage(error) || "保存失败" });
    } finally {
      saving.value = false;
    }
  }

  onMounted(list.loadRows);

  watch(resource, () => {
    list.keyword.value = "";
    list.page.value = 1;
    void list.loadRows();
  });

  return {
    isDark,
    rows: list.rows,
    loading: list.loading,
    saving,
    checkingModel: wizard.checkingModel,
    keyword: list.keyword,
    dialogOpen,
    editingId,
    page: list.page,
    rowsPerPage: list.rowsPerPage,
    showApiKey: wizard.showApiKey,
    showSecretKey: wizard.showSecretKey,
    revealingCredentials: wizard.revealingCredentials,
    trendDialogOpen: list.trendDialogOpen,
    trendRow: list.trendRow,
    providerPresetKey: wizard.providerPresetKey,
    providerStep: wizard.providerStep,
    providerAddMode: wizard.providerAddMode,
    catalogProviderId: wizard.catalogProviderId,
    catalogProviderOptions: wizard.catalogProviderOptions,
    catalogProviderHint: wizard.catalogProviderHint,
    catalogProviderDocUrl: wizard.catalogProviderDocUrl,
    catalogModelsHint: wizard.catalogModelsHint,
    catalogProviderSearch: wizard.catalogProviderSearch,
    reloadCatalogProviders: wizard.reloadCatalogProviders,
    useCatalogModelPicker: wizard.useCatalogModelPicker,
    providerRuntimeLocked: wizard.providerRuntimeLocked,
    providerRuntimeSummary: wizard.providerRuntimeSummary,
    catalogLoading: wizard.catalogLoading,
    catalogModelsLoading: wizard.catalogModelsLoading,
    filterCatalogModelsLocal: wizard.filterCatalogModelsLocal,
    providerTypeFilter: list.providerTypeFilter,
    categoryOptions: wizard.categoryOptions,
    providerTypeOptions: wizard.providerTypeOptions,
    providerTypeFilterOptions: wizard.providerTypeFilterOptions,
    haCandidateProviderTypeOptions: wizard.haCandidateProviderTypeOptions,
    variantOptions: wizard.variantOptions,
    form,
    providerForm: wizard.providerForm,
    providerHAForm: wizard.providerHAForm,
    columns,
    isProviderResource,
    pageTitle,
    pageSubtitle,
    filteredRows: list.filteredRows,
    pageCount: list.pageCount,
    pagedProviderRows: list.pagedProviderRows,
    providerPresetOptions: wizard.providerPresetOptions,
    currentAuthType: wizard.currentAuthType,
    isLocalProviderModel: wizard.isLocalProviderModel,
    canInspectProviderModel: wizard.canInspectProviderModel,
    apiKeyFieldHint: wizard.apiKeyFieldHint,
    apiKeyMaskedPlaceholder: wizard.apiKeyMaskedPlaceholder,
    secretKeyMaskedPlaceholder: wizard.secretKeyMaskedPlaceholder,
    showPricingWarning: wizard.showPricingWarning,
    canSubmitNewProviderModel: wizard.canSubmitNewProviderModel,
    providerIdentityChanged: wizard.providerIdentityChanged,
    providerRuntimeBindingPreview: wizard.providerRuntimeBindingPreview,
    catalogPricingMissing: wizard.catalogPricingMissing,
    providerModelOptions: wizard.providerModelOptions,
    dialogTitle: wizard.dialogTitle,
    dialogSubtitle: wizard.dialogSubtitle,
    pricingWarningMessage,
    openCreate,
    openEdit,
    saveRow,
    applyProviderPreset: wizard.applyProviderPreset,
    applyModelPreset: wizard.applyModelPreset,
    applyCatalogProvider: wizard.applyCatalogProvider,
    applyCatalogModel: wizard.applyCatalogModel,
    setProviderAddMode: wizard.setProviderAddMode,
    setCustomModelValue: wizard.setCustomModelValue,
    inspectCurrentProviderModel: wizard.inspectCurrentProviderModel,
    toggleApiKeyVisibility: wizard.toggleApiKeyVisibility,
    toggleSecretKeyVisibility: wizard.toggleSecretKeyVisibility,
    toggleEnabled: list.toggleEnabled,
    confirmRemoveRow: list.confirmRemoveRow,
    openTrend: list.openTrend,
    listKeyState: list.listKeyState,
    toggleListKeyReveal: list.toggleListKeyReveal,
    trendOverview: list.providerTrend.overview,
    trendOverviewLoading: list.providerTrend.loading,
    trendMetric: list.providerTrend.metric,
    trendMetricOptions: list.providerTrend.metricOptions,
    providerCodeRule: wizard.providerCodeRule,
    updateHAForm: wizard.updateHAForm,
    getCategories,
    metadataLabel: wizard.metadataLabel,
    credentialEncryptionAvailable: list.credentialEncryptionAvailable,
  };
}
