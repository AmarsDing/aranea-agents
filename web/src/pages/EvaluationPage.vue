<template>
  <!--
    Harness › 运行与治理 › 质量评估（2026-11-19 重排）：回答「质量有没有退化」。
    数据集（用例）→ 选 Agent 跑评估 → 看得分、失败归类与趋势 → 发布门禁把关上线。
  -->
  <q-page class="app-standard-page app-registry-page op-page evaluation-page">
    <AppPageHero :title="$t('ops.evaluation.title')" :subtitle="$t('ops.evaluation.hint')">
      <template #actions>
        <q-btn
          color="primary"
          unelevated
          no-caps
          icon="add"
          :label="$t('evaluationPage.createDataset')"
          @click="createOpen = true"
        />
        <q-btn flat no-caps icon="gavel" :label="$t('evaluationPage.gateOpen')" @click="openGate" />
        <q-btn
          flat
          round
          dense
          icon="refresh"
          :aria-label="$t('ops.common.refresh')"
          :loading="loading"
          @click="loadDatasets"
        />
      </template>
    </AppPageHero>

    <AppStatePanel v-if="pageState === 'loading'" variant="loading" :rows="4" />
    <AppStatePanel
      v-else-if="pageState === 'error'"
      variant="error"
      :title="error"
      action-label="重试"
      action-icon="refresh"
      @action="loadDatasets"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" variant="forbidden" />
    <OpsGuide
      v-else-if="pageState === 'empty'"
      data-state="empty"
      icon="fact_check"
      :title="$t('ops.evaluation.guide.title')"
      :description="$t('ops.evaluation.guide.desc')"
      :steps="[
        $t('ops.evaluation.guide.step1'),
        $t('ops.evaluation.guide.step2'),
        $t('ops.evaluation.guide.step3'),
        $t('ops.evaluation.guide.step4'),
      ]"
      :action-label="$t('evaluationPage.createDataset')"
      @action="createOpen = true"
    />

    <AppStatePanel
      v-if="pageState === 'partial'"
      class="q-mb-md"
      variant="partial"
      dense
      :description="error"
      action-label="重试"
      action-icon="refresh"
      @action="loadDatasets"
    />

    <div v-if="pageState === null || pageState === 'partial'" class="evaluation-layout">
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
            <!-- 主操作只留「启动评估」与「用例」，其余收进「更多」 -->
            <div class="app-actions-bar app-actions-bar--start">
              <q-btn
                unelevated
                no-caps
                color="primary"
                icon="play_arrow"
                :label="$t('ops.evaluation.run')"
                @click="runOpen = true"
              />
              <q-btn
                outline
                no-caps
                color="primary"
                icon="list"
                :label="$t('evaluationPage.manageCases')"
                @click="openCases"
              />
              <q-btn flat round dense icon="more_vert" :aria-label="$t('ops.common.more')">
                <q-menu anchor="bottom right" self="top right">
                  <q-list dense>
                    <q-item v-close-popup clickable @click="uploadOpen = true">
                      <q-item-section avatar><q-icon name="upload_file" /></q-item-section>
                      <q-item-section>{{ $t('evaluationPage.uploadCases') }}</q-item-section>
                    </q-item>
                    <q-item v-close-popup clickable @click="openVersions">
                      <q-item-section avatar><q-icon name="history" /></q-item-section>
                      <q-item-section>{{ $t('evaluationPage.manageVersions') }}</q-item-section>
                    </q-item>
                    <q-item v-close-popup clickable @click="openEditDataset">
                      <q-item-section avatar><q-icon name="edit" /></q-item-section>
                      <q-item-section>{{ $t('evaluationPage.editDataset') }}</q-item-section>
                    </q-item>
                    <q-separator />
                    <q-item v-close-popup clickable class="text-negative" @click="confirmDeleteDataset">
                      <q-item-section avatar><q-icon name="delete" /></q-item-section>
                      <q-item-section>{{ $t('ops.evaluation.deleteDataset') }}</q-item-section>
                    </q-item>
                  </q-list>
                </q-menu>
              </q-btn>
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
                  <span class="op-tag" :class="`op-tag--${runTone(props.row.status)}`">{{
                    runLabel(props.row.status)
                  }}</span>
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
                    <q-btn
                      v-if="props.row.status === 'pending' || props.row.status === 'running'"
                      flat
                      dense
                      round
                      icon="stop"
                      color="warning"
                      :aria-label="$t('evaluationPage.runCancel')"
                      @click="cancelEvalRun(props.row)"
                    />
                    <q-btn
                      flat
                      dense
                      round
                      icon="delete"
                      color="negative"
                      :aria-label="$t('evaluationPage.runDelete')"
                      @click="confirmDeleteRun(props.row)"
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
          :divergence-error="divergenceError"
          :failure-groups="failureGroups"
          :failure-groups-total="failureGroupsTotal"
          :failure-groups-loading="failureGroupsLoading"
          :preferences="preferences"
          :preferences-loading="preferencesLoading"
          :preference-saving="preferenceSaving"
          :dataset-changed="datasetChanged"
          :experiment-pivots="experimentPivots"
          @refresh-trend="loadTrend"
          @refresh-divergence="loadDivergence"
          @refresh-failures="loadFailureGroups"
          @compare="submitCompare"
          @prefer="submitPreferenceWinner"
        />
      </div>
    </div>

    <evaluation-feedback-panel v-if="pageState === null || pageState === 'partial'" class="q-mt-md" />

    <evaluation-create-dialog
      v-model:open="createOpen"
      v-model:name="createForm.name"
      v-model:description="createForm.description"
      :loading="createLoading"
      :title="$t('evaluationPage.createDataset')"
      :submit-label="$t('evaluationPage.createDatasetSubmit')"
      @submit="submitCreate"
    />
    <evaluation-create-dialog
      v-model:open="editDatasetOpen"
      v-model:name="createForm.name"
      v-model:description="createForm.description"
      :loading="createLoading"
      :title="$t('evaluationPage.editDataset')"
      :submit-label="$t('evaluationPage.saveDataset')"
      @submit="submitEditDataset"
    />
    <evaluation-cases-dialog
      v-model:open="casesOpen"
      v-model:edit-input="caseEditInput"
      v-model:edit-expected="caseEditExpected"
      :rows="cases"
      :columns="caseColumns"
      :loading="casesLoading"
      :saving="casesSaving"
      :edit-id="caseEditId"
      @edit="startEditCase"
      @remove="confirmDeleteCase"
      @save="saveCase"
      @cancel-edit="cancelEditCase"
    />
    <evaluation-run-dialog
      v-model:open="runOpen"
      v-model:agent-id="runForm.agent_id"
      v-model:metrics="runForm.metrics"
      v-model:num-runs="runForm.num_runs"
      v-model:user-simulation="runForm.use_user_simulation"
      v-model:extra-agent-ids="runForm.extra_agent_ids"
      v-model:model="runForm.model"
      v-model:extra-models="runForm.extra_models"
      v-model:prompt="runForm.prompt"
      v-model:tools="runForm.tools"
      v-model:extra-tools="runForm.extra_tools"
      :version-label="runForm.dataset_version ? String(runForm.dataset_version) : ''"
      :loading="runLoading"
      :agent-options="agentOptions"
      @submit="submitRun"
    />
    <evaluation-versions-dialog
      v-model:open="versionsOpen"
      :rows="versions"
      :columns="versionColumns"
      :loading="versionsLoading"
      @run="runPinnedVersion"
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
      v-model:mode="gateForm.mode"
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
      @open-trace="openResultTrace"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import OpsGuide from '../components/ops/OpsGuide.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import EvaluationAnalyticsPanel from '../components/evaluation/EvaluationAnalyticsPanel.vue';
import EvaluationFeedbackPanel from '../components/evaluation/EvaluationFeedbackPanel.vue';
import EvaluationDatasetList from '../components/evaluation/EvaluationDatasetList.vue';
import EvaluationCreateDialog from '../components/evaluation/EvaluationCreateDialog.vue';
import EvaluationCasesDialog from '../components/evaluation/EvaluationCasesDialog.vue';
import EvaluationRunDialog from '../components/evaluation/EvaluationRunDialog.vue';
import EvaluationResultsDialog from '../components/evaluation/EvaluationResultsDialog.vue';
import EvaluationUploadCasesDialog from '../components/evaluation/EvaluationUploadCasesDialog.vue';
import EvaluationGateDialog from '../components/evaluation/EvaluationGateDialog.vue';
import EvaluationVersionsDialog from '../components/evaluation/EvaluationVersionsDialog.vue';
import { useEvaluationPage } from '../features/evaluation/useEvaluationPage';
import { resolvePageState } from './pageState';

const { t, te } = useI18n();

/** 运行状态：中文标签 + 语义色（原来是英文原值 + 实心彩块） */
function runTone(status: string) {
  if (status === 'completed') return 'ok';
  if (status === 'failed') return 'alert';
  if (status === 'running' || status === 'pending') return 'run';
  return 'off';
}
function runLabel(status: string) {
  const key = `ops.evaluation.status.${status}`;
  return te(key) ? t(key) : status;
}

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
  forbidden,
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
  loadDatasets,
  selectDataset,
  submitCreate,
  confirmDeleteDataset,
  editDatasetOpen,
  openEditDataset,
  submitEditDataset,
  casesOpen,
  casesLoading,
  casesSaving,
  cases,
  caseColumns,
  caseEditId,
  caseEditInput,
  caseEditExpected,
  openCases,
  startEditCase,
  cancelEditCase,
  saveCase,
  confirmDeleteCase,
  cancelEvalRun,
  confirmDeleteRun,
  submitRun,
  experimentPivots,
  versionsOpen,
  versionsLoading,
  versions,
  versionColumns,
  openVersions,
  runPinnedVersion,
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
  divergenceError,
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
  openResultTrace,
} = useEvaluationPage();

// 五态契约：错误优先于空态（error 与 empty 互斥，见 pages/pageState.ts）。
const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: datasets.value.length,
    forbidden: forbidden.value,
  }),
);

// Gate dialog needs all datasets as options (gate config is a global singleton,
// not bound to the currently selected dataset).
const datasetOptions = computed(() =>
  datasets.value.map((d) => ({
    label: t('evaluationPage.datasetOptionLabel', { name: d.name, count: d.case_count }),
    value: d.id,
  })),
);
</script>
