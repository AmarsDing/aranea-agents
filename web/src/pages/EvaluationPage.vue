<template>
  <q-page class="app-standard-page app-registry-page evaluation-page">
    <AppPageHero kicker="EvalSet / metrics" title="评估管理" subtitle="EvalSet + FrameworkBridge（LLM UserSim / 扩展指标 / 趋势对比已接入）。">
      <template #actions>
        <q-btn color="primary" rounded unelevated no-caps icon="add" label="新建数据集" @click="createOpen = true" />
        <q-btn outline rounded no-caps color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadDatasets" />
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
              <div class="text-caption text-grey-7">{{ selectedDataset.description || "无描述" }}</div>
            </div>
            <div class="app-actions-bar app-actions-bar--start">
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
              :rows="pagedRuns"
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
                  <q-chip dense :color="runStatusColor(props.row.status)" text-color="white" size="sm">{{ props.row.status }}</q-chip>
                </q-td>
              </template>
              <template #body-cell-actions="props">
                <q-td :props="props">
                  <div class="app-registry-cell-actions">
                    <q-btn flat dense round icon="analytics" color="primary" aria-label="查看结果" @click="openResults(props.row)" />
                  </div>
                </q-td>
              </template>
            </AppRegistryTable>
            <AppRegistryPagination
              v-model:page="runsPage"
              v-model:page-size="runsPageSize"
              :page-max="runsPageMax"
              :total="runs.length"
              :loading="runsLoading"
              label="条运行"
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
import { computed, ref, watch } from "vue";
import AppPageHero from "../components/layout/AppPageHero.vue";
import AppRegistryTable from "../components/layout/AppRegistryTable.vue";
import AppRegistryPagination from "../components/layout/AppRegistryPagination.vue";
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

const runsPage = ref(1);
const runsPageSize = ref(8);
const runsPageMax = computed(() => Math.max(1, Math.ceil(runs.value.length / runsPageSize.value)));
const pagedRuns = computed(() => {
  const start = (runsPage.value - 1) * runsPageSize.value;
  return runs.value.slice(start, start + runsPageSize.value);
});

watch(
  () => runs.value.length,
  () => {
    if (runsPage.value > runsPageMax.value) runsPage.value = runsPageMax.value;
  }
);
</script>
