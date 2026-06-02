import { defineStore } from "pinia";
import { ref } from "vue";
import {
  listPlatformResources,
  listPlatformResourceTree,
  createPlatformResource,
  updatePlatformResource,
  deletePlatformResource,
  revealProviderModelCredentials,
  validateModel,
  inspectProviderModel
} from "../../features/platform/api";
import { getSystemSettings } from "../../features/system-settings/api";
import type {
  PlatformResource,
  PlatformResourceTreeNode,
  PlatformResourceInput,
  PlatformResourceName,
  InspectProviderModelInput,
  InspectProviderModelResult
} from "../../features/platform/types";

export const usePlatformStore = defineStore("platform", () => {
  const providerModels = ref<PlatformResource[]>([]);
  const categoryTree = ref<PlatformResourceTreeNode[]>([]);
  const loading = ref(false);
  const credentialEncryptionAvailable = ref<boolean | null>(null);

  async function loadProviderModels() {
    loading.value = true;
    try {
      providerModels.value = await listPlatformResources("llm-provider-models");
    } finally {
      loading.value = false;
    }
  }

  async function loadCategoryTree(resource: "agent-categories" | "taxonomy" = "taxonomy") {
    categoryTree.value = await listPlatformResourceTree(resource);
  }

  async function loadResource(resource: PlatformResourceName) {
    return listPlatformResources(resource);
  }

  async function addResource(resource: PlatformResourceName, payload: PlatformResourceInput) {
    return createPlatformResource(resource, payload);
  }

  async function editResource(resource: PlatformResourceName, id: string, payload: Partial<PlatformResourceInput>) {
    return updatePlatformResource(resource, id, payload);
  }

  async function removeResource(resource: PlatformResourceName, id: string) {
    return deletePlatformResource(resource, id);
  }

  async function checkModel(provider: string, model: string) {
    return validateModel(provider, model);
  }

  async function revealCredentials(modelId: string) {
    return revealProviderModelCredentials(modelId);
  }

  async function inspectModel(payload: InspectProviderModelInput): Promise<InspectProviderModelResult> {
    return inspectProviderModel(payload);
  }

  async function loadCredentialStatus() {
    try {
      const settings = await getSystemSettings();
      credentialEncryptionAvailable.value = settings.credentialEncryptionKeyConfigured ?? false;
    } catch {
      credentialEncryptionAvailable.value = null;
    }
  }

  return {
    providerModels,
    categoryTree,
    loading,
    credentialEncryptionAvailable,
    loadProviderModels,
    loadCategoryTree,
    loadResource,
    addResource,
    editResource,
    removeResource,
    checkModel,
    revealCredentials,
    inspectModel,
    loadCredentialStatus
  };
});
