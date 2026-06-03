import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import {
  getModelCatalogPolicy,
  getModelCatalogStatus,
  listModelCatalogSyncLogs,
  syncModelCatalog,
  updateModelCatalogPolicy,
  previewModelCatalogMigration,
  getProviderMigrationRules,
  applyProviderMigration,
  listCatalogProviders,
  searchCatalogBlocks,
  type ModelCatalogPolicy,
  type ModelCatalogStatus,
} from './api';
import { clearProviderLogoCache } from './providerLogo';
import { resetProviderMigrationCache } from './providerMigration';

export function useModelCatalogTab() {
  const { t } = useI18n();
  const $q = useQuasar();

  const loading = ref(false);
  const savingPolicy = ref(false);
  const syncing = ref(false);
  const error = ref('');
  const status = ref<ModelCatalogStatus | null>(null);

  // --- JSON search ---
  const jsonFilter = ref('');
  const jsonSearchBlocks = ref<string[]>([]);
  const jsonSearchTotal = ref(0);
  const jsonSearchOffset = ref(0);
  const jsonSearchLimit = 1;
  const jsonSearchCap = 200;
  const jsonSearchLoading = ref(false);
  const jsonSearchError = ref('');
  const jsonSearchLegacyMode = ref(false);
  const jsonSearchTruncated = ref(false);

  const jsonSearchDisplayText = computed(() => jsonSearchBlocks.value[0] ?? '');
  const jsonSearchQuery = computed(() => (jsonFilter.value ?? '').trim());

  // --- Provider browse ---
  const providerBrowseQ = ref('');
  const providerBrowseItems = ref<Awaited<ReturnType<typeof listCatalogProviders>>['items']>([]);
  const providerBrowseTotal = ref(0);
  const providerBrowseOffset = ref(0);
  const providerBrowseLimit = 50;

  // --- Logs ---
  const logs = ref<Awaited<ReturnType<typeof listModelCatalogSyncLogs>>>([]);

  // --- Migration ---
  const loadingMigration = ref(false);
  const applyingMigration = ref(false);
  const migrationLoaded = ref(false);
  const migrationItems = ref<Awaited<ReturnType<typeof previewModelCatalogMigration>>['items']>([]);
  const migrationRules = ref<{ legacy: string; catalog: string }[]>([]);
  const migrationVersion = ref('');
  const migrationLastApplied = ref('');

  // --- Policy form ---
  const policyForm = reactive<ModelCatalogPolicy>({
    sourceUrl: 'https://models.dev/api.json',
    syncPolicy: 'scheduled',
    syncIntervalHours: 24,
    autoApply: 'metadata_and_pricing',
  });

  const syncPolicyOptions = computed(() => [
    { label: t('catalogTab.syncOff'), value: 'off' },
    { label: t('catalogTab.syncScheduled'), value: 'scheduled' },
  ]);

  const autoApplyOptions = computed(() => [
    { label: t('catalogTab.autoApplyNone'), value: 'none' },
    { label: t('catalogTab.autoApplyMetadataAndPricing'), value: 'metadata_and_pricing' },
    { label: t('catalogTab.autoApplyFullSpec'), value: 'full_spec' },
    { label: t('catalogTab.autoApplyFullSpecAndRuntime'), value: 'full_spec_and_runtime_overlay' },
  ]);

  const lastSyncLabel = computed(() => {
    const ts = status.value?.lastSyncAt;
    if (!ts) return '—';
    if (typeof ts === 'string') return ts;
    if (typeof ts === 'object' && ts !== null && 'seconds' in ts) {
      const sec = Number((ts as { seconds?: number }).seconds ?? 0);
      if (sec > 0) return new Date(sec * 1000).toLocaleString();
    }
    return '—';
  });

  function providerDocHref(doc?: string) {
    const d = doc?.trim();
    if (!d) return '';
    return /^https?:\/\//i.test(d) ? d : `https://${d}`;
  }

  async function loadProviderBrowse(resetOffset = false) {
    if (resetOffset) providerBrowseOffset.value = 0;
    try {
      const res = await listCatalogProviders(
        providerBrowseQ.value.trim(),
        providerBrowseLimit,
        providerBrowseOffset.value,
      );
      providerBrowseItems.value = res.items;
      providerBrowseTotal.value = res.total;
    } catch (e) {
      providerBrowseItems.value = [];
      providerBrowseTotal.value = 0;
      const msg = e instanceof Error ? e.message : t('catalogTab.loadProvidersFailed');
      error.value = msg;
      $q.notify({ type: 'warning', message: msg });
    }
  }

  function providerBrowsePrev() {
    providerBrowseOffset.value = Math.max(0, providerBrowseOffset.value - providerBrowseLimit);
    void loadProviderBrowse(false);
  }

  function providerBrowseNext() {
    providerBrowseOffset.value += providerBrowseLimit;
    void loadProviderBrowse(false);
  }

  function onJsonFilterChange(value: string | number | null) {
    jsonFilter.value = value == null ? '' : String(value);
    void loadJsonSearch(true);
  }

  async function loadJsonSearch(resetOffset = false) {
    if (resetOffset) jsonSearchOffset.value = 0;
    jsonSearchLoading.value = true;
    jsonSearchError.value = '';
    jsonSearchLegacyMode.value = false;
    jsonSearchTruncated.value = false;
    try {
      const q = jsonSearchQuery.value;
      const res = await searchCatalogBlocks(q, jsonSearchLimit, jsonSearchOffset.value);
      jsonSearchBlocks.value = res.blocks;
      jsonSearchTotal.value = res.total;
      jsonSearchOffset.value = res.offset;
      jsonSearchTruncated.value = res.truncated;
      jsonSearchLegacyMode.value = res.legacyLineMode;
      if (res.legacyLineMode) {
        jsonSearchError.value = t('catalogTab.legacyFormatError');
      }
    } catch (e) {
      jsonSearchBlocks.value = [];
      jsonSearchTotal.value = 0;
      const msg = e instanceof Error ? e.message : t('catalogTab.searchCatalogFailed');
      jsonSearchError.value = msg;
      error.value = msg;
      $q.notify({ type: 'warning', message: msg });
    } finally {
      jsonSearchLoading.value = false;
    }
  }

  function jsonSearchPrev() {
    jsonSearchOffset.value = Math.max(0, jsonSearchOffset.value - jsonSearchLimit);
    void loadJsonSearch(false);
  }

  function jsonSearchNext() {
    jsonSearchOffset.value += jsonSearchLimit;
    void loadJsonSearch(false);
  }

  async function loadAll() {
    loading.value = true;
    error.value = '';
    try {
      const [st, pol, logItems] = await Promise.all([
        getModelCatalogStatus(),
        getModelCatalogPolicy(),
        listModelCatalogSyncLogs(30),
      ]);
      status.value = st;
      Object.assign(policyForm, pol);
      logs.value = logItems;
      await Promise.all([loadProviderBrowse(true), loadJsonSearch(true)]);
    } catch (e) {
      error.value = e instanceof Error ? e.message : t('catalogTab.loadFailed');
    } finally {
      loading.value = false;
    }
  }

  async function savePolicy() {
    savingPolicy.value = true;
    error.value = '';
    try {
      await updateModelCatalogPolicy({ ...policyForm });
      $q.notify({ type: 'positive', message: t('catalogTab.policySaved') });
      await loadAll();
    } catch (e) {
      error.value = e instanceof Error ? e.message : t('catalogTab.saveFailed');
    } finally {
      savingPolicy.value = false;
    }
  }

  async function loadMigrationPreview() {
    loadingMigration.value = true;
    error.value = '';
    migrationItems.value = [];
    migrationLoaded.value = false;
    try {
      const res = await previewModelCatalogMigration();
      migrationItems.value = res.items ?? [];
      migrationLoaded.value = true;
    } catch (e) {
      error.value = e instanceof Error ? e.message : t('catalogTab.previewFailed');
      migrationItems.value = [];
      migrationLoaded.value = false;
    } finally {
      loadingMigration.value = false;
    }
  }

  async function loadMigrationRules() {
    try {
      const res = await getProviderMigrationRules();
      migrationRules.value = (res.rules ?? []).map((r) => ({
        legacy: r.legacy ?? '',
        catalog: r.catalog ?? '',
      }));
      migrationVersion.value = res.version ?? '';
      migrationLastApplied.value = res.lastAppliedAt ?? '';
    } catch {
      migrationRules.value = [];
    }
  }

  async function runApplyMigration() {
    applyingMigration.value = true;
    error.value = '';
    try {
      const res = await applyProviderMigration();
      $q.notify({
        type: res.ok ? 'positive' : 'warning',
        message: res.message || (res.ok ? t('catalogTab.migrationComplete') : t('catalogTab.migrationFailed')),
      });
      await loadMigrationPreview();
      resetProviderMigrationCache();
      await loadMigrationRules();
    } catch (e) {
      error.value = e instanceof Error ? e.message : t('catalogTab.migrationFailed');
    } finally {
      applyingMigration.value = false;
    }
  }

  async function runSync(dryRun: boolean) {
    syncing.value = true;
    error.value = '';
    try {
      const res = await syncModelCatalog(dryRun);
      const applyFailed = res.applyFailed || (res.applyErrors?.length ?? 0) > 0;
      if (res.ok && !applyFailed) {
        clearProviderLogoCache();
      }
      $q.notify({
        type: res.ok && !applyFailed ? 'positive' : 'warning',
        message:
          res.message ||
          (applyFailed
            ? t('catalogTab.syncCompleteButApplyFailed', { errors: (res.applyErrors ?? []).join('; ') })
            : res.ok
              ? t('catalogTab.syncComplete')
              : t('catalogTab.syncFailed')),
      });
      await loadAll();
    } catch (e) {
      error.value = e instanceof Error ? e.message : t('catalogTab.syncFailed');
    } finally {
      syncing.value = false;
    }
  }

  onMounted(() => {
    void loadAll();
    void loadMigrationRules();
  });

  return {
    // State
    loading,
    savingPolicy,
    syncing,
    error,
    status,
    policyForm,
    logs,
    loadingMigration,
    applyingMigration,
    migrationLoaded,
    migrationItems,
    migrationRules,
    migrationVersion,
    migrationLastApplied,
    providerBrowseQ,
    providerBrowseItems,
    providerBrowseTotal,
    providerBrowseOffset,
    jsonFilter,
    jsonSearchBlocks,
    jsonSearchTotal,
    jsonSearchOffset,
    jsonSearchLimit,
    jsonSearchCap,
    jsonSearchLoading,
    jsonSearchError,
    jsonSearchLegacyMode,
    jsonSearchTruncated,

    // Computed
    syncPolicyOptions,
    autoApplyOptions,
    lastSyncLabel,
    jsonSearchDisplayText,
    jsonSearchQuery,

    // Functions
    loadAll,
    savePolicy,
    runSync,
    loadMigrationPreview,
    runApplyMigration,
    loadProviderBrowse,
    providerBrowsePrev,
    providerBrowseNext,
    providerDocHref,
    onJsonFilterChange,
    loadJsonSearch,
    jsonSearchPrev,
    jsonSearchNext,

    // i18n (needed by template)
    t,
  };
}
