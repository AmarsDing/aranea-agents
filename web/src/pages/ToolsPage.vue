<template>
  <q-page class="app-page-cream tools-page">
    <section class="tools-hero">
      <div>
        <div class="tools-kicker">Tool registry</div>
        <h1 class="tools-title">Tools 管理</h1>
        <p class="tools-subtitle">统一管理 Tool 元数据、运行时绑定、风险策略、配置 Schema 与调用记录。</p>
      </div>
      <div class="row q-gutter-sm">
        <q-btn outline rounded color="primary" icon="history" label="调用记录" :to="{ name: 'tool-runs' }" />
        <q-btn rounded color="primary" icon="add" label="新建 Tool" @click="openCreate" />
      </div>
    </section>

    <div class="row q-col-gutter-md q-mb-md">
      <div v-for="card in summaryCards" :key="card.label" class="col-12 col-sm-6 col-lg-3">
        <q-card flat bordered class="tools-summary-card">
          <q-card-section>
            <div class="text-caption text-grey-7">{{ card.label }}</div>
            <div class="text-h5 text-weight-bold q-mt-xs">{{ card.value }}</div>
            <div class="text-caption text-grey-7 q-mt-xs">{{ card.hint }}</div>
          </q-card-section>
        </q-card>
      </div>
    </div>

    <q-card flat bordered class="tools-filter-card q-mb-md">
      <q-card-section class="row q-col-gutter-sm items-center">
        <div class="col-12 col-md-4">
          <q-input v-model="search" dense outlined clearable debounce="350" placeholder="搜索 Tool 名称、Key、描述...">
            <template #prepend><q-icon name="search" /></template>
          </q-input>
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-select v-model="category" dense outlined clearable emit-value map-options label="分类" :options="categoryOptions" />
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-select v-model="riskLevel" dense outlined clearable emit-value map-options label="风险" :options="riskOptions" />
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-select v-model="enabled" dense outlined clearable emit-value map-options label="启用状态" :options="enabledOptions" />
        </div>
        <div class="col-12 col-md-2 row justify-end q-gutter-sm">
          <q-btn flat rounded icon="restart_alt" label="重置" @click="resetFilters" />
          <q-btn flat rounded icon="refresh" label="刷新" :loading="loading" @click="loadRows" />
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <q-table flat bordered class="tools-table" row-key="id" :rows="rows" :columns="columns" :loading="loading" :pagination="tablePagination" hide-pagination>
      <template #body-cell-name="props">
        <q-td :props="props">
          <div class="text-weight-medium">{{ props.row.display_name }}</div>
          <div class="text-caption text-grey-7">{{ props.row.key }}</div>
        </q-td>
      </template>

      <template #body-cell-category="props">
        <q-td :props="props">
          <q-chip dense color="primary" text-color="white">{{ props.row.category || "custom" }}</q-chip>
          <q-chip dense outline color="grey" class="q-ml-xs">{{ props.row.source || "external" }}</q-chip>
        </q-td>
      </template>

      <template #body-cell-risk="props">
        <q-td :props="props">
          <q-badge rounded :color="riskColor(props.row.risk_level)">{{ riskLabel(props.row.risk_level) }}</q-badge>
          <q-badge v-if="props.row.requires_confirmation" rounded color="warning" class="q-ml-xs">需确认</q-badge>
        </q-td>
      </template>

      <template #body-cell-runtime="props">
        <q-td :props="props">
          <q-badge rounded :color="props.row.runtime_status === 'catalog_only' ? 'grey' : 'positive'">
            {{ props.row.runtime_status || "available" }}
          </q-badge>
          <div class="text-caption text-grey-7 q-mt-xs">{{ props.row.supports_streaming ? "streaming" : "function" }}</div>
        </q-td>
      </template>

      <template #body-cell-enabled="props">
        <q-td :props="props">
          <q-toggle dense color="primary" :model-value="props.row.enabled" :disable="busyId === props.row.id" @update:model-value="toggleEnabled(props.row as Tool, Boolean($event))" />
        </q-td>
      </template>

      <template #body-cell-stats="props">
        <q-td :props="props">
          <div class="text-weight-medium">{{ props.row.invoke_count }} 次</div>
          <div class="text-caption text-grey-7">24h {{ props.row.invoke_count_24h }} · 失败 {{ props.row.failure_count }}</div>
        </q-td>
      </template>

      <template #body-cell-actions="props">
        <q-td :props="props" class="q-gutter-xs">
          <q-btn flat dense round color="primary" icon="visibility" @click="openDetail(props.row as Tool)">
            <q-tooltip>查看</q-tooltip>
          </q-btn>
          <q-btn flat dense round color="primary" icon="edit" @click="openEdit(props.row as Tool)">
            <q-tooltip>编辑</q-tooltip>
          </q-btn>
          <q-btn flat dense round color="negative" icon="delete" :loading="busyId === props.row.id" @click="removeTool(props.row as Tool)">
            <q-tooltip>删除</q-tooltip>
          </q-btn>
        </q-td>
      </template>
    </q-table>

    <skill-pagination v-model:page="page" v-model:page-size="pageSize" :page-max="pageMax" :total="total" :loading="loading" label="个 Tool" />

    <q-dialog v-model="detailOpen">
      <q-card class="tool-detail-card">
        <q-card-section class="row items-start justify-between q-gutter-md">
          <div>
            <div class="text-h6">{{ detailTarget?.display_name }}</div>
            <div class="text-caption text-grey-7">{{ detailTarget?.key }}</div>
          </div>
          <q-btn flat dense round icon="close" aria-label="关闭详情" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section v-if="detailTarget" class="q-gutter-md">
          <q-banner rounded class="bg-grey-2 text-grey-9">{{ detailTarget.description || "暂无描述" }}</q-banner>
          <div class="row q-col-gutter-sm">
            <div class="col-6"><b>分类：</b>{{ detailTarget.category }}</div>
            <div class="col-6"><b>来源：</b>{{ detailTarget.source }}</div>
            <div class="col-6"><b>风险：</b>{{ riskLabel(detailTarget.risk_level) }}</div>
            <div class="col-6"><b>运行时：</b>{{ detailTarget.runtime_status || "available" }}</div>
            <div class="col-6"><b>调用次数：</b>{{ detailTarget.invoke_count }}</div>
            <div class="col-6"><b>成功 / 失败：</b>{{ detailTarget.success_count }} / {{ detailTarget.failure_count }}</div>
          </div>
          <q-expansion-item dense-toggle default-open label="参数 Schema"><pre class="tool-schema">{{ prettyJSON(detailTarget.parameters_schema_json) }}</pre></q-expansion-item>
          <q-expansion-item dense-toggle label="返回 Schema"><pre class="tool-schema">{{ prettyJSON(detailTarget.result_schema_json) }}</pre></q-expansion-item>
          <q-expansion-item dense-toggle label="配置 JSON"><pre class="tool-schema">{{ prettyJSON(detailTarget.config_json) }}</pre></q-expansion-item>
          <q-expansion-item dense-toggle label="默认配置"><pre class="tool-schema">{{ prettyJSON(detailTarget.default_config_json) }}</pre></q-expansion-item>
          <q-expansion-item dense-toggle label="元数据"><pre class="tool-schema">{{ prettyJSON(detailTarget.metadata_json) }}</pre></q-expansion-item>
          <q-btn flat color="primary" icon="history" label="查看调用记录" :to="{ name: 'tool-runs', query: { tool_key: detailTarget.key } }" v-close-popup />
        </q-card-section>
      </q-card>
    </q-dialog>

    <q-dialog v-model="editorOpen" persistent>
      <q-card class="tool-editor-card">
        <q-card-section class="row items-center justify-between">
          <div class="text-h6">{{ editingId ? "编辑 Tool" : "新建 Tool" }}</div>
          <q-btn flat dense round icon="close" :disable="saving" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section class="q-gutter-md">
          <div class="row q-col-gutter-sm">
            <q-input class="col-12 col-md-6" v-model="form.key" dense outlined label="Key" :disable="Boolean(editingId)" />
            <q-input class="col-12 col-md-6" v-model="form.display_name" dense outlined label="显示名称" />
            <q-input class="col-12" v-model="form.description" dense outlined autogrow label="描述" />
            <q-input class="col-12 col-md-4" v-model="form.category" dense outlined label="分类" />
            <q-input class="col-12 col-md-4" v-model="form.source" dense outlined label="来源" />
            <q-select class="col-12 col-md-4" v-model="form.risk_level" dense outlined emit-value map-options label="风险" :options="riskOptions" />
          </div>
          <div class="row q-col-gutter-sm">
            <q-toggle class="col-auto" v-model="form.enabled" label="启用" />
            <q-toggle class="col-auto" v-model="form.readonly" label="只读" />
            <q-toggle class="col-auto" v-model="form.requires_confirmation" label="需确认" />
            <q-toggle class="col-auto" v-model="form.supports_streaming" label="流式" />
            <q-toggle class="col-auto" v-model="form.supports_concurrency" label="并发" />
          </div>
          <q-input v-model="form.parameters_schema_json" type="textarea" outlined label="参数 Schema JSON" :error="Boolean(jsonErrors.parameters_schema_json)" :error-message="jsonErrors.parameters_schema_json" />
          <q-input v-model="form.result_schema_json" type="textarea" outlined label="返回 Schema JSON" :error="Boolean(jsonErrors.result_schema_json)" :error-message="jsonErrors.result_schema_json" />
          <q-input v-model="form.config_json" type="textarea" outlined label="配置 JSON" :error="Boolean(jsonErrors.config_json)" :error-message="jsonErrors.config_json" />
          <q-input v-model="form.default_config_json" type="textarea" outlined label="默认配置 JSON" :error="Boolean(jsonErrors.default_config_json)" :error-message="jsonErrors.default_config_json" />
          <q-input v-model="form.metadata_json" type="textarea" outlined label="元数据 JSON" :error="Boolean(jsonErrors.metadata_json)" :error-message="jsonErrors.metadata_json" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="取消" :disable="saving" v-close-popup />
          <q-btn color="primary" label="保存" :loading="saving" @click="saveTool" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { useQuasar, type QTableColumn } from "quasar";
import SkillPagination from "../features/skills/components/SkillPagination.vue";
import { createTool, deleteTool, getTool, listTools, toggleToolEnabled, updateTool } from "../features/tools/api";
import type { Tool, ToolSummary, ToolUpsertInput } from "../features/tools/types";

const $q = useQuasar();
const search = ref("");
const category = ref("");
const riskLevel = ref("");
const enabled = ref<boolean | null>(null);
const page = ref(1);
const pageSize = ref(20);
const rows = ref<Tool[]>([]);
const total = ref(0);
const summary = ref<ToolSummary>({ total_tools: 0, enabled_tools: 0, high_risk_enabled: 0, calls_24h: 0, failure_rate_24h: 0 });
const loading = ref(false);
const error = ref("");
const busyId = ref("");
const detailOpen = ref(false);
const detailTarget = ref<Tool | null>(null);
const editorOpen = ref(false);
const editingId = ref("");
const saving = ref(false);
const jsonErrors = reactive<Record<string, string>>({});

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
const summaryCards = computed(() => [
  { label: "总工具", value: summary.value.total_tools, hint: "已注册 Tool" },
  { label: "已启用", value: summary.value.enabled_tools, hint: "全局启用" },
  { label: "高风险启用", value: summary.value.high_risk_enabled, hint: "high / critical" },
  { label: "24h 调用", value: summary.value.calls_24h, hint: `失败率 ${(summary.value.failure_rate_24h * 100).toFixed(1)}%` }
]);
const categoryOptions = ["system", "web", "filesystem", "skill", "memory", "media", "runtime", "custom"].map((value) => ({ label: value, value }));
const riskOptions = [
  { label: "低", value: "low" },
  { label: "中", value: "medium" },
  { label: "高", value: "high" },
  { label: "严重", value: "critical" }
];
const enabledOptions = [
  { label: "已启用", value: true },
  { label: "已禁用", value: false }
];
const tablePagination = { rowsPerPage: 0 };
const columns: QTableColumn<Tool>[] = [
  { name: "name", label: "Tool", field: "display_name", align: "left" },
  { name: "category", label: "分类 / 来源", field: "category", align: "left" },
  { name: "risk", label: "风险", field: "risk_level", align: "left" },
  { name: "runtime", label: "运行时", field: "runtime_status", align: "left" },
  { name: "enabled", label: "启用", field: "enabled", align: "left" },
  { name: "stats", label: "调用", field: "invoke_count", align: "left" },
  { name: "actions", label: "操作", field: "id", align: "right" }
];

async function loadRows() {
  loading.value = true;
  error.value = "";
  try {
    const data = await listTools({ search: search.value, category: category.value, risk_level: riskLevel.value, enabled: enabled.value, page: page.value, page_size: pageSize.value });
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
  for (const key of ["parameters_schema_json", "result_schema_json", "config_json", "default_config_json", "metadata_json"] as const) {
    try {
      JSON.parse(form[key] || "{}");
    } catch (err) {
      jsonErrors[key] = err instanceof Error ? err.message : "JSON 格式错误";
    }
  }
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

function prettyJSON(raw: string) {
  try {
    return JSON.stringify(JSON.parse(raw || "{}"), null, 2);
  } catch {
    return raw || "{}";
  }
}

function riskLabel(value: string) {
  return ({ low: "低", medium: "中", high: "高", critical: "严重" } as Record<string, string>)[value] ?? value;
}

function riskColor(value: string) {
  return ({ low: "positive", medium: "warning", high: "orange", critical: "negative" } as Record<string, string>)[value] ?? "grey";
}

watch([search, category, riskLevel, enabled], () => {
  page.value = 1;
  void loadRows();
});
watch([page, pageSize], () => void loadRows());
onMounted(loadRows);
</script>

<style scoped>
.tools-page {
  padding: 24px;
}

.tools-hero {
  align-items: center;
  display: flex;
  justify-content: space-between;
  margin-bottom: 20px;
  gap: 16px;
}

.tools-kicker {
  color: var(--q-primary);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.tools-title {
  font-size: 32px;
  line-height: 1.1;
  margin: 4px 0;
}

.tools-subtitle {
  color: #6f6a60;
  margin: 0;
}

.tools-summary-card,
.tools-filter-card,
.tools-table,
.tool-detail-card,
.tool-editor-card {
  border-radius: 18px;
}

.tool-detail-card,
.tool-editor-card {
  max-width: 960px;
  width: 92vw;
}

.tool-schema {
  background: #151515;
  border-radius: 12px;
  color: #f5f5f5;
  max-height: 320px;
  overflow: auto;
  padding: 12px;
  white-space: pre-wrap;
}
</style>

