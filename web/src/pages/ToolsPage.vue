<template>
  <q-page class="app-page-cream tools-page">
    <tool-hero-section kicker="Tool registry" title="Tools 管理" subtitle="统一管理 Tool 元数据、运行时绑定、风险策略、配置 Schema 与调用记录。">
      <template #actions>
        <q-btn outline rounded no-caps class="tool-outline-btn" icon="history" label="调用记录" :to="{ name: 'tool-runs' }" />
        <q-btn rounded no-caps unelevated class="tool-primary-btn" icon="add" label="新建 Tool" @click="openCreate" />
      </template>
    </tool-hero-section>

    <tools-metric-strip :cards="summaryCards" />

    <tool-catalog-filters
      :search="search"
      :category="category"
      :risk-level="riskLevel"
      :enabled="enabled"
      :category-options="categoryOptions"
      :risk-options="riskOptions"
      :enabled-options="enabledOptions"
      :loading="loading"
      @update:search="search = $event"
      @update:category="category = $event"
      @update:risk-level="riskLevel = $event"
      @update:enabled="enabled = $event"
      @reset="resetFilters"
      @refresh="loadRows"
    />

    <q-banner v-if="error" rounded class="tools-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense label="重试" class="text-white" @click="loadRows" />
      </template>
    </q-banner>

    <tools-table
      :rows="rows"
      :loading="loading"
      :busy-id="busyId"
      @toggleEnabled="toggleEnabled"
      @viewDetail="openDetail"
      @edit="openEdit"
      @remove="removeTool"
    />

    <skill-pagination v-model:page="page" v-model:page-size="pageSize" :page-max="pageMax" :total="total" :loading="loading" label="个 Tool" />

    <q-dialog v-model="detailOpen">
      <q-card class="tool-dialog-card tool-dialog-card--detail">
        <q-card-section class="row items-start justify-between q-gutter-md">
          <div>
            <div class="text-h6">{{ detailTarget?.display_name }}</div>
            <div class="text-caption muted-caption">{{ detailTarget?.key }}</div>
          </div>
          <q-btn flat dense round icon="close" aria-label="关闭详情" class="tool-icon-btn" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section>
          <tool-detail-content :tool="detailTarget" />
        </q-card-section>
      </q-card>
    </q-dialog>

    <q-dialog v-model="editorOpen" persistent>
      <q-card class="tool-dialog-card tool-dialog-card--editor">
        <q-card-section class="row items-center justify-between">
          <div class="text-h6">{{ editingId ? "编辑 Tool" : "新建 Tool" }}</div>
          <q-btn flat dense round icon="close" class="tool-icon-btn" :disable="saving" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section>
          <tool-editor-form :form="form" :editing-id="editingId" :errors="jsonErrors" :risk-options="riskOptions" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps label="取消" :disable="saving" v-close-popup />
          <q-btn no-caps unelevated class="tool-primary-btn" label="保存" :loading="saving" @click="saveTool" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { useQuasar } from "quasar";
import SkillPagination from "../components/skills/SkillPagination.vue";
import ToolHeroSection from "../components/tools/ToolHeroSection.vue";
import ToolsMetricStrip from "../components/tools/ToolsMetricStrip.vue";
import ToolCatalogFilters from "../components/tools/ToolCatalogFilters.vue";
import ToolsTable from "../components/tools/ToolsTable.vue";
import ToolDetailContent from "../components/tools/ToolDetailContent.vue";
import ToolEditorForm from "../components/tools/ToolEditorForm.vue";
import {
  buildToolSummaryCards,
  categoryFilterOptions,
  enabledTriStateOptions,
  riskLevelOptions,
  validateToolJsonFields,
  toolEditorJsonKeys
} from "../components/tools/toolUi";
import { createTool, deleteTool, getTool, listTools, toggleToolEnabled, updateTool } from "../features/tools/api";
import type { Tool, ToolUpsertInput } from "../features/tools/types";

const $q = useQuasar();
const search = ref("");
const category = ref("");
const riskLevel = ref("");
const enabled = ref<boolean | null>(null);
const page = ref(1);
const pageSize = ref(20);
const rows = ref<Tool[]>([]);
const total = ref(0);
const summary = ref({
  total_tools: 0,
  enabled_tools: 0,
  high_risk_enabled: 0,
  calls_24h: 0,
  failure_rate_24h: 0
});
const loading = ref(false);
const error = ref("");
const busyId = ref("");
const detailOpen = ref(false);
const detailTarget = ref<Tool | null>(null);
const editorOpen = ref(false);
const editingId = ref("");
const saving = ref(false);
const jsonErrors = reactive<Record<string, string>>({});

const categoryOptions = categoryFilterOptions;
const riskOptions = riskLevelOptions;
const enabledOptions = enabledTriStateOptions;

const blankForm = (): ToolUpsertInput => ({
  key: "",
  display_name: "",
  description: "",
  category: "custom",
  source: "external",
  risk_level: "low",
  enabled: true,
  readonly: false,
  requires_confirmation: false,
  supports_streaming: false,
  supports_concurrency: false,
  parameters_schema_json: "{}",
  result_schema_json: "{}",
  config_schema_json: "{}",
  config_json: "{}",
  default_config_json: "{}",
  metadata_json: "{}"
});
const form = reactive<ToolUpsertInput>(blankForm());

const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
const summaryCards = computed(() => buildToolSummaryCards(summary.value));

async function loadRows() {
  loading.value = true;
  error.value = "";
  try {
    const data = await listTools({
      search: search.value,
      category: category.value,
      risk_level: riskLevel.value,
      enabled: enabled.value,
      page: page.value,
      page_size: pageSize.value
    });
    rows.value = data.items;
    total.value = data.total;
    summary.value = data.summary;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载 Tools 失败";
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  search.value = "";
  category.value = "";
  riskLevel.value = "";
  enabled.value = null;
  page.value = 1;
  void loadRows();
}

async function openDetail(tool: Tool) {
  detailTarget.value = await getTool(tool.id || tool.key);
  detailOpen.value = true;
}

function assignForm(input: ToolUpsertInput) {
  Object.assign(form, input);
  Object.keys(jsonErrors).forEach((key) => delete jsonErrors[key]);
}

function openCreate() {
  editingId.value = "";
  assignForm(blankForm());
  editorOpen.value = true;
}

function openEdit(tool: Tool) {
  editingId.value = tool.id;
  assignForm({
    key: tool.key,
    display_name: tool.display_name,
    description: tool.description,
    category: tool.category,
    source: tool.source,
    risk_level: tool.risk_level,
    enabled: tool.enabled,
    readonly: tool.readonly,
    requires_confirmation: tool.requires_confirmation,
    supports_streaming: tool.supports_streaming,
    supports_concurrency: tool.supports_concurrency,
    parameters_schema_json: tool.parameters_schema_json || "{}",
    result_schema_json: tool.result_schema_json || "{}",
    config_schema_json: tool.config_schema_json || "{}",
    config_json: tool.config_json || "{}",
    default_config_json: tool.default_config_json || "{}",
    metadata_json: tool.metadata_json || "{}"
  });
  editorOpen.value = true;
}

function validateJSONFields() {
  Object.keys(jsonErrors).forEach((key) => delete jsonErrors[key]);
  const keys = [...toolEditorJsonKeys];
  const fieldObj = keys.reduce(
    (acc, k) => {
      acc[k] = form[k];
      return acc;
    },
    {} as Record<string, string>
  );
  const errs = validateToolJsonFields(fieldObj, keys);
  Object.assign(jsonErrors, errs);
  return Object.keys(jsonErrors).length === 0;
}

async function saveTool() {
  if (!validateJSONFields()) return;
  saving.value = true;
  try {
    if (editingId.value) {
      await updateTool(editingId.value, { ...form });
    } else {
      await createTool({ ...form });
    }
    editorOpen.value = false;
    await loadRows();
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存 Tool 失败" });
  } finally {
    saving.value = false;
  }
}

async function toggleEnabled(tool: Tool, value: boolean) {
  if (value && (tool.risk_level === "high" || tool.risk_level === "critical")) {
    $q.dialog({
      title: "高风险工具确认",
      message: `即将启用高风险工具「${tool.display_name}」（风险等级：${tool.risk_level}）。请输入工具 Key 以确认：${tool.key}`,
      prompt: { model: "", type: "text", label: "请输入 Tool Key" },
      cancel: true,
      persistent: true
    }).onOk(async (inputKey: string) => {
      if (inputKey !== tool.key) {
        $q.notify({ type: "negative", message: "输入的 Key 不匹配，操作已取消" });
        return;
      }
      busyId.value = tool.id;
      try {
        await toggleToolEnabled(tool.id || tool.key, value, tool.key);
        await loadRows();
      } catch (err) {
        $q.notify({ type: "negative", message: err instanceof Error ? err.message : "操作失败" });
      } finally {
        busyId.value = "";
      }
    });
    return;
  }
  busyId.value = tool.id;
  try {
    await toggleToolEnabled(tool.id || tool.key, value);
    await loadRows();
  } finally {
    busyId.value = "";
  }
}

function removeTool(tool: Tool) {
  $q.dialog({ title: "删除 Tool", message: `确认删除 ${tool.display_name}（${tool.key}）？`, cancel: true, persistent: true }).onOk(async () => {
    busyId.value = tool.id;
    try {
      await deleteTool(tool.id || tool.key);
      await loadRows();
    } finally {
      busyId.value = "";
    }
  });
}

watch([search, category, riskLevel, enabled], () => {
  page.value = 1;
  void loadRows();
});
watch([page, pageSize], () => void loadRows());
onMounted(loadRows);
</script>

<style scoped lang="sass">
.tools-page
  padding: 24px

.muted-caption
  color: var(--color-text-secondary)

.tools-error-banner
  background: rgba(229, 92, 92, 0.92)
  color: #fff
  border: 1px solid rgba(255, 255, 255, 0.25)

body.body--dark .tools-error-banner
  background: rgba(255, 94, 122, 0.22)
  color: var(--color-text-primary)
  border-color: rgba(255, 255, 255, 0.12)

.tool-dialog-card
  max-width: 960px
  width: 92vw
  border-radius: 22px
  border: 1px solid var(--glass-border)
  background: var(--glass-elevated)
  backdrop-filter: blur(var(--glass-blur-elevated))
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated))
  box-shadow: none

.tool-primary-btn
  background: var(--color-accent)
  color: #fff

body:not(.body--dark) .tool-primary-btn:hover
  background: var(--color-accent-hover)

.tool-outline-btn
  border-color: rgba(208, 192, 168, 0.85)
  color: var(--color-text-primary)

body:not(.body--dark) .tool-outline-btn:hover
  background: var(--interaction-surface-hover)

.tool-icon-btn
  color: var(--color-icon-muted)

body:not(.body--dark) .tool-icon-btn:hover
  color: var(--color-accent)
</style>
