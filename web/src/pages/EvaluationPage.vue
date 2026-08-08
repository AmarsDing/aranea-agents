<template>
  <q-page class="app-standard-page app-registry-page evaluation-page">
    <AppPageHero
      kicker="EvalSet / metrics"
      title="评估管理"
      subtitle="EvalSet + FrameworkBridge（LLM UserSim / 扩展指标 / 趋势对比已接入）。"
    >
      <template #actions>
        <q-btn color="primary" rounded unelevated no-caps icon="add" label="新建数据集" @click="createOpen = true" />
        <q-btn
          outline
          rounded
          no-caps
          color="primary"
          icon="gavel"
          :label="$t('evaluationPage.gateOpen')"
          @click="openGate"
        />
        <q-btn
          outline
          rounded
          no-caps
          color="primary"
          icon="refresh"
          label="刷新"
          :loading="loading"
          @click="loadDatasets"
        />
      </template>
    </AppPageHero>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadDatasets" />
      </template>
    </q-banner>

    <div class="evaluation-layout">
      <evaluation-dataset-list
        :datasets="datasets"
        :selected-id="selectedDatasetId"
        :loading="loading"
        @select="selectDataset"
      />

      <div>
        <q-card v-if="selectedDataset" flat class="app-entity-glass-panel evaluation-detail-card">
          <q-card-section class="row items-center justify-between">
            <div>
              <div class="text-h6">{{ selectedDataset.name }}</div>
              <div class="text-caption text-grey-7">{{ selectedDataset.description || '无描述' }}</div>
            </div>
            <div class="app-actions-bar app-actions-bar--start">
              <q-btn
                outline
                no-caps
                color="primary"
                icon="upload_file"
                :label="$t('evaluationPage.uploadCases')"
                @click="uploadOpen = true"
              />
              <q-btn outline no-caps color="primary" icon="play_arrow" label="启动评估" @click="runOpen = true" />
              <q-btn flat no-caps color="negative" icon="delete" label="删除" @click="confirmDeleteDataset" />
            </div>
          </q-card-section>
          <q-separator />
          <q-card-section>
            <div class="text-subtitle2 q-mb-sm">运行记录</div>
            <AppRegistryTable
              :shell="false"
              :data-shell="true"
              :rows="runs"
              :columns="runColumns"
              row-key="id"
              :loading="runsLoading"
              hide-pagination
              :pagination="{ rowsPerPage: 0 }"
            >
              <template #body-cell-id="props">
                <q-td :props="props">
                  <span class="app-registry-cell-sub ellipsis" :title="props.row.id">{{ props.row.id }}</span>
                </q-td>
              </template>
              <template #body-cell-status="props">
                <q-td :props="props">
                  <q-chip dense :color="runStatusColor(props.row.status)" text-color="white" size="sm">{{
                    props.row.status
                  }}</q-chip>
                </q-td>
              </template>
              <template #body-cell-actions="props">
                <q-td :props="props">
                  <div class="app-registry-cell-actions">
                    <q-btn
                      flat
                      dense
                      round
                      icon="analytics"
                      color="primary"
                      aria-label="查看结果"
                      @click="openResults(props.row)"
                    />
                  </div>
                </q-td>
              </template>
            </AppRegistryTable>
            <AppRegistryPagination
              :page="runsPage"
              :page-size="runsPageSize"
              :page-max="runsPageMax"
              :total="runsTotal"
              :loading="runsLoading"
              label="条运行"
              :page-size-options="[8, 10, 20, 50]"
              @update:page="onRunsPage"
              @update:page-size="onRunsPageSize"
            />
          </q-card-section>
        </q-card>
        <div v-else class="app-registry-empty app-entity-empty">
          <q-icon name="analytics" size="48px" color="grey-6" />
          <div class="text-h6">请选择或新建数据集</div>
          <div class="text-body2">左侧选择 EvalSet，或点击右上角「新建数据集」开始评估。</div>
        </div>

        <evaluation-analytics-panel
          v-if="selectedDataset"
          v-model:agent-id="trendAgentId"
          class="q-mt-md"
          :agent-options="agentOptions"
          :runs="runs"
          :trend-points="trendPoints"
          :comparisons="comparisons"
          :divergence="divergence"
          :trend-loading="trendLoading"
          :compare-loading="compareLoading"
          :divergence-loading="divergenceLoading"
          :failure-groups="failureGroups"
          :failure-groups-total="failureGroupsTotal"
          :failure-groups-loading="failureGroupsLoading"
          :preferences="preferences"
          :preferences-loading="preferencesLoading"
          :preference-saving="preferenceSaving"
          :dataset-changed="datasetChanged"
          @refresh-trend="loadTrend"
          @refresh-divergence="loadDivergence"
          @refresh-failures="loadFailureGroups"
          @compare="submitCompare"
          @prefer="submitPreferenceWinner"
        />
      </div>
    </div>

    <evaluation-feedback-panel class="q-mt-md" />

    <evaluation-create-dialog
      v-model:open="createOpen"
      v-model:name="createForm.name"
      v-model:description="createForm.description"
      :loading="createLoading"
      @submit="submitCreate"
    />
    <evaluation-run-dialog
      v-model:open="runOpen"
      v-model:agent-id="runForm.agent_id"
      v-model:metrics="runForm.metrics"
      v-model:num-runs="runForm.num_runs"
      v-model:user-simulation="runForm.use_user_simulation"
      :loading="runLoading"
      :agent-options="agentOptions"
      @submit="submitRun"
    />
    <evaluation-upload-cases-dialog
      v-model:open="uploadOpen"
      v-model:text="uploadText"
      :loading="uploadLoading"
      :dataset-name="selectedDataset?.name ?? ''"
      @submit="submitUpload"
    />
    <evaluation-gate-dialog
      v-model:open="gateOpen"
      v-model:enabled="gateForm.enabled"
      v-model:agent-id="gateForm.agent_id"
      v-model:dataset-id="gateForm.dataset_id"
      v-model:metric="gateForm.metric"
      v-model:min-score="gateForm.min_score"
      v-model:max-drop="gateForm.max_drop"
      :loading="gateLoading"
      :saving="gateSaving"
      :agent-options="agentOptions"
      :dataset-options="datasetOptions"
      @submit="saveGate"
    />
    <evaluation-results-dialog
      v-model:open="resultsOpen"
      :run-id="resultsRun?.id ?? ''"
      :run="resultsRun"
      :rows="caseResults"
      :total="caseResultsTotal"
      :page="resultsPage"
      :page-size="resultsPageSize"
      :loading="resultsLoading"
      :saving-id="savingResultId"
      :exporting="exportingResults"
      :columns="resultColumns"
      @annotate="saveAnnotation"
      @update-row="updateResultRow"
      @page-change="onResultsPage"
      @page-size-change="onResultsPageSize"
      @export-csv="exportResults('csv')"
      @export-json="exportResults('json')"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import EvaluationAnalyticsPanel from '../components/evaluation/EvaluationAnalyticsPanel.vue';
import EvaluationFeedbackPanel from '../components/evaluation/EvaluationFeedbackPanel.vue';
import EvaluationDatasetList from '../components/evaluation/EvaluationDatasetList.vue';
import EvaluationCreateDialog from '../components/evaluation/EvaluationCreateDialog.vue';
import EvaluationRunDialog from '../components/evaluation/EvaluationRunDialog.vue';
import EvaluationResultsDialog from '../components/evaluation/EvaluationResultsDialog.vue';
import EvaluationUploadCasesDialog from '../components/evaluation/EvaluationUploadCasesDialog.vue';
import EvaluationGateDialog from '../components/evaluation/EvaluationGateDialog.vue';
import { useEvaluationPage } from '../features/evaluation/useEvaluationPage';

const { t } = useI18n();

const {
  datasets,
  runs,
  runsTotal,
  loading,
  selectedDatasetId,
  selectedDataset,
  runsPage,
  runsPageSize,
  runsPageMax,
  onRunsPage,
  onRunsPageSize,
  runsLoading,
  error,
  createOpen,
  createLoading,
  runOpen,
  runLoading,
  resultsOpen,
  resultsLoading,
  resultsRun,
  caseResults,
  caseResultsTotal,
  resultsPage,
  resultsPageSize,
  onResultsPage,
  onResultsPageSize,
  agentOptions,
  createForm,
  runForm,
  runColumns,
  resultColumns,
  runStatusColor,
  loadDatasets,
  selectDataset,
  submitCreate,
  confirmDeleteDataset,
  submitRun,
  uploadOpen,
  uploadLoading,
  uploadText,
  submitUpload,
  openResults,
  savingResultId,
  updateResultRow,
  saveAnnotation,
  exportingResults,
  exportResults,
  trendAgentId,
  trendPoints,
  trendLoading,
  comparisons,
  compareLoading,
  loadTrend,
  submitCompare,
  divergence,
  divergenceLoading,
  loadDivergence,
  failureGroups,
  failureGroupsTotal,
  failureGroupsLoading,
  loadFailureGroups,
  preferences,
  preferencesLoading,
  preferenceSaving,
  submitPreferenceWinner,
  datasetChanged,
  gateOpen,
  gateLoading,
  gateSaving,
  gateForm,
  openGate,
  saveGate,
} = useEvaluationPage();

// Gate dialog needs all datasets as options (gate config is a global singleton,
// not bound to the currently selected dataset).
const datasetOptions = computed(() =>
  datasets.value.map((d) => ({
    label: t('evaluationPage.datasetOptionLabel', { name: d.name, count: d.case_count }),
    value: d.id,
  })),
);
</script>
