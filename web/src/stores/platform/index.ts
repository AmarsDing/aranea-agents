import { defineStore } from "pinia";
import { ref } from "vue";
import {
  listPlatformResources,
  listPlatformResourceTree,
  createPlatformResource,
  updatePlatformResource,
  deletePlatformResource,
  validateModel,
  type PlatformResource,
  type PlatformResourceTreeNode,
  type PlatformResourceInput,
  type PlatformResourceName
} from "../../features/platform/api";

export const usePlatformStore = defineStore("platform", () => {
  const providerModels = ref<PlatformResource[]>([]);
  const categoryTree = ref<PlatformResourceTreeNode[]>([]);
  const loading = ref(false);

  async function loadProviderModels() {
    loading.value = true;
    try {
      providerModels.value = await listPlatformResources("llm-provider-models");
    } finally {
      loading.value = false;
    }
  }

  async function loadCategoryTree(resource: PlatformResourceName = "agent-categories") {
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

  return { providerModels, categoryTree, loading, loadProviderModels, loadCategoryTree, loadResource, addResource, editResource, removeResource, checkModel };
});
