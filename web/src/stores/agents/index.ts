import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import {
  checkAgentKey,
  deleteAgent as deleteAgentApi,
  duplicateAgent,
  listAgentCreators,
  listAgentTemplates,
  listAgentsPaged,
  toggleAgentFavorite as toggleAgentFavoriteApi,
} from '../../features/agents/api';
import type { Agent, AgentCreatorOption, AgentTemplatePreset } from '../../features/agents/types';
import { listPlatformResources, listPlatformResourceTree, validateModel } from '../../features/platform/api';
import type { PlatformResource, PlatformResourceTreeNode } from '../../features/platform/types';
import { flattenTaxonomyPositions, formatContext } from '../../components/agents/agentUi';
import { buildAgentTableColumns } from '../../components/agents/agentTableUi';
import { findTaxonomyPath, formatTaxonomyPath } from '../../features/platform/taxonomyTreeUtils';
import { emitSessionMutation } from '../sessionSync';

/** Agent 列表页：筛选、分页、依赖数据与列表 CRUD；Agent HTTP 经 features/agents/api（Kratos）。 */
export const useAgentsPageStore = defineStore('agentsPage', () => {
  const keyword = ref('');
  const selectedStatus = ref<string | null>(null);
  const selectedProvider = ref<string | null>(null);
  const selectedTaxonomy = ref<string | null>(null);
  const selectedCreator = ref<string | null>(null);
  const creatorOptions = ref<AgentCreatorOption[]>([]);
  const page = ref(1);
  const rowsPerPage = ref(21);
  const agents = ref<Agent[]>([]);
  const total = ref(0);
  const listLoading = ref(false);

  const taxonomyTree = ref<PlatformResourceTreeNode[]>([]);
  const providerModels = ref<PlatformResource[]>([]);

  const checkingModel = ref(false);
  const modelCheckPassed = ref(false);

  const industryNodes = computed(() => taxonomyTree.value.filter((row) => row.level === 'company' && row.enabled));
  const taxonomyPositionOptions = computed(() => flattenTaxonomyPositions(industryNodes.value));
  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / rowsPerPage.value)));

  const providerOptions = computed(() =>
    Array.from(new Set(providerModels.value.map((row) => row.provider).filter(Boolean))).map((provider) => ({
      label: provider,
      value: provider,
    })),
  );

  function taxonomyLabel(id: string) {
    if (!id) return '未分类';
    const path = findTaxonomyPath(taxonomyTree.value, id);
    if (path.length) return formatTaxonomyPath(path);
    return taxonomyPositionOptions.value.find((item) => item.value === id)?.label ?? '未分类';
  }

  const tableColumns = computed(() => buildAgentTableColumns(taxonomyLabel, formatContext));

  async function loadAgentList() {
    listLoading.value = true;
    try {
      const result = await listAgentsPaged({
        keyword: keyword.value || undefined,
        status: selectedStatus.value || undefined,
        provider: selectedProvider.value || undefined,
        org_node_id: selectedTaxonomy.value || undefined,
        created_by: selectedCreator.value && selectedCreator.value !== '' ? selectedCreator.value : undefined,
        limit: rowsPerPage.value,
        offset: (page.value - 1) * rowsPerPage.value,
      });
      agents.value = result.items;
      total.value = result.total;
    } finally {
      listLoading.value = false;
    }
  }

  async function loadAgentsDependencies() {
    const [treeRows, providerRows, creators] = await Promise.all([
      listPlatformResourceTree('organization'),
      listPlatformResources('llm-provider-models'),
      listAgentCreators().catch(() => [] as AgentCreatorOption[]),
    ]);
    taxonomyTree.value = treeRows;
    providerModels.value = providerRows;
    creatorOptions.value = [{ user_id: '', label: '所有创建者' }, ...creators];
    emitSessionMutation({ type: 'agents_dependencies_loaded' });
  }

  async function ensureTaxonomyTree() {
    if (taxonomyTree.value.length > 0) return;
    taxonomyTree.value = await listPlatformResourceTree('organization');
  }

  async function removeListedAgent(id: string) {
    await deleteAgentApi(id);
    await loadAgentList();
  }

  async function toggleAgentFavorite(id: string) {
    const agent = agents.value.find((item) => item.id === id);
    if (!agent) return;
    const previous = agent.is_favorite;
    agent.is_favorite = !previous;
    try {
      const updated = await toggleAgentFavoriteApi(id);
      agents.value = agents.value.map((item) => (item.id === id ? updated : item));
      emitSessionMutation({ type: 'agent_updated', agent: updated });
    } catch (error) {
      agent.is_favorite = previous;
      throw error;
    }
  }

  async function validateCreateModel(provider: string, model: string) {
    checkingModel.value = true;
    try {
      const result = await validateModel(provider, model);
      modelCheckPassed.value = result.ok;
      return result;
    } finally {
      checkingModel.value = false;
    }
  }

  async function verifyAgentKey(agentKey: string) {
    return checkAgentKey(agentKey);
  }

  async function fetchAgentTemplates(): Promise<AgentTemplatePreset[]> {
    return listAgentTemplates();
  }

  async function copyAgent(id: string): Promise<Agent> {
    return duplicateAgent(id);
  }

  function resetListFiltersAfterCreate() {
    keyword.value = '';
    selectedStatus.value = null;
    selectedProvider.value = null;
    selectedTaxonomy.value = null;
    selectedCreator.value = null;
    page.value = 1;
  }

  function reorderAgents(ids: string[]) {
    const map = new Map(agents.value.map((a) => [a.id, a]));
    const reordered = ids.map((id) => map.get(id)).filter(Boolean) as Agent[];
    const remaining = agents.value.filter((a) => !ids.includes(a.id));
    agents.value = [...reordered, ...remaining];
  }

  return {
    keyword,
    selectedStatus,
    selectedProvider,
    selectedTaxonomy,
    selectedCreator,
    creatorOptions,
    page,
    rowsPerPage,
    agents,
    total,
    listLoading,
    taxonomyTree,
    providerModels,
    checkingModel,
    modelCheckPassed,
    industryNodes,
    taxonomyPositionOptions,
    pageMax,
    providerOptions,
    taxonomyLabel,
    tableColumns,
    loadAgentList,
    loadAgentsDependencies,
    ensureTaxonomyTree,
    removeListedAgent,
    toggleAgentFavorite,
    validateCreateModel,
    resetListFiltersAfterCreate,
    verifyAgentKey,
    fetchAgentTemplates,
    copyAgent,
    reorderAgents,
  };
});

export { useAgentDetailStore } from './detail';
