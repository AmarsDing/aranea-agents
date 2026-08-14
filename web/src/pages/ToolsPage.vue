<template>
  <q-page class="app-standard-page app-registry-page tools-page">
    <tool-hero-section
      :kicker="$t('toolsPage.hero.kicker')"
      :title="$t('toolsPage.hero.title')"
      :subtitle="$t('toolsPage.hero.subtitle')"
    >
      <template #actions>
        <q-btn
          outline
          rounded
          no-caps
          class="app-outline-btn"
          icon="policy"
          :label="$t('toolsPage.hero.audits')"
          :to="{ name: 'tool-audits' }"
        />
        <q-btn
          outline
          rounded
          no-caps
          class="app-outline-btn"
          icon="history"
          :label="$t('toolsPage.hero.runs')"
          :to="{ name: 'tool-runs' }"
        />
        <q-btn
          rounded
          no-caps
          unelevated
          class="app-accent-btn"
          icon="add"
          :label="$t('toolsPage.hero.create')"
          @click="editorStore.openCreate()"
        />
      </template>
    </tool-hero-section>

    <tools-metric-strip :cards="summaryCards" />

    <tool-catalog-filters
      :search="search"
      :category="category"
      :source="source"
      :risk-level="riskLevel"
      :enabled="enabled"
      :abnormal="abnormal"
      :category-options="categoryOptions"
      :source-options="sourceOptions"
      :risk-options="riskOptions"
      :enabled-options="enabledOptions"
      :loading="loading"
      @update:search="search = $event"
      @update:category="category = $event"
      @update:source="source = $event"
      @update:risk-level="riskLevel = $event"
      @update:enabled="enabled = $event"
      @update:abnormal="abnormal = $event"
      @reset="resetFilters"
      @refresh="loadRows"
    />

    <q-banner v-if="error" rounded class="app-page-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense :label="$t('common.retry')" class="text-white" @click="loadRows" />
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
      <span class="text-caption q-mr-md">{{ $t('toolsPage.list.selectedCount', { count: selected.length }) }}</span>
      <q-btn
        flat
        dense
        no-caps
        icon="toggle_on"
        :label="$t('toolsPage.list.batchEnable')"
        size="sm"
        class="app-registry-accent-btn"
        @click="batchToggle(true)"
      />
      <q-btn flat dense no-caps icon="toggle_off" :label="$t('toolsPage.list.batchDisable')" size="sm" @click="batchToggle(false)" />
      <q-btn flat dense no-caps icon="delete" :label="$t('toolsPage.list.batchRemove')" size="sm" color="negative" @click="batchRemove" />
    </div>

    <AppRegistryPagination
      v-model:page="page"
      v-model:page-size="pageSize"
      :page-max="pageMax"
      :total="total"
      :loading="loading"
      :label="$t('toolsPage.list.pageUnit')"
    />

    <tool-detail-drawer
      :open="detailStore.open"
      :tool="detailStore.tool"
      :active-tab="detailStore.activeTab"
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
      :agent-binding-error="detailStore.agentBindingError"
      :agent-options="detailStore.agentOptions"
      :agents-loading="detailStore.agentsLoading"
      @close="detailStore.closeDetail()"
      @update:active-tab="detailStore.activeTab = $event"
      @update:test-args-json="detailStore.testArgsJson = $event"
      @update:test-timeout-sec="detailStore.testTimeoutSec = $event"
      @run-test="onRunTest"
      @update:config-json="detailStore.configJson = $event"
      @save-config="onSaveConfig"
      @update:config-schema-json="onSaveConfigSchema($event)"
      @edit-override="detailStore.openOverrideEditor($event)"
      @delete-override="onRemoveOverride($event)"
      @update:override-editor-open="detailStore.overrideEditorOpen = $event"
      @update:override-form="detailStore.overrideForm = $event"
      @save-override="onSaveOverride"
      @retry-agent-bindings="detailStore.loadAgentBindingSummary()"
      @edit-tool="onEditTool"
      @remove-tool="removeTool"
    />

    <tool-editor-dialog
      :open="editorStore.open"
      :editing-id="editorStore.editingId"
      :form="editorStore.form"
      :original-form="editorStore.originalForm"
      :saving="editorStore.saving"
      :dirty="editorStore.dirty"
      :json-errors="editorStore.jsonErrors"
      :selected-template="editorStore.selectedTemplate"
      :active-section="editorStore.activeTab"
      @close="editorStore.closeEditor()"
      @request-close="onEditorRequestClose"
      @save="saveEditor"
      @apply-template="editorStore.applyTemplate($event)"
      @patch-form="onPatchForm"
      @update:active-section="editorStore.activeTab = $event"
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
  source,
  riskLevel,
  enabled,
  abnormal,
  page,
  pageSize,
  error,
  selected,
  pageMax,
  summaryCards,
  categoryOptions,
  sourceOptions,
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
  onEditorRequestClose,
  saveEditor,
  onRunTest,
  onSaveConfig,
  onSaveConfigSchema,
  onSaveOverride,
  onRemoveOverride,
} = useToolsPage();
</script>
