<template>
  <q-page class="app-standard-page app-registry-page tools-page">
    <tool-hero-section kicker="Tool registry" title="Tools 管理" subtitle="统一管理 Tool 元数据、运行时绑定、风险策略、配置 Schema 与调用记录。">
      <template #actions>
        <q-btn outline rounded no-caps class="app-outline-btn" icon="policy" label="审计日志" :to="{ name: 'tool-audits' }" />
        <q-btn outline rounded no-caps class="app-outline-btn" icon="history" label="调用记录" :to="{ name: 'tool-runs' }" />
        <q-btn rounded no-caps unelevated class="app-accent-btn" icon="add" label="新建 Tool" @click="editorStore.openCreate()" />
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

    <q-banner v-if="error" rounded class="app-page-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense label="重试" class="text-white" @click="loadRows" />
      </template>
    </q-banner>

    <tools-table
      :rows="rows"
      :loading="loading"
      :busy-id="busyId"
      :selected="selected"
      @update:selected="selected = $event"
      @toggleEnabled="toggleEnabled"
      @updateRisk="updateRisk"
      @viewDetail="openDetail"
      @edit="editorStore.openEdit($event)"
      @remove="removeTool"
    />

    <div v-if="selected.length" class="tools-batch-bar q-pa-sm">
      <span class="text-caption q-mr-md">已选 {{ selected.length }} 项</span>
      <q-btn flat dense no-caps icon="toggle_on" label="批量启用" size="sm" class="app-registry-accent-btn" @click="batchToggle(true)" />
      <q-btn flat dense no-caps icon="toggle_off" label="批量停用" size="sm" @click="batchToggle(false)" />
      <q-btn flat dense no-caps icon="delete" label="批量删除" size="sm" color="negative" @click="batchRemove" />
    </div>

    <AppRegistryPagination v-model:page="page" v-model:page-size="pageSize" :page-max="pageMax" :total="total" :loading="loading" label="个 Tool" />

    <tool-detail-drawer />

    <tool-editor-dialog />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { storeToRefs } from "pinia";
import { useQuasar } from "quasar";
import AppRegistryPagination from "../components/layout/AppRegistryPagination.vue";
import ToolHeroSection from "../components/tools/ToolHeroSection.vue";
import ToolsMetricStrip from "../components/tools/ToolsMetricStrip.vue";
import ToolCatalogFilters from "../components/tools/ToolCatalogFilters.vue";
import ToolsTable from "../components/tools/ToolsTable.vue";
import ToolDetailDrawer from "../components/tools/ToolDetailDrawer.vue";
import ToolEditorDialog from "../components/tools/ToolEditorDialog.vue";
import {
  buildToolSummaryCards,
  categoryFilterOptions,
  enabledTriStateOptions,
  riskLevelOptions
} from "../components/tools/toolUi";
import { useToolDetailStore } from "../stores/tools/toolDetail";
import { useToolEditorStore } from "../stores/tools/toolEditor";
import { useToolToggle } from "../features/tools/useToolEditor";
import type { Tool } from "../features/tools/types";
import { useToolsStore } from "../stores/tools";

const $q = useQuasar();
const toolsStore = useToolsStore();
const detailStore = useToolDetailStore();
const editorStore = useToolEditorStore();
const { tools: rows, total, summary, loading } = storeToRefs(toolsStore);

const search = ref("");
const category = ref("");
const riskLevel = ref("");
const enabled = ref<boolean | null>(null);
const page = ref(1);
const pageSize = ref(20);
const error = ref("");
const selected = ref<Tool[]>([]);

const categoryOptions = categoryFilterOptions;
const riskOptions = riskLevelOptions;
const enabledOptions = enabledTriStateOptions;

const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
const summaryCards = computed(() => buildToolSummaryCards(summary.value));

async function loadRows() {
  error.value = "";
  try {
    await toolsStore.loadTools({
      search: search.value,
      category: category.value,
      risk_level: riskLevel.value,
      enabled: enabled.value,
      page: page.value,
      page_size: pageSize.value
    });
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载 Tools 失败";
  }
}

const { busyId, toggleEnabled, removeTool } = useToolToggle(loadRows);

editorStore.setCallbacks({
  onSaved: loadRows,
  onCreated: async (tool) => {
    const fetched = await toolsStore.fetchTool(tool.id || tool.key);
    detailStore.openDetail(fetched);
  }
});

function resetFilters() {
  search.value = "";
  category.value = "";
  riskLevel.value = "";
  enabled.value = null;
  page.value = 1;
  void loadRows();
}

async function openDetail(tool: Tool) {
  const fetched = await toolsStore.fetchTool(tool.id || tool.key);
  detailStore.openDetail(fetched);
}

async function updateRisk(tool: Tool, value: string) {
  try {
    await toolsStore.editTool(tool.id || tool.key, {
      key: tool.key,
      display_name: tool.display_name,
      description: tool.description,
      category: tool.category,
      source: tool.source,
      risk_level: value,
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
    $q.notify({ type: "positive", message: "风险级别已更新" });
    await loadRows();
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "更新风险级别失败" });
  }
}

async function batchToggle(value: boolean) {
  for (const tool of selected.value) {
    try {
      await toolsStore.toggle(tool.id || tool.key, value);
    } catch {
      // continue
    }
  }
  selected.value = [];
  await loadRows();
}

function batchRemove() {
  const count = selected.value.length;
  $q.dialog({
    title: "批量删除",
    message: `确认删除选中的 ${count} 个 Tool？`,
    cancel: true,
    persistent: true
  }).onOk(async () => {
    for (const tool of selected.value) {
      try {
        await toolsStore.remove(tool.id || tool.key);
      } catch {
        // continue
      }
    }
    selected.value = [];
    await loadRows();
  });
}

watch([search, category, riskLevel, enabled], () => {
  page.value = 1;
  void loadRows();
});
watch([page, pageSize], () => void loadRows());
onMounted(loadRows);
</script>
