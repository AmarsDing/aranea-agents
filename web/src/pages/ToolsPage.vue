<template>
  <q-page class="app-page-cream tools-page">
    <tool-hero-section kicker="Tool registry" title="Tools 管理" subtitle="统一管理 Tool 元数据、运行时绑定、风险策略、配置 Schema 与调用记录。">
      <template #actions>
        <q-btn outline rounded no-caps class="tool-outline-btn" icon="policy" label="审计日志" :to="{ name: 'tool-audits' }" />
        <q-btn outline rounded no-caps class="tool-outline-btn" icon="history" label="调用记录" :to="{ name: 'tool-runs' }" />
        <q-btn rounded no-caps unelevated class="tool-primary-btn" icon="add" label="新建 Tool" @click="openCreateTool()" />
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
      @edit="openEditTool"
      @remove="removeTool"
    />

    <skill-pagination v-model:page="page" v-model:page-size="pageSize" :page-max="pageMax" :total="total" :loading="loading" label="个 Tool" />

    <tool-detail-dialog v-model:open="detailOpen" :tool="detailTarget">
      <tool-detail-content
        :tool="detailTarget"
        :overrides="overrides"
        :overrides-loading="overridesLoading"
        :recent-runs="recentRuns"
        :runs-loading="runsLoading"
        :test-args-json="testArgsJson"
        :test-timeout-sec="testTimeoutSec"
        :test-running="testRunning"
        :test-result="testResult"
        :override-editor-open="overrideEditorOpen"
        :editing-override="editingOverride"
        :override-saving="overrideSaving"
        :override-form="overrideForm"
        @update:test-args-json="testArgsJson = $event"
        @update:test-timeout-sec="testTimeoutSec = $event"
        @run-test="runToolTest()"
        @edit-override="openOverrideEditor($event)"
        @delete-override="confirmRemoveOverride($event)"
        @update:override-editor-open="overrideEditorOpen = $event"
        @update:override-form="overrideForm = $event"
        @save-override="saveOverride()"
      />
    </tool-detail-dialog>

    <tool-editor-dialog
      v-model:open="editorOpen"
      :editing-id="editorEditingId"
      :saving="editorSaving"
      :form="editorForm"
      :errors="editorJsonErrors"
      :risk-options="editorRiskOptions"
      @save="saveEditor()"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { storeToRefs } from "pinia";
import SkillPagination from "../components/skills/SkillPagination.vue";
import ToolHeroSection from "../components/tools/ToolHeroSection.vue";
import ToolsMetricStrip from "../components/tools/ToolsMetricStrip.vue";
import ToolCatalogFilters from "../components/tools/ToolCatalogFilters.vue";
import ToolsTable from "../components/tools/ToolsTable.vue";
import ToolDetailContent from "../components/tools/ToolDetailContent.vue";
import ToolDetailDialog from "../components/tools/ToolDetailDialog.vue";
import ToolEditorDialog from "../components/tools/ToolEditorDialog.vue";
import {
  buildToolSummaryCards,
  categoryFilterOptions,
  enabledTriStateOptions,
  riskLevelOptions
} from "../components/tools/toolUi";
import { useToolDetailPanel } from "../features/tools/useToolDetailPanel";
import { useToolEditor, useToolToggle } from "../features/tools/useToolEditor";
import type { Tool } from "../features/tools/types";
import { useToolsStore } from "../stores/tools";

const toolsStore = useToolsStore();
const { tools: rows, total, summary, loading } = storeToRefs(toolsStore);
const search = ref("");
const category = ref("");
const riskLevel = ref("");
const enabled = ref<boolean | null>(null);
const page = ref(1);
const pageSize = ref(20);
const error = ref("");
const detailOpen = ref(false);
const detailTarget = ref<Tool | null>(null);

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

const {
  open: editorOpen,
  editingId: editorEditingId,
  saving: editorSaving,
  jsonErrors: editorJsonErrors,
  form: editorForm,
  riskOptions: editorRiskOptions,
  openCreate: openCreateTool,
  openEdit: openEditTool,
  save: saveEditor
} = useToolEditor(loadRows);
const { busyId, toggleEnabled, removeTool } = useToolToggle(loadRows);

const {
  overrides,
  overridesLoading,
  recentRuns,
  runsLoading,
  testArgsJson,
  testTimeoutSec,
  testRunning,
  testResult,
  overrideEditorOpen,
  editingOverride,
  overrideSaving,
  overrideForm,
  runToolTest,
  openOverrideEditor,
  saveOverride,
  confirmRemoveOverride
} = useToolDetailPanel(detailTarget);

function resetFilters() {
  search.value = "";
  category.value = "";
  riskLevel.value = "";
  enabled.value = null;
  page.value = 1;
  void loadRows();
}

async function openDetail(tool: Tool) {
  detailTarget.value = await toolsStore.fetchTool(tool.id || tool.key);
  detailOpen.value = true;
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

.tools-error-banner
  background: rgba(229, 92, 92, 0.92)
  color: var(--color-on-accent)
  border: 1px solid rgba(255, 255, 255, 0.25)

body.body--dark .tools-error-banner
  background: rgba(255, 94, 122, 0.22)
  color: var(--color-text-primary)
  border-color: rgba(255, 255, 255, 0.12)

.tool-primary-btn
  background: var(--color-accent)
  color: var(--color-on-accent)

body:not(.body--dark) .tool-primary-btn:hover
  background: var(--color-accent-hover)

.tool-outline-btn
  border-color: rgba(208, 192, 168, 0.85)
  color: var(--color-text-primary)

body:not(.body--dark) .tool-outline-btn:hover
  background: var(--interaction-surface-hover)
</style>
