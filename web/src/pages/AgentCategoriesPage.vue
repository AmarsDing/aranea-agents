<template>
  <q-page :class="['agent-categories-page', { 'is-dark': isDark }]">
    <section class="category-hero">
      <div>
        <div class="category-kicker">Agent Type</div>
        <h1 class="category-title">Agent 行业分类</h1>
        <p class="category-subtitle">按行业、部门、职位三层组织 Agent 业务画像。创建 Agent 时仅绑定职位叶子，列表筛选同源读取数据库。</p>
      </div>
      <div class="category-actions">
        <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadTree" />
        <q-btn color="primary" rounded unelevated icon="add" label="新增行业" @click="openCreate('industry')" />
      </div>
    </section>

    <q-card flat bordered class="category-toolbar">
      <q-card-section class="row q-col-gutter-md items-center">
        <div class="col-12 col-md-5">
          <q-input v-model="keyword" dense outlined clearable debounce="200" placeholder="搜索行业、部门或职位..." class="category-control">
            <template #prepend><q-icon name="search" /></template>
          </q-input>
        </div>
        <div class="col-12 col-md">
          <div class="category-stats">
            <div><strong>{{ stats.industries }}</strong><span>行业</span></div>
            <div><strong>{{ stats.departments }}</strong><span>部门</span></div>
            <div><strong>{{ stats.positions }}</strong><span>职位</span></div>
          </div>
        </div>
        <div class="col-12 col-md-auto">
          <q-toggle v-model="onlyCustom" color="primary" label="仅看自建" />
        </div>
      </q-card-section>
    </q-card>

    <div v-if="loading" class="category-grid q-mt-lg">
      <q-card v-for="i in 3" :key="i" flat bordered class="industry-card">
        <q-card-section>
          <q-skeleton type="text" width="42%" />
          <q-skeleton class="q-mt-md" height="80px" />
        </q-card-section>
      </q-card>
    </div>

    <q-card v-else-if="filteredTree.length === 0" flat bordered class="category-empty q-mt-lg">
      <q-card-section class="column items-center text-center">
        <div class="category-empty__visual"><q-icon name="account_tree" size="44px" color="primary" /></div>
        <div class="text-h6 q-mt-md">暂无匹配分类</div>
        <div class="text-body2 text-grey-7 q-mt-sm">创建第一个行业，再添加部门与职位。</div>
        <q-btn class="q-mt-md" color="primary" rounded unelevated icon="add" label="新增行业" @click="openCreate('industry')" />
      </q-card-section>
    </q-card>

    <section v-else class="category-grid q-mt-lg">
      <q-card v-for="industry in filteredTree" :key="industry.id" flat bordered class="industry-card">
        <q-card-section class="industry-card__header">
          <div class="row items-start q-gutter-md no-wrap">
            <q-avatar rounded color="primary" text-color="white" icon="domain" class="industry-avatar" />
            <div class="col min-width-0">
              <div class="row items-center q-gutter-sm">
                <div class="text-h6 ellipsis">{{ industry.name }}</div>
                <q-chip dense square :class="isSystem(industry) ? 'system-chip' : 'custom-chip'">{{ isSystem(industry) ? "系统" : "自建" }}</q-chip>
              </div>
              <div v-if="trimmedDesc(industry.description)" class="text-body2 text-grey-8 q-mt-xs category-desc-line">{{ trimmedDesc(industry.description) }}</div>
              <div v-else class="text-caption text-grey-7">暂无行业描述 · 可在编辑中补充</div>
            </div>
          </div>
          <div class="row q-gutter-xs">
            <q-btn flat dense round color="primary" icon="edit" @click="openEdit(industry)" />
            <q-btn flat dense rounded color="primary" icon="add" label="部门" @click="openCreate('department', industry)" />
            <q-btn flat dense round color="negative" icon="delete" @click="removeNode(industry)" />
          </div>
        </q-card-section>

        <q-card-section class="department-list">
          <q-expansion-item
            v-for="department in industry.children"
            :key="department.id"
            default-opened
            expand-icon="keyboard_arrow_down"
            class="department-item"
          >
            <template #header>
              <q-item-section avatar><q-icon name="lan" color="primary" /></q-item-section>
              <q-item-section>
                <q-item-label class="text-weight-bold">{{ department.name }}</q-item-label>
                <q-item-label v-if="trimmedDesc(department.description)" caption class="category-dept-desc">{{ trimmedDesc(department.description) }}</q-item-label>
                <q-item-label v-else caption class="text-grey-6">暂无部门描述</q-item-label>
              </q-item-section>
              <q-item-section side>
                <div class="row q-gutter-xs">
                  <q-btn flat dense round color="primary" icon="edit" @click.stop="openEdit(department)" />
                  <q-btn flat dense rounded color="primary" icon="add" label="职位" @click.stop="openCreate('position', department)" />
                  <q-btn flat dense round color="negative" icon="delete" @click.stop="removeNode(department)" />
                </div>
              </q-item-section>
            </template>

            <div class="position-list">
              <div v-for="position in department.children" :key="position.id" class="position-item">
                <div class="row items-center q-gutter-sm">
                  <q-icon name="badge" color="primary" />
                  <div class="col min-width-0">
                    <div class="text-weight-medium">{{ position.name }}</div>
                    <div class="text-caption text-grey-7 position-path">{{ fullPath(industry, department, position) }}</div>
                    <div v-if="positionDescChain(industry, department, position)" class="text-caption position-desc-chain q-mt-xs">
                      {{ positionDescChain(industry, department, position) }}
                    </div>
                  </div>
                </div>
                <div class="row q-gutter-xs position-item__actions">
                  <q-btn flat dense round color="primary" icon="edit" @click="openEdit(position)" />
                  <q-btn flat dense round color="negative" icon="delete" @click="removeNode(position)" />
                </div>
              </div>
              <q-btn flat rounded color="primary" icon="add" label="新增职位" class="q-mt-sm" @click="openCreate('position', department)" />
            </div>
          </q-expansion-item>

          <q-btn flat rounded color="primary" icon="add" label="新增部门" class="q-mt-sm" @click="openCreate('department', industry)" />
        </q-card-section>
      </q-card>
    </section>

    <q-dialog v-model="dialogOpen" persistent>
      <q-card class="category-dialog">
        <q-card-section class="row items-center justify-between">
          <div>
            <div class="text-h6">{{ editingId ? "编辑分类" : `新增${levelLabel(form.level)}` }}</div>
            <div class="text-caption text-grey-7">固定三层结构：行业 → 部门 → 职位。</div>
          </div>
          <q-btn flat round icon="close" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section class="row q-col-gutter-md">
          <q-input v-model.trim="form.name" class="col-12 col-md-7 category-control" dense outlined label="名称 *" />
          <q-input v-model.number="form.sort_order" class="col-12 col-md-5 category-control" dense outlined type="number" label="排序" />
          <q-input :model-value="levelLabel(form.level)" class="col-12 col-md-6 category-control" dense outlined readonly label="层级" />
          <q-input :model-value="parentName" class="col-12 col-md-6 category-control" dense outlined readonly label="父级" />
          <q-input v-model="form.description" class="col-12 category-control" dense outlined autogrow type="textarea" label="描述" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat rounded label="取消" v-close-popup />
          <q-btn color="primary" rounded unelevated label="保存" :loading="saving" @click="saveNode" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useQuasar } from "quasar";
import {
  createPlatformResource,
  deletePlatformResource,
  listPlatformResourceTree,
  updatePlatformResource,
  type PlatformResourceTreeNode,
  type PlatformResourceInput
} from "../features/platform/api";

type CategoryLevel = "industry" | "department" | "position";

const $q = useQuasar();
const isDark = computed(() => $q.dark.isActive);
const loading = ref(false);
const saving = ref(false);
const keyword = ref("");
const onlyCustom = ref(false);
const dialogOpen = ref(false);
const editingId = ref("");
const parentNode = ref<PlatformResourceTreeNode | null>(null);
const tree = ref<PlatformResourceTreeNode[]>([]);

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

const filteredTree = computed(() => filterNodes(tree.value.filter((node) => node.level === "industry")));
const stats = computed(() => {
  const rows = flatten(tree.value);
  return {
    industries: rows.filter((row) => row.level === "industry").length,
    departments: rows.filter((row) => row.level === "department").length,
    positions: rows.filter((row) => row.level === "position").length
  };
});
const parentName = computed(() => parentNode.value?.name || "无");

onMounted(loadTree);

async function loadTree() {
  loading.value = true;
  try {
    tree.value = await listPlatformResourceTree("agent-categories");
  } finally {
    loading.value = false;
  }
}

function filterNodes(nodes: PlatformResourceTreeNode[]): PlatformResourceTreeNode[] {
  const q = keyword.value.trim().toLowerCase();
  return nodes
    .map((node) => ({
      ...node,
      children: filterNodes(node.children ?? [])
    }))
    .filter((node) => {
      const matchKeyword = !q || [node.name, node.description, node.key].some((value) => (value || "").toLowerCase().includes(q)) || (node.children?.length ?? 0) > 0;
      const matchCustom = !onlyCustom.value || !isSystem(node) || (node.children?.length ?? 0) > 0;
      return matchKeyword && matchCustom;
    });
}

function flatten(nodes: PlatformResourceTreeNode[]): PlatformResourceTreeNode[] {
  return nodes.flatMap((node) => [node, ...flatten(node.children ?? [])]);
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
      await updatePlatformResource("agent-categories", editingId.value, payload);
    } else {
      await createPlatformResource("agent-categories", payload);
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
  if (isSystem(node)) {
    $q.notify({ type: "warning", message: "系统预置分类不可删除" });
    return;
  }
  if ((node.children?.length ?? 0) > 0) {
    $q.notify({ type: "warning", message: "请先删除或迁移子分类" });
    return;
  }
  try {
    await deletePlatformResource("agent-categories", node.id);
    await loadTree();
    $q.notify({ type: "positive", message: "已删除分类" });
  } catch (error) {
    $q.notify({ type: "negative", message: errorMessage(error) || "删除分类失败" });
  }
}

function nextSortOrder(parent?: PlatformResourceTreeNode) {
  const siblings = parent ? parent.children ?? [] : tree.value.filter((node) => node.level === "industry");
  return siblings.length > 0 ? Math.max(...siblings.map((node) => node.sort_order || 0)) + 10 : 10;
}

function findNode(id: string) {
  if (!id) return null;
  return flatten(tree.value).find((node) => node.id === id) ?? null;
}

function isSystem(node: PlatformResourceTreeNode) {
  try {
    return Boolean(JSON.parse(node.metadata_json || "{}").is_system);
  } catch {
    return false;
  }
}

function levelLabel(level: string) {
  const labels: Record<string, string> = { industry: "行业", department: "部门", position: "职位" };
  return labels[level] ?? "分类";
}

function fullPath(industry: PlatformResourceTreeNode, department: PlatformResourceTreeNode, position: PlatformResourceTreeNode) {
  return `${industry.name} / ${department.name} / ${position.name}`;
}

/** 名称路径下一行：三层「描述」串联（空则跳过），与截图中 行业/部门/职位 信息一致。 */
function positionDescChain(
  industry: PlatformResourceTreeNode,
  department: PlatformResourceTreeNode,
  position: PlatformResourceTreeNode
) {
  const parts = [trimmedDesc(industry.description), trimmedDesc(department.description), trimmedDesc(position.description)].filter(Boolean);
  return parts.join(" / ");
}

function trimmedDesc(raw?: string | null) {
  const s = (raw ?? "").trim();
  return s;
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
</script>

<style scoped>
.agent-categories-page {
  min-height: 100%;
  padding: 28px;
  background:
    radial-gradient(circle at 86% 0%, rgba(25, 118, 210, 0.12), transparent 28%),
    radial-gradient(circle at 10% 16%, rgba(245, 158, 11, 0.09), transparent 24%),
    linear-gradient(180deg, #fbfcff 0%, #f7f9fc 46%, #ffffff 100%);
}

.category-hero,
.category-actions,
.category-stats,
.industry-card__header {
  display: flex;
  align-items: center;
}

.category-hero {
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.category-kicker {
  display: inline-flex;
  align-items: center;
  height: 28px;
  padding: 0 12px;
  border: 1px solid rgba(25, 118, 210, 0.14);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.78);
  color: #155ebc;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  box-shadow: 0 8px 24px rgba(16, 24, 40, 0.04);
}

.category-title {
  margin: 12px 0 0;
  color: #101828;
  font-size: clamp(32px, 5vw, 52px);
  font-weight: 800;
  letter-spacing: -0.055em;
  line-height: 1;
}

.category-subtitle {
  max-width: 720px;
  margin: 10px 0 0;
  color: #5f6b7a;
  font-size: 15px;
  line-height: 1.65;
}

.category-actions {
  gap: 10px;
  flex-wrap: wrap;
}

.category-toolbar,
.industry-card,
.category-empty {
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.86);
  box-shadow: 0 18px 48px rgba(16, 24, 40, 0.06);
  backdrop-filter: blur(16px);
}

.category-toolbar {
  margin-top: 22px;
}

.category-control :deep(.q-field__control) {
  min-height: 44px;
  border-radius: 16px;
  background: #ffffff;
}

.category-control :deep(.q-field__control::before) {
  border-color: rgba(15, 23, 42, 0.12);
}

.category-stats {
  gap: 10px;
  flex-wrap: wrap;
}

.category-stats div {
  min-width: 96px;
  padding: 10px 14px;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 16px;
  background: #fbfcff;
}

.category-stats strong {
  display: block;
  color: #101828;
  font-size: 20px;
  line-height: 1;
}

.category-stats span {
  color: #667085;
  font-size: 12px;
  font-weight: 700;
}

.category-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 18px;
}

.industry-card {
  overflow: hidden;
}

.industry-card__header {
  justify-content: space-between;
  gap: 14px;
  padding: 18px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(251, 252, 255, 0.92)),
    radial-gradient(circle at top right, rgba(25, 118, 210, 0.08), transparent 34%);
}

.industry-avatar {
  box-shadow: 0 14px 34px rgba(25, 118, 210, 0.2);
}

.system-chip,
.custom-chip {
  font-weight: 700;
}

.system-chip {
  border: 1px solid rgba(25, 118, 210, 0.18);
  background: #eef6ff;
  color: #155ebc;
}

.custom-chip {
  border: 1px solid rgba(34, 197, 94, 0.18);
  background: #ecfdf3;
  color: #027a48;
}

.department-list {
  padding: 14px;
}

.department-item {
  margin-bottom: 10px;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 18px;
  background: #fbfcff;
  overflow: hidden;
}

.position-list {
  display: grid;
  gap: 8px;
  padding: 0 14px 14px 64px;
}

.position-item {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 14px;
  background: #ffffff;
}

.position-path {
  line-height: 1.35;
}

.position-desc-chain {
  color: #475467;
  line-height: 1.5;
}

.category-desc-line {
  line-height: 1.55;
}

.position-item__actions {
  flex-shrink: 0;
  align-self: center;
}

.category-empty {
  background:
    radial-gradient(circle at center 26%, rgba(25, 118, 210, 0.08), transparent 22%),
    linear-gradient(180deg, #ffffff, #fbfcff);
}

.category-empty :deep(.q-card__section) {
  min-height: 250px;
  padding: 38px 24px;
}

.category-empty__visual {
  display: grid;
  place-items: center;
  width: 108px;
  height: 108px;
  border: 1px solid rgba(25, 118, 210, 0.12);
  border-radius: 32px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.9), rgba(238, 246, 255, 0.9));
  box-shadow: 0 18px 42px rgba(16, 24, 40, 0.08);
}

.category-dialog {
  width: 640px;
  max-width: 94vw;
  border-radius: 24px;
  overflow: hidden;
}

.category-dialog :deep(.q-card__actions) {
  padding: 14px 22px 20px;
  background: rgba(248, 250, 252, 0.58);
}

.min-width-0 {
  min-width: 0;
}

.agent-categories-page.is-dark {
  background:
    radial-gradient(circle at 86% 0%, rgba(59, 130, 246, 0.16), transparent 30%),
    radial-gradient(circle at 10% 16%, rgba(245, 158, 11, 0.1), transparent 24%),
    linear-gradient(160deg, #0b1220 0%, #111827 48%, #0f172a 100%);
  color: #e5e7eb;
}

.agent-categories-page.is-dark .category-kicker {
  border-color: rgba(96, 165, 250, 0.22);
  background: rgba(15, 23, 42, 0.74);
  color: #93c5fd;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.28);
}

.agent-categories-page.is-dark .category-title {
  color: #f8fafc;
}

.agent-categories-page.is-dark .category-subtitle {
  color: #94a3b8;
}

.agent-categories-page.is-dark .category-toolbar,
.agent-categories-page.is-dark .industry-card,
.agent-categories-page.is-dark .category-empty,
.agent-categories-page.is-dark .category-dialog {
  border-color: rgba(148, 163, 184, 0.16);
  background: rgba(17, 24, 39, 0.88);
  box-shadow: 0 14px 38px rgba(0, 0, 0, 0.32);
}

.agent-categories-page.is-dark .category-control :deep(.q-field__control) {
  background: rgba(30, 41, 59, 0.76);
}

.agent-categories-page.is-dark .category-control :deep(.q-field__control::before) {
  border-color: rgba(148, 163, 184, 0.18);
}

.agent-categories-page.is-dark .category-stats div {
  border-color: rgba(148, 163, 184, 0.16);
  background: rgba(30, 41, 59, 0.72);
}

.agent-categories-page.is-dark .category-stats strong {
  color: #f8fafc;
}

.agent-categories-page.is-dark .category-stats span {
  color: #94a3b8;
}

.agent-categories-page.is-dark .industry-card__header {
  background:
    linear-gradient(180deg, rgba(17, 24, 39, 0.96), rgba(15, 23, 42, 0.9)),
    radial-gradient(circle at top right, rgba(59, 130, 246, 0.14), transparent 34%);
}

.agent-categories-page.is-dark .system-chip {
  border-color: rgba(96, 165, 250, 0.28);
  background: rgba(30, 64, 175, 0.24);
  color: #93c5fd;
}

.agent-categories-page.is-dark .custom-chip {
  border-color: rgba(34, 197, 94, 0.28);
  background: rgba(22, 101, 52, 0.24);
  color: #86efac;
}

.agent-categories-page.is-dark .department-item {
  border-color: rgba(148, 163, 184, 0.14);
  background: rgba(15, 23, 42, 0.76);
}

.agent-categories-page.is-dark .position-item {
  border-color: rgba(148, 163, 184, 0.14);
  background: rgba(30, 41, 59, 0.7);
}

.agent-categories-page.is-dark .position-desc-chain,
.agent-categories-page.is-dark .category-dept-desc,
.agent-categories-page.is-dark .category-desc-line {
  color: #94a3b8;
}

.agent-categories-page.is-dark .category-empty {
  background:
    radial-gradient(circle at center 26%, rgba(59, 130, 246, 0.12), transparent 22%),
    linear-gradient(180deg, rgba(17, 24, 39, 0.94), rgba(15, 23, 42, 0.92));
}

.agent-categories-page.is-dark .category-empty__visual {
  border-color: rgba(96, 165, 250, 0.2);
  background: linear-gradient(180deg, rgba(30, 41, 59, 0.86), rgba(15, 23, 42, 0.9));
  box-shadow: 0 18px 42px rgba(0, 0, 0, 0.32);
}

.agent-categories-page.is-dark .category-dialog :deep(.q-card__actions) {
  background: rgba(15, 23, 42, 0.72);
}

@media (max-width: 599px) {
  .agent-categories-page {
    padding: 18px;
  }

  .category-grid {
    grid-template-columns: 1fr;
  }

  .position-list {
    padding-left: 14px;
  }
}
</style>
