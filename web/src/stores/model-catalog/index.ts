import { defineStore } from "pinia";
import { ref } from "vue";
import {
  getModelCatalogStatus,
  getModelCatalogPolicy,
  updateModelCatalogPolicy,
  syncModelCatalog,
  listModelCatalogSyncLogs,
  listCatalogProviders,
  searchCatalogBlocks,
  previewModelCatalogMigration,
  getProviderMigrationRules,
  applyProviderMigration
} from "../../features/model-catalog/api";
import {
  fetchProviderLogoSvg as fetchProviderLogoSvgFromApi,
  clearProviderLogoCache
} from "../../features/model-catalog/providerLogo";
import {
  ensureProviderMigrationMap,
  resetProviderMigrationCache
} from "../../features/model-catalog/providerMigration";
import type { ModelCatalogPolicy, ModelCatalogStatus } from "../../services/kratos/model_catalog/v1/index";

export const useModelCatalogStore = defineStore("model-catalog", () => {
  const status = ref<ModelCatalogStatus | null>(null);
  const policy = ref<ModelCatalogPolicy | null>(null);
  const loading = ref(false);

  async function loadStatus() {
    status.value = await getModelCatalogStatus();
    return status.value;
  }

  async function loadPolicy() {
    policy.value = await getModelCatalogPolicy();
    return policy.value;
  }

  async function savePolicy(p: ModelCatalogPolicy) {
    policy.value = await updateModelCatalogPolicy(p);
    return policy.value;
  }

  async function runSync(dryRun = false) {
    return syncModelCatalog(dryRun);
  }

  async function loadSyncLogs(limit = 30) {
    return listModelCatalogSyncLogs(limit);
  }

  async function loadProviders(q = "", limit = 200, offset = 0) {
    return listCatalogProviders(q, limit, offset);
  }

  async function searchBlocks(q = "", limit = 10, offset = 0) {
    return searchCatalogBlocks(q, limit, offset);
  }

  async function loadMigrationPreview() {
    return previewModelCatalogMigration();
  }

  async function loadMigrationRules() {
    return getProviderMigrationRules();
  }

  async function runApplyMigration() {
    return applyProviderMigration();
  }

  async function fetchProviderLogoSvg(providerId: string) {
    return fetchProviderLogoSvgFromApi(providerId);
  }

  function clearLogoCache() {
    clearProviderLogoCache();
  }

  async function loadMigrationMap() {
    return ensureProviderMigrationMap();
  }

  function resetMigrationCache() {
    resetProviderMigrationCache();
  }

  return {
    status,
    policy,
    loading,
    loadStatus,
    loadPolicy,
    savePolicy,
    runSync,
    loadSyncLogs,
    loadProviders,
    searchBlocks,
    loadMigrationPreview,
    loadMigrationRules,
    runApplyMigration,
    fetchProviderLogoSvg,
    clearLogoCache,
    loadMigrationMap,
    resetMigrationCache
  };
});
