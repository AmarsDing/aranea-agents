import { defineStore } from "pinia";
import { computed, ref } from "vue";
import type { QTableColumn } from "quasar";
import {
  checkAgentKey,
  deleteAgent as deleteAgentApi,
  duplicateAgent,
  listAgentCreators,
  listAgentTemplates,
  listAgentsPaged,
  updateAgent
} from "../../features/agents/api";
import type { Agent, AgentCreatorOption, AgentTemplatePreset } from "../../features/agents/types";
import {
  listPlatformResources,
  listPlatformResourceTree,
  validateModel
} from "../../features/platform/api";
import type { PlatformResource, PlatformResourceTreeNode } from "../../features/platform/types";
import { flattenCategoryPositions, formatContext } from "../../components/agents/agentUi";
import { findCategoryPath, formatCategoryPath } from "../../features/platform/categoryTreeUtils";
import { useAppStore } from "../app";
import { useAvatarCatalogStore } from "../avatar";
import { registryColWidth } from "../../features/ui/registryTableColumns";

/** Agent 列表页：筛选、分页、依赖数据与列表 CRUD；Agent HTTP 经 features/agents/api（Kratos）。 */
export const useAgentsPageStore = defineStore("agentsPage", () => {
  const keyword = ref("");
  const selectedStatus = ref<string | null>(null);
  const selectedProvider = ref<string | null>(null);
  const selectedCategory = ref<string | null>(null);
  const selectedCreator = ref<string | null>(null);
  const creatorOptions = ref<AgentCreatorOption[]>([]);
  const page = ref(1);
  const rowsPerPage = ref(20);
  const agents = ref<Agent[]>([]);
  const total = ref(0);
  const listLoading = ref(false);

  const categoryTree = ref<PlatformResourceTreeNode[]>([]);
  const providerModels = ref<PlatformResource[]>([]);

  const checkingModel = ref(false);
  const modelCheckPassed = ref(false);

  const industryNodes = computed(() => categoryTree.value.filter((row) => row.level === "industry" && row.enabled));
  const categoryPositionOptions = computed(() => flattenCategoryPositions(industryNodes.value));
  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / rowsPerPage.value)));

  const providerOptions = computed(() =>
    Array.from(new Set(providerModels.value.map((row) => row.provider).filter(Boolean))).map((provider) => ({
      label: provider,
      value: provider
    }))
  );

  function categoryLabel(id: string) {
    if (!id) return "未分类";
    const path = findCategoryPath(categoryTree.value, id);
    if (path.length) return formatCategoryPath(path);
    return categoryPositionOptions.value.find((item) => item.value === id)?.label ?? "未分类";
  }

  const tableColumns = computed<QTableColumn<Agent>[]>(() => [
    { name: "name", label: "名称", field: "display_name", align: "left", ...registryColWidth("14%") },
    { name: "status", label: "状态", field: "status", align: "left", ...registryColWidth("9%") },
    { name: "model", label: "模型", field: (row) => `${row.provider} / ${row.model}`, align: "left", ...registryColWidth("20%") },
    { name: "category", label: "业务分类", field: (row) => categoryLabel(row.category_position_id), align: "left", ...registryColWidth("11%") },
    { name: "ctx", label: "上下文", field: (row) => formatContext(row.context_window), align: "left", ...registryColWidth("9%") },
    { name: "actions", label: "操作", field: "id", align: "right", ...registryColWidth("108px") }
  ]);

  async function loadAgentList() {
    listLoading.value = true;
    try {
      const result = await listAgentsPaged({
        keyword: keyword.value || undefined,
        status: selectedStatus.value || undefined,
        provider: selectedProvider.value || undefined,
        category_id: selectedCategory.value || undefined,
        created_by:
          selectedCreator.value && selectedCreator.value !== "" ? selectedCreator.value : undefined,
        limit: rowsPerPage.value,
        offset: (page.value - 1) * rowsPerPage.value
      });
      agents.value = result.items;
      total.value = result.total;
    } finally {
      listLoading.value = false;
    }
  }

  async function loadAgentsDependencies() {
    const [treeRows, providerRows, creators] = await Promise.all([
      listPlatformResourceTree("agent-categories"),
      listPlatformResources("llm-provider-models"),
      listAgentCreators().catch(() => [] as AgentCreatorOption[])
    ]);
    categoryTree.value = treeRows;
    providerModels.value = providerRows;
    creatorOptions.value = [{ user_id: "", label: "所有创建者" }, ...creators];
    const avatarCatalog = useAvatarCatalogStore();
    await avatarCatalog.ensureAgentsCatalog();
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
      const updated = await updateAgent(id, { ...agent, is_favorite: agent.is_favorite });
      agents.value = agents.value.map((item) => (item.id === id ? updated : item));
      useAppStore().upsertAgent(updated);
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
    keyword.value = "";
    selectedStatus.value = null;
    selectedProvider.value = null;
    selectedCategory.value = null;
    selectedCreator.value = null;
    page.value = 1;
  }

  return {
    keyword,
    selectedStatus,
    selectedProvider,
    selectedCategory,
    selectedCreator,
    creatorOptions,
    page,
    rowsPerPage,
    agents,
    total,
    listLoading,
    categoryTree,
    providerModels,
    checkingModel,
    modelCheckPassed,
    industryNodes,
    categoryPositionOptions,
    pageMax,
    providerOptions,
    categoryLabel,
    tableColumns,
    loadAgentList,
    loadAgentsDependencies,
    removeListedAgent,
    toggleAgentFavorite,
    validateCreateModel,
    resetListFiltersAfterCreate,
    verifyAgentKey,
    fetchAgentTemplates,
    copyAgent
  };
});

export { useAgentDetailStore } from "./detail";
