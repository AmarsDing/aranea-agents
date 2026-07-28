import { computed, onMounted, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import type { EvalCaseResult, EvalRun, EvalRunComparison, EvalTrendPoint } from './types';
import { useEvaluationStore } from '../../stores/evaluation';
import { exportEvalRunCsv, exportEvalRunJson } from './exportRunResults';
import { EVAL_RESULTS_PAGE_SIZE_DEFAULT, EVAL_RUNS_PAGE_SIZE_DEFAULT } from '../constants/queryLimits';
import { EVAL_RESULT_TABLE_COLUMNS, EVAL_RUN_TABLE_COLUMNS } from './evaluationTableUi';

export function useEvaluationPage() {
  const $q = useQuasar();
  const { t } = useI18n();
  const evaluationStore = useEvaluationStore();
  const { datasets, runs, runsTotal, loading, agentOptions } = storeToRefs(evaluationStore);

  const selectedDatasetId = ref('');
  const runsPage = ref(1);
  const runsPageSize = ref(EVAL_RUNS_PAGE_SIZE_DEFAULT);
  const runsPageMax = computed(() => Math.max(1, Math.ceil(Math.max(0, runsTotal.value) / runsPageSize.value)));
  const runsLoading = ref(false);
  const error = ref('');
  const createOpen = ref(false);
  const createLoading = ref(false);
  const runOpen = ref(false);
  const runLoading = ref(false);
  const resultsOpen = ref(false);
  const resultsLoading = ref(false);
  const resultsRun = ref<EvalRun | null>(null);
  const caseResults = ref<EvalCaseResult[]>([]);
  const caseResultsTotal = ref(0);
  const resultsPage = ref(1);
  const resultsPageSize = ref(EVAL_RESULTS_PAGE_SIZE_DEFAULT);
  const resultsPageMax = computed(() =>
    Math.max(1, Math.ceil(Math.max(0, caseResultsTotal.value) / resultsPageSize.value)),
  );
  const savingResultId = ref('');

  const trendAgentId = ref('');
  const trendPoints = ref<EvalTrendPoint[]>([]);
  const trendLoading = ref(false);
  const comparisons = ref<EvalRunComparison[]>([]);
  const compareLoading = ref(false);

  const createForm = ref({ name: '', description: '' });
  const runForm = ref({ agent_id: '', metrics: '', num_runs: 1 });

  const selectedDataset = computed(() => datasets.value.find((d) => d.id === selectedDatasetId.value));

  const runColumns = EVAL_RUN_TABLE_COLUMNS;
  const resultColumns = EVAL_RESULT_TABLE_COLUMNS;

  function runStatusColor(status: string) {
    if (status === 'completed') return 'positive';
    if (status === 'failed') return 'negative';
    if (status === 'running') return 'warning';
    return 'grey';
  }

  async function loadAgentOptions() {
    await evaluationStore.loadAgentOptions();
    if (agentOptions.value.length && !runForm.value.agent_id) {
      runForm.value.agent_id = agentOptions.value[0].value;
    }
    if (agentOptions.value.length && !trendAgentId.value) {
      trendAgentId.value = agentOptions.value[0].value;
      void loadTrend();
    }
  }

  async function loadDatasets() {
    error.value = '';
    try {
      const res = await evaluationStore.loadDatasets({ limit: 100 });
      if (!selectedDatasetId.value && res.items.length) {
        selectedDatasetId.value = res.items[0].id;
        await loadRuns();
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : '加载失败';
    }
  }

  async function loadRuns() {
    if (!selectedDatasetId.value) return;
    runsLoading.value = true;
    try {
      const limit = runsPageSize.value;
      let page = runsPage.value;
      // Pre-clamp using last known total when possible (avoids empty last page).
      const maxPage = Math.max(1, Math.ceil(Math.max(0, runsTotal.value) / limit) || 1);
      if (runsTotal.value > 0 && page > maxPage) {
        page = maxPage;
        runsPage.value = page;
      }
      const offset = (page - 1) * limit;
      await evaluationStore.loadRuns({ dataset_id: selectedDatasetId.value, limit, offset });
      if (runsPage.value > runsPageMax.value) {
        runsPage.value = runsPageMax.value;
        if (runsPage.value !== page) {
          await evaluationStore.loadRuns({
            dataset_id: selectedDatasetId.value,
            limit,
            offset: (runsPage.value - 1) * limit,
          });
        }
      }
      if (trendAgentId.value) {
        void loadTrend();
      }
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '加载运行记录失败' });
    } finally {
      runsLoading.value = false;
    }
  }

  function selectDataset(id: string) {
    selectedDatasetId.value = id;
    runsPage.value = 1;
    void loadRuns();
  }

  function onRunsPage(page: number) {
    if (page === runsPage.value) return;
    runsPage.value = page;
    void loadRuns();
  }

  function onRunsPageSize(pageSize: number) {
    runsPageSize.value = pageSize;
    runsPage.value = 1;
    void loadRuns();
  }

  async function submitCreate() {
    if (!createForm.value.name.trim()) {
      $q.notify({ type: 'warning', message: '请填写名称' });
      return;
    }
    createLoading.value = true;
    try {
      const ds = await evaluationStore.addDataset({
        name: createForm.value.name.trim(),
        description: createForm.value.description.trim(),
      });
      createOpen.value = false;
      createForm.value = { name: '', description: '' };
      await loadDatasets();
      selectedDatasetId.value = ds.id;
      $q.notify({ type: 'positive', message: '数据集已创建' });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '创建失败' });
    } finally {
      createLoading.value = false;
    }
  }

  function confirmDeleteDataset() {
    const ds = selectedDataset.value;
    if (!ds) return;
    $q.dialog({ title: '删除数据集', message: `确定删除「${ds.name}」？`, cancel: true }).onOk(async () => {
      try {
        await evaluationStore.removeDataset(ds.id);
        if (selectedDatasetId.value === ds.id) {
          selectedDatasetId.value = '';
          runs.value = [];
        }
        await loadDatasets();
        $q.notify({ type: 'positive', message: '已删除' });
      } catch (e) {
        $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '删除失败' });
      }
    });
  }

  async function submitRun() {
    if (!selectedDatasetId.value || !runForm.value.agent_id) {
      $q.notify({ type: 'warning', message: '请选择 Agent' });
      return;
    }
    runLoading.value = true;
    try {
      await evaluationStore.startRun({
        dataset_id: selectedDatasetId.value,
        agent_id: runForm.value.agent_id,
        metrics: runForm.value.metrics.trim() || undefined,
        num_runs: runForm.value.num_runs > 1 ? runForm.value.num_runs : undefined,
      });
      runOpen.value = false;
      await loadRuns();
      $q.notify({ type: 'positive', message: '评估已提交' });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '启动失败' });
    } finally {
      runLoading.value = false;
    }
  }

  function updateResultRow(row: EvalCaseResult) {
    caseResults.value = caseResults.value.map((r) => (r.id === row.id ? row : r));
  }

  async function saveAnnotation(row: EvalCaseResult) {
    if (!resultsRun.value) return;
    savingResultId.value = row.id;
    try {
      const updated = await evaluationStore.annotateResult({
        run_id: resultsRun.value.id,
        result_id: row.id,
        human_pass: row.human_pass,
        human_score: row.human_score,
        human_comment: row.human_comment,
      });
      updateResultRow(updated);
      $q.notify({ type: 'positive', message: '标注已保存' });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '保存失败' });
    } finally {
      savingResultId.value = '';
    }
  }

  async function loadTrend() {
    if (!trendAgentId.value) return;
    trendLoading.value = true;
    try {
      trendPoints.value = await evaluationStore.loadAgentTrend({
        agent_id: trendAgentId.value,
        dataset_id: selectedDatasetId.value || undefined,
        limit: 20,
      });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '加载趋势失败' });
    } finally {
      trendLoading.value = false;
    }
  }

  async function submitCompare(runIds: string[]) {
    compareLoading.value = true;
    try {
      comparisons.value = await evaluationStore.compareRuns(runIds);
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '对比失败' });
    } finally {
      compareLoading.value = false;
    }
  }

  async function loadCaseResults() {
    if (!resultsRun.value) return;
    resultsLoading.value = true;
    try {
      const limit = resultsPageSize.value;
      let page = resultsPage.value;
      const knownMax = Math.max(1, Math.ceil(Math.max(0, caseResultsTotal.value) / limit) || 1);
      if (caseResultsTotal.value > 0 && page > knownMax) {
        page = knownMax;
        resultsPage.value = page;
      }
      let res = await evaluationStore.loadRunResults(resultsRun.value.id, {
        limit,
        offset: (page - 1) * limit,
      });
      caseResults.value = res.items;
      caseResultsTotal.value = res.total;
      if (resultsPage.value > resultsPageMax.value) {
        resultsPage.value = resultsPageMax.value;
        res = await evaluationStore.loadRunResults(resultsRun.value.id, {
          limit,
          offset: (resultsPage.value - 1) * limit,
        });
        caseResults.value = res.items;
        caseResultsTotal.value = res.total;
      }
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '加载结果失败' });
    } finally {
      resultsLoading.value = false;
    }
  }

  async function openResults(run: EvalRun) {
    resultsRun.value = run;
    resultsOpen.value = true;
    resultsPage.value = 1;
    await loadCaseResults();
  }

  function onResultsPage(page: number) {
    if (page === resultsPage.value) return;
    resultsPage.value = page;
    void loadCaseResults();
  }

  function onResultsPageSize(pageSize: number) {
    resultsPageSize.value = pageSize;
    resultsPage.value = 1;
    void loadCaseResults();
  }

  // Export is owned here (Store/Composable layer) — the results dialog only
  // emits export-csv / export-json and never touches the store itself.
  const exportingResults = ref(false);

  async function exportResults(format: 'csv' | 'json') {
    const run = resultsRun.value;
    if (!run) return;
    exportingResults.value = true;
    try {
      const res = await evaluationStore.loadRunResults(run.id, { limit: 5000, offset: 0 });
      if (format === 'csv') {
        exportEvalRunCsv(run, res.items);
      } else {
        exportEvalRunJson(run, res.items);
      }
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('evaluationPage.exportFailed') });
    } finally {
      exportingResults.value = false;
    }
  }

  onMounted(() => {
    void loadAgentOptions();
    void loadDatasets();
  });

  return {
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
    resultsPageMax,
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
  };
}
