<template>
  <q-page class="app-standard-page app-registry-page tools-page">
    <tool-hero-section
      kicker="Tool registry"
      title="Tools 管理"
      subtitle="统一管理 Tool 元数据、运行时绑定、风险策略、配置 Schema 与调用记录。"
    >
      <template #actions>
        <q-btn
          outline
          rounded
          no-caps
          class="app-outline-btn"
          icon="policy"
          label="审计日志"
          :to="{ name: 'tool-audits' }"
        />
        <q-btn
          outline
          rounded
          no-caps
          class="app-outline-btn"
          icon="history"
          label="调用记录"
          :to="{ name: 'tool-runs' }"
        />
        <q-btn
          rounded
          no-caps
          unelevated
          class="app-accent-btn"
          icon="add"
          label="新建 Tool"
          @click="editorStore.openCreate()"
        />
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
      @toggle-enabled="toggleEnabled"
      @update-risk="updateRisk"
      @view-detail="openDetail"
      @edit="editorStore.openEdit($event)"
      @remove="removeTool"
    />

    <div v-if="selected.length" class="tools-batch-bar q-pa-sm">
      <span class="text-caption q-mr-md">已选 {{ selected.length }} 项</span>
      <q-btn
        flat
        dense
        no-caps
        icon="toggle_on"
        label="批量启用"
        size="sm"
        class="app-registry-accent-btn"
        @click="batchToggle(true)"
      />
      <q-btn flat dense no-caps icon="toggle_off" label="批量停用" size="sm" @click="batchToggle(false)" />
      <q-btn flat dense no-caps icon="delete" label="批量删除" size="sm" color="negative" @click="batchRemove" />
    </div>

    <AppRegistryPagination
      v-model:page="page"
      v-model:page-size="pageSize"
      :page-max="pageMax"
      :total="total"
      :loading="loading"
      label="个 Tool"
    />

    <tool-detail-drawer
      :open="detailStore.open"
      :tool="detailStore.tool"
      :active-tab="detailStore.activeTab"
      :loading="detailStore.loading"
      :overrides="detailStore.overrides"
      :overrides-loading="detailStore.overridesLoading"
      :recent-runs="detailStore.recentRuns"
      :runs-loading="detailStore.runsLoading"
      :test-args-json="detailStore.testArgsJson"
      :test-timeout-sec="detailStore.testTimeoutSec"
      :test-running="detailStore.testRunning"
      :test-result="detailStore.testResult"
      :config-json="detailStore.configJson"
      :config-saving="detailStore.configSaving"
      :override-editor-open="detailStore.overrideEditorOpen"
      :editing-override="detailStore.editingOverride"
      :override-saving="detailStore.overrideSaving"
      :override-form="detailStore.overrideForm"
      :agent-binding-summary="detailStore.agentBindingSummary"
      :agent-binding-loading="detailStore.agentBindingLoading"
      :agent-options="detailStore.agentOptions"
      :agents-loading="detailStore.agentsLoading"
      @close="detailStore.closeDetail()"
      @update:active-tab="detailStore.activeTab = $event"
      @update:test-args-json="detailStore.testArgsJson = $event"
      @update:test-timeout-sec="detailStore.testTimeoutSec = $event"
      @run-test="detailStore.runToolTest()"
      @update:config-json="detailStore.configJson = $event"
      @save-config="detailStore.saveConfig()"
      @update:config-schema-json="detailStore.saveConfigSchema($event)"
      @edit-override="detailStore.openOverrideEditor($event)"
      @delete-override="detailStore.confirmRemoveOverride($event)"
      @update:override-editor-open="detailStore.overrideEditorOpen = $event"
      @update:override-form="detailStore.overrideForm = $event"
      @save-override="detailStore.saveOverride()"
      @edit-tool="onEditTool"
      @remove-tool="removeTool"
    />

    <tool-editor-dialog
      :open="editorStore.open"
      :editing-id="editorStore.editingId"
      :form="editorStore.form"
      :saving="editorStore.saving"
      :dirty="editorStore.dirty"
      :json-errors="editorStore.jsonErrors"
      :selected-template="editorStore.selectedTemplate"
      @close="editorStore.closeEditor()"
      @save="editorStore.save()"
      @apply-template="editorStore.applyTemplate($event)"
      @patch-form="onPatchForm"
    />
  </q-page>
</template>

<script setup lang="ts">
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import ToolHeroSection from '../components/tools/ToolHeroSection.vue';
import ToolsMetricStrip from '../components/tools/ToolsMetricStrip.vue';
import ToolCatalogFilters from '../components/tools/ToolCatalogFilters.vue';
import ToolsTable from '../components/tools/ToolsTable.vue';
import ToolDetailDrawer from '../components/tools/ToolDetailDrawer.vue';
import ToolEditorDialog from '../components/tools/ToolEditorDialog.vue';
import { useToolsPage } from '../features/tools/useToolsPage';

const {
  detailStore,
  editorStore,
  rows,
  total,
  loading,
  search,
  category,
  riskLevel,
  enabled,
  page,
  pageSize,
  error,
  selected,
  pageMax,
  summaryCards,
  categoryOptions,
  riskOptions,
  enabledOptions,
  busyId,
  loadRows,
  toggleEnabled,
  removeTool,
  updateRisk,
  resetFilters,
  openDetail,
  batchToggle,
  batchRemove,
  onEditTool,
  onPatchForm,
} = useToolsPage();
</script>
