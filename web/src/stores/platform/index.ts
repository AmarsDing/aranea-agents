import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listPlatformResources,
  listPlatformResourceTree,
  createPlatformResource,
  updatePlatformResource,
  deletePlatformResource,
  revealProviderModelCredentials,
  validateModel,
  inspectProviderModel,
  reorderTaxonomy,
} from '../../features/platform/api';
import { getSystemSettings } from '../../features/system-settings/api';
import { patchTaxonomyTreeNode } from '../../features/platform/taxonomyTreeUtils';
import type {
  PlatformResource,
  PlatformResourceTreeNode,
  PlatformResourceInput,
  PlatformResourceName,
  InspectProviderModelInput,
  InspectProviderModelResult,
} from '../../features/platform/types';

function removeTaxonomyTreeNodeInner(
  tree: PlatformResourceTreeNode[],
  id: string,
): PlatformResourceTreeNode[] {
  return tree
    .filter((node) => node.id !== id)
    .map((node) => ({
      ...node,
      children: removeTaxonomyTreeNodeInner(node.children ?? [], id),
    }));
}

function appendChildInner(
  tree: PlatformResourceTreeNode[],
  parentId: string,
  child: PlatformResourceTreeNode,
): PlatformResourceTreeNode[] {
  return tree.map((node) => {
    if (node.id === parentId) {
      return { ...node, children: [...(node.children ?? []), child] };
    }
    if (node.children?.length) {
      return { ...node, children: appendChildInner(node.children, parentId, child) };
    }
    return node;
  });
}

export const usePlatformStore = defineStore('platform', () => {
  const providerModels = ref<PlatformResource[]>([]);
  const taxonomyTree = ref<PlatformResourceTreeNode[]>([]);
  const loading = ref(false);
  const credentialEncryptionAvailable = ref<boolean | null>(null);

  async function loadProviderModels() {
    loading.value = true;
    try {
      providerModels.value = await listPlatformResources('llm-provider-models');
    } finally {
      loading.value = false;
    }
  }

  async function loadTaxonomyTree(resource: 'taxonomy-nodes' | 'taxonomy' = 'taxonomy') {
    taxonomyTree.value = await listPlatformResourceTree(resource);
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

  async function reorderTaxonomyNodes(ids: string[]) {
    return reorderTaxonomy(ids);
  }

  function setTaxonomyTree(tree: PlatformResourceTreeNode[]) {
    taxonomyTree.value = tree;
  }

  function applyTaxonomyTreePatch(id: string, patch: Partial<PlatformResourceTreeNode>) {
    taxonomyTree.value = patchTaxonomyTreeNode(taxonomyTree.value, id, patch);
  }

  function removeTaxonomyTreeNode(id: string) {
    taxonomyTree.value = removeTaxonomyTreeNodeInner(taxonomyTree.value, id);
  }

  function appendTaxonomyTreeChild(parentId: string, child: PlatformResourceTreeNode) {
    taxonomyTree.value = appendChildInner(taxonomyTree.value, parentId, child);
  }

  return {
    providerModels,
    taxonomyTree,
    loading,
    credentialEncryptionAvailable,
    loadProviderModels,
    loadTaxonomyTree,
    loadResource,
    addResource,
    editResource,
    removeResource,
    reorderTaxonomyNodes,
    setTaxonomyTree,
    applyTaxonomyTreePatch,
    removeTaxonomyTreeNode,
    appendTaxonomyTreeChild,
    checkModel,
    revealCredentials,
    inspectModel,
    loadCredentialStatus,
  };
});
