<template>
  <q-page class="app-page-cream evaluation-page q-pa-sm q-pa-md-md">
    <section class="evaluation-hero">
      <div>
        <div class="evaluation-kicker">EvalSet / metrics</div>
        <h1 class="evaluation-title">评估管理</h1>
        <p class="evaluation-subtitle">EvalSet + FrameworkBridge（LLM UserSim / 扩展指标 / 趋势对比已接入）。</p>
      </div>
      <div class="row q-gutter-sm">
        <q-btn color="primary" rounded unelevated icon="add" label="新建数据集" @click="createOpen = true" />
        <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadDatasets" />
      </div>
    </section>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadDatasets" />
      </template>
    </q-banner>

    <div class="row q-col-gutter-md">
      <div class="col-12 col-lg-4">
        <evaluation-dataset-list
          :datasets="datasets"
          :selected-id="selectedDatasetId"
          :loading="loading"
          @select="selectDataset"
        />
      </div>

      <div class="col-12 col-lg-8">
        <q-card v-if="selectedDataset" flat bordered>
          <q-card-section class="row items-center justify-between">
            <div>
              <div class="text-h6">{{ selectedDataset.name }}</div>
              <div class="text-caption text-grey-7">{{ selectedDataset.description || "无描述" }}</div>
            </div>
            <div class="row q-gutter-sm">
              <q-btn outline color="primary" icon="play_arrow" label="启动评估" @click="runOpen = true" />
              <q-btn flat color="negative" icon="delete" label="删除" @click="confirmDeleteDataset" />
            </div>
          </q-card-section>
          <q-separator />
          <q-card-section>
            <div class="text-subtitle2 q-mb-sm">运行记录</div>
            <q-table flat :rows="runs" :columns="runColumns" row-key="id" :loading="runsLoading" :pagination="{ rowsPerPage: 8 }">
              <template #body-cell-status="props">
                <q-td :props="props">
                  <q-chip dense :color="runStatusColor(props.row.status)" text-color="white" size="sm">{{ props.row.status }}</q-chip>
                </q-td>
              </template>
              <template #body-cell-actions="props">
                <q-td :props="props">
                  <q-btn flat dense label="结果" @click="openResults(props.row)" />
                </q-td>
              </template>
            </q-table>
          </q-card-section>
        </q-card>
        <q-card v-else flat bordered class="q-pa-lg text-center text-grey-7">请选择或新建数据集</q-card>

        <evaluation-analytics-panel
          v-if="selectedDataset"
          class="q-mt-md"
          v-model:agent-id="trendAgentId"
          :agent-options="agentOptions"
          :runs="runs"
          :trend-points="trendPoints"
          :comparisons="comparisons"
          :trend-loading="trendLoading"
          :compare-loading="compareLoading"
          @refresh-trend="loadTrend"
          @compare="submitCompare"
        />
      </div>
    </div>

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
      :loading="runLoading"
      :agent-options="agentOptions"
      @submit="submitRun"
    />
    <evaluation-results-dialog
      v-model:open="resultsOpen"
      :run-id="resultsRun?.id ?? ''"
      :run="resultsRun"
      :rows="caseResults"
      :loading="resultsLoading"
      :saving-id="savingResultId"
      :columns="resultColumns"
      @annotate="saveAnnotation"
      @update-row="updateResultRow"
    />
  </q-page>
</template>

<script setup lang="ts">
import EvaluationAnalyticsPanel from "../components/evaluation/EvaluationAnalyticsPanel.vue";
import EvaluationDatasetList from "../components/evaluation/EvaluationDatasetList.vue";
import EvaluationCreateDialog from "../components/evaluation/EvaluationCreateDialog.vue";
import EvaluationRunDialog from "../components/evaluation/EvaluationRunDialog.vue";
import EvaluationResultsDialog from "../components/evaluation/EvaluationResultsDialog.vue";
import { useEvaluationPage } from "../features/evaluation/useEvaluationPage";

const {
  datasets,
  runs,
  loading,
  selectedDatasetId,
  selectedDataset,
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
  openResults,
  savingResultId,
  updateResultRow,
  saveAnnotation,
  trendAgentId,
  trendPoints,
  trendLoading,
  comparisons,
  compareLoading,
  loadTrend,
  submitCompare
} = useEvaluationPage();
</script>

<style scoped>
.evaluation-hero {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  margin-bottom: 1.25rem;
}
.evaluation-kicker {
  font-size: 0.75rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--q-primary);
  font-weight: 600;
}
.evaluation-title {
  margin: 0.25rem 0;
  font-size: 1.75rem;
  font-weight: 700;
}
.evaluation-subtitle {
  margin: 0;
  color: #666;
  max-width: 36rem;
}
</style>
