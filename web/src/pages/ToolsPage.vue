<template>
  <q-page class="app-standard-page cap-page tools-page">
    <!-- 操作放在页签行右边（工具 | 工具运行 | 调用审计 那一行），不再单独占一行页头 -->
    <Teleport defer to="#app-tab-actions">
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :title="t('cap.common.refresh')"
        :aria-label="t('cap.common.refresh')"
        :loading="loading"
        @click="loadRows"
      />
      <q-btn
        unelevated
        dense
        no-caps
        color="primary"
        icon="add"
        :label="t('cap.tools.create')"
        @click="editorStore.openCreate()"
      />
    </Teleport>

    <CapStats :stats="stats" :label="t('cap.common.statsLabel')" :active="activeStat" @pick="onPickStat" />

    <div class="cap-bar" role="search">
      <q-input
        v-model="search"
        class="cap-bar__search"
        dense
        outlined
        clearable
        debounce="250"
        :placeholder="t('cap.tools.searchPlaceholder')"
        :aria-label="t('cap.tools.searchPlaceholder')"
      >
        <template #prepend><q-icon name="search" size="18px" /></template>
      </q-input>
      <div class="cap-seg" role="group" :aria-label="t('cap.common.statusFilter')">
        <button
          v-for="opt in statusSegments"
          :key="String(opt.value)"
          type="button"
          :aria-pressed="enabled === opt.value"
          @click="enabled = opt.value"
        >
          {{ opt.label }}
        </button>
      </div>
      <q-select
        v-model="category"
        dense
        outlined
        clearable
        emit-value
        map-options
        options-dense
        :label="t('cap.tools.category')"
        :options="categoryOptions"
      />
      <q-select
        v-model="source"
        dense
        outlined
        clearable
        emit-value
        map-options
        options-dense
        :label="t('cap.tools.source')"
        :options="sourceOptions"
      />
      <q-select
        v-model="riskLevel"
        dense
        outlined
        clearable
        emit-value
        map-options
        options-dense
        :label="t('cap.tools.risk')"
        :options="riskOptions"
      />
      <q-toggle v-model="abnormal" dense :label="t('cap.tools.abnormalOnly')" />
      <span class="cap-bar__spacer" />
      <q-btn
        v-if="filtersActive"
        flat
        dense
        no-caps
        icon="filter_alt_off"
        :label="t('cap.common.reset')"
        @click="resetFilters"
      />
    </div>

    <AppStatePanel v-if="pageState === 'loading'" variant="loading" :rows="5" />
    <AppStatePanel
      v-else-if="pageState === 'error'"
      variant="error"
      :title="error"
      :action-label="$t('common.retry')"
      action-icon="refresh"
      @action="loadRows"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" variant="forbidden" />
    <AppStatePanel
      v-else-if="pageState === 'empty'"
      variant="empty"
      icon="build"
      :title="t('cap.tools.emptyTitle')"
      :description="t('cap.tools.emptyHint')"
      :action-label="filtersActive ? t('cap.common.reset') : t('cap.tools.create')"
      :action-icon="filtersActive ? 'filter_alt_off' : 'add'"
      @action="filtersActive ? resetFilters() : editorStore.openCreate()"
    />

    <template v-else>
      <AppStatePanel
        v-if="pageState === 'partial'"
        variant="partial"
        dense
        :description="error"
        :action-label="$t('common.retry')"
        action-icon="refresh"
        @action="loadRows"
      />
      <tools-table
        :rows="rows"
        :loading="loading"
        :busy-id="busyId"
        :selected="selected"
        :active-id="detailStore.open ? detailStore.tool?.id : ''"
        :total="total"
        @update:selected="selected = $event"
        @toggle-enabled="toggleEnabled"
        @update-risk="updateRisk"
        @view-detail="openDetail"
        @view-agents="openAgents"
        @edit="editorStore.openEdit($event)"
        @remove="removeTool"
      />

      <AppRegistryPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="total"
        :loading="loading"
        :label="$t('toolsPage.list.pageUnit')"
      />

      <div
        v-if="selected.length"
        class="cap-dock"
        role="toolbar"
        :aria-label="t('cap.common.selected', { count: selected.length })"
      >
        <span class="cap-dock__count">{{ t('cap.common.selected', { count: selected.length }) }}</span>
        <q-btn
          flat
          dense
          no-caps
          icon="toggle_on"
          color="primary"
          :label="t('cap.common.batchEnable')"
          @click="batchToggle(true)"
        />
        <q-btn flat dense no-caps icon="toggle_off" :label="t('cap.common.batchDisable')" @click="batchToggle(false)" />
        <q-btn
          flat
          dense
          no-caps
          icon="delete"
          color="negative"
          :label="t('cap.common.batchRemove')"
          @click="batchRemove"
        />
        <q-btn flat dense round icon="close" :aria-label="t('cap.common.clearSelection')" @click="selected = []" />
      </div>
    </template>

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
      :policy="detailStore.policy"
      :policy-loading="detailStore.policyLoading"
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
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import CapStats from '../components/capability/CapStats.vue';
import { formatCount, formatRate, type CapTone } from '../components/capability/capTypes';
import ToolsTable from '../components/tools/ToolsTable.vue';
import ToolDetailDrawer from '../components/tools/ToolDetailDrawer.vue';
import ToolEditorDialog from '../components/tools/ToolEditorDialog.vue';
import { useToolsPage } from '../features/tools/useToolsPage';
import type { Tool } from '../features/tools/types';
import { resolvePageState } from './pageState';

const { t } = useI18n();

const {
  detailStore,
  editorStore,
  rows,
  total,
  summary,
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
  forbidden,
  selected,
  pageMax,
  categoryOptions,
  sourceOptions,
  riskOptions,
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

const statusSegments = computed(() => [
  { value: null, label: t('cap.common.all') },
  { value: true, label: t('cap.common.enabled') },
  { value: false, label: t('cap.common.disabled') },
]);

const filtersActive = computed(
  () =>
    Boolean(search.value || category.value || source.value || riskLevel.value || abnormal.value) ||
    enabled.value !== null,
);

// 关键数随当前筛选口径变化（后端 summary 与列表同一 where）
const stats = computed(() => {
  const s = summary.value;
  const failure: CapTone =
    s.calls_24h === 0 ? 'pending' : s.failure_rate_24h > 0.1 ? 'alert' : s.failure_rate_24h > 0.02 ? 'watch' : 'ok';
  return [
    // 后端汇总与列表同一筛选口径：有筛选时这个数是「符合筛选」，不是全部登记
    {
      key: 'total',
      label: t(filtersActive.value ? 'cap.tools.stats.matched' : 'cap.tools.stats.total'),
      value: formatCount(s.total_tools),
    },
    { key: 'enabled', label: t('cap.tools.stats.enabled'), value: formatCount(s.enabled_tools), clickable: true },
    {
      key: 'highRisk',
      label: t('cap.tools.stats.highRisk'),
      value: formatCount(s.high_risk_enabled),
      tone: (s.high_risk_enabled > 0 ? 'watch' : undefined) as CapTone | undefined,
      hint: t('cap.tools.stats.highRiskHint'),
    },
    // 平台还没怎么用时，「24h 调用 0」说明不了什么；「近 7 天用过几个工具」更能看出真实使用面
    {
      key: 'used7d',
      label: t('cap.tools.stats.used7d'),
      value: `${formatCount(s.used_tools_7d ?? 0)} / ${formatCount(s.enabled_tools)}`,
      hint: t('cap.tools.stats.used7dHint', { calls: s.calls_24h }),
    },
    {
      key: 'failure',
      label: t('cap.tools.stats.failure'),
      value: s.calls_24h === 0 ? '—' : formatRate(s.failure_rate_24h),
      tone: failure,
    },
  ];
});

const activeStat = computed(() => (enabled.value === true ? 'enabled' : ''));

// 「看哪些 Agent 能用」：打开检视抽屉并直接切到 Agent 页签
function openAgents(tool: Tool) {
  openDetail(tool);
  detailStore.activeTab = 'agents';
}

function onPickStat(key: string) {
  if (key === 'enabled') enabled.value = enabled.value === true ? null : true;
}

const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: rows.value.length,
    forbidden: forbidden.value,
  }),
);
</script>
