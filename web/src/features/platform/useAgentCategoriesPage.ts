import { computed, onMounted, reactive, ref } from "vue";
import { useQuasar } from "quasar";
import type { PlatformResourceInput, PlatformResourceTreeNode } from "./types";
import { usePlatformStore } from "../../stores/platform";
import {
  categoryTreeStats,
  collectDefaultExpandedIds,
  filterCategoryTree,
  findCategoryNode,
  levelLabel,
  parseIsSystem,
  patchCategoryTreeNode,
  trimmedDesc,
  type CategoryLevel
} from "./categoryTreeUtils";

const CATEGORY_RESOURCE = "agent-categories" as const;

export function useAgentCategoriesPage() {
  const $q = useQuasar();
  const platformStore = usePlatformStore();

  const isDark = computed(() => $q.dark.isActive);
  const loading = ref(false);
  const saving = ref(false);
  const keyword = ref("");
  const onlyCustom = ref(false);
  const dialogOpen = ref(false);
  const editingId = ref("");
  const parentNode = ref<PlatformResourceTreeNode | null>(null);
  const tree = ref<PlatformResourceTreeNode[]>([]);
  const togglingIds = ref<Set<string>>(new Set());

  const form = reactive<PlatformResourceInput & { level: CategoryLevel }>({
    key: "",
    name: "",
    description: "",
    enabled: true,
    sort_order: 0,
    parent_id: "",
    level: "industry",
    config_json: "{}",
    metadata_json: "{}"
  });

  const filteredTree = computed(() =>
    filterCategoryTree(
      tree.value.filter((node) => node.level === "industry"),
      keyword.value,
      onlyCustom.value
    )
  );
  const stats = computed(() => categoryTreeStats(tree.value));
  const parentName = computed(() => parentNode.value?.name || "无");

  async function loadTree(opts?: { silent?: boolean }) {
    if (!opts?.silent) loading.value = true;
    try {
      await platformStore.loadCategoryTree(CATEGORY_RESOURCE);
      tree.value = platformStore.categoryTree;
    } finally {
      if (!opts?.silent) loading.value = false;
    }
  }

  function syncTreePatch(id: string, patch: Partial<PlatformResourceTreeNode>) {
    tree.value = patchCategoryTreeNode(tree.value, id, patch);
    platformStore.categoryTree = tree.value;
  }

  function openCreate(level: CategoryLevel, parent?: PlatformResourceTreeNode) {
    const canonicalParent = parent ? findNode(parent.id) ?? parent : null;
    editingId.value = "";
    parentNode.value = canonicalParent;
    Object.assign(form, {
      key: "",
      name: "",
      description: "",
      enabled: true,
      sort_order: nextSortOrder(canonicalParent ?? undefined),
      parent_id: canonicalParent?.id ?? "",
      level,
      config_json: "{}",
      metadata_json: JSON.stringify({ is_system: false })
    });
    dialogOpen.value = true;
  }

  function openEdit(node: PlatformResourceTreeNode) {
    editingId.value = node.id;
    parentNode.value = findNode(node.parent_id);
    Object.assign(form, {
      key: node.key,
      name: node.name,
      description: node.description,
      enabled: node.enabled,
      sort_order: node.sort_order,
      parent_id: node.parent_id,
      level: node.level as CategoryLevel,
      config_json: node.config_json || "{}",
      metadata_json: node.metadata_json || "{}"
    });
    dialogOpen.value = true;
  }

  async function saveNode() {
    if (!form.name.trim()) {
      $q.notify({ type: "negative", message: "名称必填" });
      return;
    }
    saving.value = true;
    try {
      const payload: PlatformResourceInput = {
        ...form,
        key: editingId.value ? form.key : buildKey(form.level, form.name),
        parent_id: form.parent_id || "",
        metadata_json: form.metadata_json || JSON.stringify({ is_system: false })
      };
      if (editingId.value) {
        await platformStore.editResource(CATEGORY_RESOURCE, editingId.value, payload);
      } else {
        await platformStore.addResource(CATEGORY_RESOURCE, payload);
      }
      dialogOpen.value = false;
      await loadTree();
      $q.notify({ type: "positive", message: "已保存分类" });
    } catch (error) {
      $q.notify({ type: "negative", message: errorMessage(error) || "保存分类失败" });
    } finally {
      saving.value = false;
    }
  }

  async function removeNode(node: PlatformResourceTreeNode) {
    if (parseIsSystem(node)) {
      $q.notify({ type: "warning", message: "系统预置分类不可删除" });
      return;
    }
    if ((node.children?.length ?? 0) > 0) {
      $q.notify({ type: "warning", message: "请先删除或迁移子分类" });
      return;
    }
    try {
      await platformStore.removeResource(CATEGORY_RESOURCE, node.id);
      await loadTree();
      $q.notify({ type: "positive", message: "已删除分类" });
    } catch (error) {
      $q.notify({ type: "negative", message: errorMessage(error) || "删除分类失败" });
    }
  }

  async function toggleNodeEnabled(node: PlatformResourceTreeNode, enabled: boolean) {
    if (node.enabled === enabled || togglingIds.value.has(node.id)) return;

    const previous = node.enabled;
    togglingIds.value = new Set([...togglingIds.value, node.id]);
    syncTreePatch(node.id, { enabled });

    try {
      const updated = await platformStore.editResource(CATEGORY_RESOURCE, node.id, { enabled });
      syncTreePatch(node.id, { enabled: updated.enabled });
      $q.notify({ type: "positive", message: enabled ? "分类已启用" : "分类已停用" });
    } catch (error) {
      syncTreePatch(node.id, { enabled: previous });
      $q.notify({ type: "negative", message: errorMessage(error) || "更新分类状态失败" });
    } finally {
      const next = new Set(togglingIds.value);
      next.delete(node.id);
      togglingIds.value = next;
    }
  }

  function nextSortOrder(parent?: PlatformResourceTreeNode) {
    const siblings = parent ? parent.children ?? [] : tree.value.filter((node) => node.level === "industry");
    return siblings.length > 0 ? Math.max(...siblings.map((node) => node.sort_order || 0)) + 10 : 10;
  }

  function findNode(id: string) {
    if (!id) return null;
    return findCategoryNode(tree.value, id);
  }

  function buildKey(level: string, name: string) {
    const ascii = name
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-|-$/g, "");
    const parentPart = form.parent_id ? form.parent_id.replace(/[^a-z0-9]+/gi, "").slice(-8).toLowerCase() : "root";
    const entropy = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`;
    return `${level}-${parentPart}-${ascii || "node"}-${entropy}`;
  }

  function errorMessage(error: unknown) {
    if (typeof error === "object" && error && "response" in error) {
      const response = (error as { response?: { data?: { error?: string } } }).response;
      return response?.data?.error;
    }
    return error instanceof Error ? error.message : "";
  }

  onMounted(loadTree);

  return {
    isDark,
    loading,
    saving,
    togglingIds,
    keyword,
    onlyCustom,
    dialogOpen,
    editingId,
    parentNode,
    tree,
    form,
    filteredTree,
    stats,
    parentName,
    loadTree,
    openCreate,
    openEdit,
    saveNode,
    removeNode,
    toggleNodeEnabled,
    levelLabel,
    trimmedDesc
  };
}
