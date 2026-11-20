import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import {
  batchUpdateAgents as batchUpdateAgentsApi,
  checkAgentKey,
  deleteAgent as deleteAgentApi,
  duplicateAgent,
  listAgentTemplates,
  listAgentsPaged,
  toggleAgentFavorite as toggleAgentFavoriteApi,
} from '../../features/agents/api';
import type { Agent, AgentTemplatePreset } from '../../features/agents/types';
import { listPlatformResources, listPlatformResourceTree, validateModel } from '../../features/platform/api';
import type { PlatformResource, PlatformResourceTreeNode } from '../../features/platform/types';
import { findTaxonomyPath, formatTaxonomyPath } from '../../features/platform/taxonomyTreeUtils';
import { emitSessionMutation } from '../sessionMutationBus';

/**
 * 花名册一次拉全量：左侧编制树要算每个公司 / 部门的人数，按页取拿不到。
 * shortcut: 上限 1000 个 Agent，超过时页面提示「只显示前 1000 个」；Agent 过千再改成服务端按节点分页 + 计数接口。
 */
export const ROSTER_LIMIT = 1000;

/** Agent 列表页（编制花名册）：全量 Agent、组织树、模型目录与列表上的增删改；HTTP 经 features/agents/api。 */
export const useAgentsPageStore = defineStore('agentsPage', () => {
  const agents = ref<Agent[]>([]);
  const total = ref(0);
  const listLoading = ref(false);
  /** 列表加载失败信息；非空时页面必须渲染可重试错误态，不得退化为空态。 */
  const listError = ref('');

  const taxonomyTree = ref<PlatformResourceTreeNode[]>([]);
  const providerModels = ref<PlatformResource[]>([]);

  const checkingModel = ref(false);
  const modelCheckPassed = ref(false);

  const selectedAgentIds = ref<string[]>([]);

  const providerOptions = computed(() =>
    Array.from(new Set(providerModels.value.map((row) => row.provider).filter(Boolean))).map((provider) => ({
      label: provider,
      value: provider,
    })),
  );

  function taxonomyLabel(id: string) {
    const path = findTaxonomyPath(taxonomyTree.value, id);
    return path.length ? formatTaxonomyPath(path) : '';
  }

  async function loadAgentList() {
    listLoading.value = true;
    listError.value = '';
    try {
      const result = await listAgentsPaged({ limit: ROSTER_LIMIT, offset: 0 });
      agents.value = result.items;
      total.value = result.total;
      // 已不在列表里的选中项（被删、被别处改成只读）一并清掉
      const ids = new Set(result.items.filter((a) => !a.readonly).map((a) => a.id));
      selectedAgentIds.value = selectedAgentIds.value.filter((id) => ids.has(id));
    } catch (error) {
      listError.value = error instanceof Error ? error.message : String(error);
      throw error;
    } finally {
      listLoading.value = false;
    }
  }

  async function loadAgentsDependencies() {
    const [treeRows, providerRows] = await Promise.all([
      listPlatformResourceTree('organization'),
      listPlatformResources('llm-provider-models'),
    ]);
    taxonomyTree.value = treeRows;
    providerModels.value = providerRows;
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

  function toggleAgentSelected(id: string) {
    const idx = selectedAgentIds.value.indexOf(id);
    if (idx >= 0) {
      selectedAgentIds.value.splice(idx, 1);
      return;
    }
    const agent = agents.value.find((item) => item.id === id);
    if (!agent || agent.readonly) return;
    selectedAgentIds.value.push(id);
  }

  function setSelection(ids: string[]) {
    const ok = new Set(agents.value.filter((a) => !a.readonly).map((a) => a.id));
    selectedAgentIds.value = ids.filter((id) => ok.has(id));
  }

  function clearAgentSelection() {
    selectedAgentIds.value = [];
  }

  /** 批量启停 / 删除；ids 缺省为当前选中。返回后端实际影响的条数。 */
  async function batchUpdateListedAgents(
    action: { status?: 'active' | 'inactive'; delete?: boolean },
    ids: string[] = [...selectedAgentIds.value],
  ): Promise<number> {
    if (ids.length === 0) return 0;
    const affected = await batchUpdateAgentsApi(ids, action);
    selectedAgentIds.value = selectedAgentIds.value.filter((id) => !ids.includes(id));
    await loadAgentList();
    return affected;
  }

  async function toggleAgentFavorite(id: string) {
    const agent = agents.value.find((item) => item.id === id);
    if (!agent) return;
    const previous = agent.is_favorite;
    agent.is_favorite = !previous;
    try {
      const updated = await toggleAgentFavoriteApi(id);
      agents.value = agents.value.map((item) =>
        item.id === id ? { ...item, is_favorite: updated.is_favorite } : item,
      );
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

  return {
    agents,
    total,
    listLoading,
    listError,
    taxonomyTree,
    providerModels,
    checkingModel,
    modelCheckPassed,
    providerOptions,
    taxonomyLabel,
    loadAgentList,
    loadAgentsDependencies,
    ensureTaxonomyTree,
    removeListedAgent,
    selectedAgentIds,
    toggleAgentSelected,
    setSelection,
    clearAgentSelection,
    batchUpdateListedAgents,
    toggleAgentFavorite,
    validateCreateModel,
    verifyAgentKey,
    fetchAgentTemplates,
    copyAgent,
  };
});

export { useAgentDetailStore } from './detail';
