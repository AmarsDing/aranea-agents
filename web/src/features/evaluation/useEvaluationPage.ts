import { computed, onMounted, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import type {
  AnnotateCaseResultInput,
  EvalCase,
  EvalCaseResult,
  EvalFailureGroup,
  EvalRun,
  EvalRunComparison,
  EvalDatasetVersion,
  EvalRunPreference,
  EvalTrendPoint,
  ExperimentVariant,
  JudgeDivergence,
} from './types';
import { useEvaluationStore } from '../../stores/evaluation';
import { exportEvalRunCsv, exportEvalRunJson } from './exportRunResults';
import { useEvalRunPolling, hasActiveRuns } from './useEvalRunPolling';
import { EVAL_RESULTS_PAGE_SIZE_DEFAULT, EVAL_RUNS_PAGE_SIZE_DEFAULT } from '../constants/queryLimits';
import {
  EVAL_CASE_TABLE_COLUMNS,
  EVAL_RESULT_TABLE_COLUMNS,
  EVAL_RUN_TABLE_COLUMNS,
  EVAL_VERSION_TABLE_COLUMNS,
} from './evaluationTableUi';
import { useEvaluationGate } from './useEvaluationGate';
import { useMonitorRunNavigation } from '../monitor/useMonitorRunNavigation';

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

  // Monotonic guards for async state writes: prevent stale API responses
  // from clobbering newer state when the user switches datasets/agents/pages
  // quickly or when overlapping refreshes are in flight.
  let divergenceSeq = 0;
  let failureGroupsSeq = 0;
  let preferencesSeq = 0;
  let trendSeq = 0;
  let caseResultsSeq = 0;

  const trendAgentId = ref('');
  const trendPoints = ref<EvalTrendPoint[]>([]);
  const trendLoading = ref(false);
  const comparisons = ref<EvalRunComparison[]>([]);
  const compareLoading = ref(false);
  // P1-3: judge calibration summary for the selected dataset (+ trend agent).
  const divergence = ref<JudgeDivergence | null>(null);
  const divergenceLoading = ref(false);
  // P2-3: failure groups (error_message aggregation) for the selected dataset.
  const failureGroups = ref<EvalFailureGroup[]>([]);
  const failureGroupsTotal = ref(0);
  const failureGroupsLoading = ref(false);
  // P3-3: pairwise human preferences for the selected dataset.
  const preferences = ref<EvalRunPreference[]>([]);
  const preferencesLoading = ref(false);
  const preferenceSaving = ref(false);
  // P3-5: compared runs recorded different dataset hashes → scores not comparable.
  const datasetChanged = computed(() => {
    const hashes = comparisons.value.map((c) => c.dataset_hash).filter((h) => h);
    const versions = comparisons.value.map((c) => c.dataset_version).filter((v) => v);
    return (
      (hashes.length > 1 && new Set(hashes).size > 1) ||
      (versions.length > 1 && new Set(versions).size > 1)
    );
  });
  const { gateOpen, gateLoading, gateSaving, gateForm, openGate, saveGate } = useEvaluationGate();
  const { openMonitorTab } = useMonitorRunNavigation();

  const editDatasetOpen = ref(false);
  const casesOpen = ref(false);
  const casesLoading = ref(false);
  const casesSaving = ref(false);
  const cases = ref<EvalCase[]>([]);
  const caseEditId = ref('');
  const caseEditInput = ref('');
  const caseEditExpected = ref('');
  const createForm = ref({ name: '', description: '' });
  const runForm = ref({
    agent_id: '',
    metrics: '',
    num_runs: 1,
    use_user_simulation: false,
    extra_agent_ids: [] as string[],
    model: '',
    extra_models: [] as string[],
    prompt: '',
    tools: '',
    extra_tools: [] as string[],
    dataset_version_id: '',
    dataset_version: 0,
  });
  const versionsOpen = ref(false);
  const versionsLoading = ref(false);
  const versions = ref<EvalDatasetVersion[]>([]);
  const versionColumns = EVAL_VERSION_TABLE_COLUMNS;
  const uploadOpen = ref(false);
  const uploadLoading = ref(false);
  const uploadText = ref('');

  const selectedDataset = computed(() => datasets.value.find((d) => d.id === selectedDatasetId.value));

  const runColumns = EVAL_RUN_TABLE_COLUMNS;
  const resultColumns = EVAL_RESULT_TABLE_COLUMNS;
  const caseColumns = EVAL_CASE_TABLE_COLUMNS;

  function runStatusColor(status: string) {
    if (status === 'completed') return 'positive';
    if (status === 'failed') return 'negative';
    if (status === 'running') return 'warning';
    if (status === 'cancelled') return 'grey';
    return 'grey';
  }

  async function loadAgentOptions() {
    await evaluationStore.loadAgentOptions();
    if (agentOptions.value.length && !runForm.value.agent_id) {
      runForm.value.agent_id = agentOptions.value[0].value;
    }
    if (agentOptions.value.length && !trendAgentId.value) {
      trendAgentId.value = agentOptions.value[0].value;
      // No loadTrend here: onMounted chains it after loadDatasets so the first
      // trend request is dataset-scoped (avoids a duplicate agent-wide fetch).
    }
  }

  async function loadDatasets() {
    error.value = '';
    try {
      const res = await evaluationStore.loadDatasets({ limit: 100 });
      if (!selectedDatasetId.value && res.items.length) {
        selectedDatasetId.value = res.items[0].id;
      }
      // ISSUE-003: refresh cascades to runs even when a dataset is already selected.
      if (selectedDatasetId.value) {
        await loadRuns();
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : '加载失败';
    }
  }

  async function loadRuns(silent = false) {
    if (!selectedDatasetId.value) return;
    if (!silent) runsLoading.value = true;
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
      if (trendAgentId.value && !silent) {
        void loadTrend();
      }
    } catch (e) {
      // Silent polls must not spam notifications on transient network errors.
      if (!silent) {
        $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '加载运行记录失败' });
      } else {
        console.warn('[evaluation] silent runs poll failed:', e);
      }
    } finally {
      if (!silent) runsLoading.value = false;
    }
  }

  // ISSUE-003: poll run progress while any run is pending/running; when the
  // last active run reaches a terminal status, refresh the trend panel once.
  useEvalRunPolling(runs, async () => {
    await loadRuns(true);
    if (!hasActiveRuns(runs.value)) {
      if (trendAgentId.value) {
        void loadTrend();
      }
      // Terminal runs may have produced new failures — regroup.
      void loadFailureGroups();
    }
  });

  function selectDataset(id: string) {
    selectedDatasetId.value = id;
    runsPage.value = 1;
    void loadRuns();
    // Divergence is chained off loadRuns → loadTrend (shared agent filter);
    // only fire directly when no trend agent exists (chain skips it then).
    if (!trendAgentId.value) {
      void loadDivergence();
    }
    void loadFailureGroups();
    void loadPreferences();
  }

  async function loadDivergence() {
    const seq = ++divergenceSeq;
    if (!selectedDatasetId.value) {
      divergence.value = null;
      return;
    }
    divergenceLoading.value = true;
    try {
      const res = await evaluationStore.loadJudgeDivergence(selectedDatasetId.value, {
        agent_id: trendAgentId.value || undefined,
      });
      if (seq !== divergenceSeq) return; // stale — a newer request owns the state
      divergence.value = res;
    } catch (e) {
      // Divergence is an auxiliary panel — never notify, just degrade to empty.
      console.warn('[evaluation] load judge divergence failed:', e);
      if (seq === divergenceSeq) divergence.value = null;
    } finally {
      if (seq === divergenceSeq) divergenceLoading.value = false;
    }
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
          runsTotal.value = 0; // keep pagination consistent with the cleared list
        }
        await loadDatasets();
        $q.notify({ type: 'positive', message: '已删除' });
      } catch (e) {
        $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '删除失败' });
      }
    });
  }

  function openEditDataset() {
    const ds = selectedDataset.value;
    if (!ds) return;
    createForm.value = { name: ds.name, description: ds.description };
    editDatasetOpen.value = true;
  }

  async function submitEditDataset() {
    const ds = selectedDataset.value;
    if (!ds || !createForm.value.name.trim()) return;
    createLoading.value = true;
    try {
      await evaluationStore.editDataset({
        id: ds.id,
        name: createForm.value.name.trim(),
        description: createForm.value.description.trim(),
      });
      editDatasetOpen.value = false;
      $q.notify({ type: 'positive', message: t('evaluationPage.datasetUpdated') });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('evaluationPage.datasetUpdateFailed') });
    } finally {
      createLoading.value = false;
    }
  }

  async function openCases() {
    if (!selectedDatasetId.value) return;
    casesOpen.value = true;
    caseEditId.value = '';
    casesLoading.value = true;
    try {
      cases.value = await evaluationStore.loadCases(selectedDatasetId.value);
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('evaluationPage.casesLoadFailed') });
    } finally {
      casesLoading.value = false;
    }
  }

  function startEditCase(row: EvalCase) {
    caseEditId.value = row.id;
    caseEditInput.value = row.input;
    caseEditExpected.value = row.expected_output;
  }

  function cancelEditCase() {
    caseEditId.value = '';
    caseEditInput.value = '';
    caseEditExpected.value = '';
  }

  async function saveCase() {
    if (!selectedDatasetId.value || !caseEditId.value || !caseEditInput.value.trim()) return;
    casesSaving.value = true;
    try {
      const updated = await evaluationStore.saveCase({
        dataset_id: selectedDatasetId.value,
        id: caseEditId.value,
        input: caseEditInput.value.trim(),
        expected_output: caseEditExpected.value,
      });
      cases.value = cases.value.map((c) => (c.id === updated.id ? updated : c));
      cancelEditCase();
      $q.notify({ type: 'positive', message: t('evaluationPage.caseSaved') });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('evaluationPage.caseSaveFailed') });
    } finally {
      casesSaving.value = false;
    }
  }

  function confirmDeleteCase(row: EvalCase) {
    $q.dialog({ title: t('evaluationPage.caseDelete'), message: t('evaluationPage.caseDeleteConfirm'), cancel: true }).onOk(
      async () => {
        try {
          await evaluationStore.removeCase(row.dataset_id, row.id);
          cases.value = cases.value.filter((c) => c.id !== row.id);
          if (caseEditId.value === row.id) cancelEditCase();
          $q.notify({ type: 'positive', message: t('evaluationPage.caseDeleted') });
        } catch (e) {
          $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('evaluationPage.caseDeleteFailed') });
        }
      },
    );
  }

  async function cancelEvalRun(row: EvalRun) {
    try {
      await evaluationStore.stopRun(row.id);
      $q.notify({ type: 'positive', message: t('evaluationPage.runCancelled') });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('evaluationPage.runCancelFailed') });
    }
  }

  function confirmDeleteRun(row: EvalRun) {
    $q.dialog({ title: t('evaluationPage.runDelete'), message: t('evaluationPage.runDeleteConfirm'), cancel: true }).onOk(
      async () => {
        try {
          await evaluationStore.removeRun(row.id);
          $q.notify({ type: 'positive', message: t('evaluationPage.runDeleted') });
        } catch (e) {
          $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('evaluationPage.runDeleteFailed') });
        }
      },
    );
  }

  function buildExperimentVariants(): ExperimentVariant[] {
    const agents = [
      runForm.value.agent_id,
      ...runForm.value.extra_agent_ids.filter((id) => id && id !== runForm.value.agent_id),
    ];
    const models = [
      runForm.value.model.trim(),
      ...runForm.value.extra_models.map((m) => m.trim()).filter(Boolean),
    ].filter((m, i, all) => all.indexOf(m) === i);
    const modelAxis = models.length ? models : [''];
    const tools = [
      runForm.value.tools.trim(),
      ...runForm.value.extra_tools.map((x) => x.trim()).filter(Boolean),
    ].filter((x, i, all) => all.indexOf(x) === i);
    const toolAxis = tools.length ? tools : [''];
    const prompt = runForm.value.prompt.trim();
    const out: ExperimentVariant[] = [];
    for (const agent_id of agents) {
      for (const model of modelAxis) {
        for (const tool of toolAxis) {
          out.push({
            agent_id,
            model: model || undefined,
            prompt: prompt || undefined,
            tools: tool || undefined,
          });
        }
      }
    }
    return out;
  }

  const experimentPivots = computed(() => {
    const grouped = new Map<string, EvalRun[]>();
    for (const run of runs.value) {
      if (!run.experiment_id) continue;
      const list = grouped.get(run.experiment_id) ?? [];
      list.push(run);
      grouped.set(run.experiment_id, list);
    }
    return [...grouped.entries()]
      .filter(([, items]) => items.length > 1)
      .map(([experiment_id, items]) => ({
        experiment_id,
        items: items.slice().sort((a, b) => (a.variant_label || '').localeCompare(b.variant_label || '')),
      }));
  });

  async function openVersions() {
    if (!selectedDatasetId.value) return;
    versionsOpen.value = true;
    versionsLoading.value = true;
    try {
      versions.value = await evaluationStore.loadVersions(selectedDatasetId.value);
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('evaluationPage.versionsLoadFailed') });
    } finally {
      versionsLoading.value = false;
    }
  }

  function runPinnedVersion(row: EvalDatasetVersion) {
    runForm.value.dataset_version_id = row.id;
    runForm.value.dataset_version = row.version;
    versionsOpen.value = false;
    runOpen.value = true;
  }

  async function submitRun() {
    if (!selectedDatasetId.value || !runForm.value.agent_id) {
      $q.notify({ type: 'warning', message: '请选择 Agent' });
      return;
    }
    runLoading.value = true;
    try {
      const variants = buildExperimentVariants();
      const shared = {
        dataset_id: selectedDatasetId.value,
        metrics: runForm.value.metrics.trim() || undefined,
        num_runs: runForm.value.num_runs > 1 ? runForm.value.num_runs : undefined,
        use_user_simulation: runForm.value.use_user_simulation || undefined,
        dataset_version_id: runForm.value.dataset_version_id || undefined,
      };
      if (variants.length > 1) {
        await evaluationStore.startExperiment({
          ...shared,
          variants,
        });
      } else {
        const cell = variants[0];
        await evaluationStore.startRun({
          ...shared,
          agent_id: cell.agent_id,
          model: cell.model,
          prompt: cell.prompt,
        });
      }
      runOpen.value = false;
      runForm.value.dataset_version_id = '';
      runForm.value.dataset_version = 0;
      await loadRuns();
      $q.notify({ type: 'positive', message: '评估已提交' });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '启动失败' });
    } finally {
      runLoading.value = false;
    }
  }

  // ISSUE-001: upload cases into the selected dataset (JSON array textarea).
  async function submitUpload() {
    const raw = uploadText.value.trim();
    if (!selectedDatasetId.value) return;
    if (!raw) {
      $q.notify({ type: 'warning', message: t('evaluationPage.uploadEmpty') });
      return;
    }
    try {
      const parsed: unknown = JSON.parse(raw);
      if (!Array.isArray(parsed) || parsed.length === 0) {
        throw new Error('not-array');
      }
    } catch {
      $q.notify({ type: 'warning', message: t('evaluationPage.uploadInvalidJson') });
      return;
    }
    uploadLoading.value = true;
    try {
      const n = await evaluationStore.importCases(selectedDatasetId.value, raw);
      uploadOpen.value = false;
      uploadText.value = '';
      // Refresh datasets so the case_count column reflects the import.
      await evaluationStore.loadDatasets({ limit: 100 });
      $q.notify({ type: 'positive', message: t('evaluationPage.uploadImported', { n }) });
    } catch (e) {
      $q.notify({
        type: 'negative',
        message: e instanceof Error ? e.message : t('evaluationPage.uploadImportFailed'),
      });
    } finally {
      uploadLoading.value = false;
    }
  }

  function updateResultRow(row: EvalCaseResult) {
    caseResults.value = caseResults.value.map((r) => (r.id === row.id ? row : r));
  }

  async function saveAnnotation(row: EvalCaseResult) {
    if (!resultsRun.value) return;
    savingResultId.value = row.id;
    try {
      // 行内三态（unset/pass/fail、空分数）全量落库：null/undefined 走显式清除位，
      // 否则发送值字段（后端清除位优先于值字段）。
      const input: AnnotateCaseResultInput = {
        run_id: resultsRun.value.id,
        result_id: row.id,
        human_comment: row.human_comment,
      };
      if (row.human_pass == null) input.clear_human_pass = true;
      else input.human_pass = row.human_pass;
      if (row.human_score == null) input.clear_human_score = true;
      else input.human_score = row.human_score;
      const updated = await evaluationStore.annotateResult(input);
      updateResultRow(updated);
      // Annotation directly feeds the judge calibration summary.
      void loadDivergence();
      $q.notify({ type: 'positive', message: '标注已保存' });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '保存失败' });
    } finally {
      savingResultId.value = '';
    }
  }

  async function loadTrend() {
    const seq = ++trendSeq;
    if (!trendAgentId.value) return;
    trendLoading.value = true;
    try {
      const res = await evaluationStore.loadAgentTrend({
        agent_id: trendAgentId.value,
        dataset_id: selectedDatasetId.value || undefined,
        limit: 20,
      });
      if (seq !== trendSeq) return; // stale — agent/dataset switched mid-flight
      trendPoints.value = res;
      // The divergence panel shares the trend agent as its filter.
      void loadDivergence();
    } catch (e) {
      if (seq === trendSeq) {
        $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '加载趋势失败' });
      }
    } finally {
      if (seq === trendSeq) trendLoading.value = false;
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

  // P2-3: failure groups follow the trend agent filter (online vs manual runs).
  async function loadFailureGroups() {
    const seq = ++failureGroupsSeq;
    if (!selectedDatasetId.value) {
      failureGroups.value = [];
      failureGroupsTotal.value = 0;
      return;
    }
    failureGroupsLoading.value = true;
    try {
      const res = await evaluationStore.loadFailureGroups(selectedDatasetId.value, {
        agent_id: trendAgentId.value || undefined,
      });
      if (seq !== failureGroupsSeq) return;
      failureGroups.value = res.groups;
      failureGroupsTotal.value = res.total_failed;
    } catch (e) {
      // Auxiliary panel — degrade to empty instead of notifying.
      console.warn('[evaluation] load failure groups failed:', e);
      if (seq === failureGroupsSeq) {
        failureGroups.value = [];
        failureGroupsTotal.value = 0;
      }
    } finally {
      if (seq === failureGroupsSeq) failureGroupsLoading.value = false;
    }
  }

  // P3-3: pairwise preference — challengeRow is a non-baseline comparison row;
  // the baseline is always comparisons[0] (backend sorts by created_at asc).
  async function loadPreferences() {
    const seq = ++preferencesSeq;
    if (!selectedDatasetId.value) {
      preferences.value = [];
      return;
    }
    preferencesLoading.value = true;
    try {
      const res = await evaluationStore.loadPreferences(selectedDatasetId.value);
      if (seq !== preferencesSeq) return;
      preferences.value = res;
    } catch (e) {
      console.warn('[evaluation] load preferences failed:', e);
      if (seq === preferencesSeq) preferences.value = [];
    } finally {
      if (seq === preferencesSeq) preferencesLoading.value = false;
    }
  }

  function submitPreferenceWinner(challengeRow: EvalRunComparison) {
    const base = comparisons.value[0];
    if (!base || challengeRow.run_id === base.run_id) return;
    $q.dialog({
      title: t('evaluationPage.preferenceDialogTitle'),
      message: t('evaluationPage.preferenceDialogMessage'),
      prompt: { model: '', type: 'text' },
      cancel: true,
    }).onOk(async (comment: string) => {
      preferenceSaving.value = true;
      try {
        await evaluationStore.submitPreference({
          dataset_id: challengeRow.dataset_id,
          run_id_a: base.run_id,
          run_id_b: challengeRow.run_id,
          winner_run_id: challengeRow.run_id,
          comment: comment?.trim() || undefined,
        });
        await loadPreferences();
        $q.notify({ type: 'positive', message: t('evaluationPage.preferenceSaved') });
      } catch (e) {
        $q.notify({
          type: 'negative',
          message: e instanceof Error ? e.message : t('evaluationPage.preferenceSaveFailed'),
        });
      } finally {
        preferenceSaving.value = false;
      }
    });
  }

  function openResultTrace(row: EvalCaseResult) {
    openMonitorTab('traces', {
      session: row.session_id || undefined,
      trace: row.trace_run_id || undefined,
    });
  }

  async function loadCaseResults() {
    const seq = ++caseResultsSeq;
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
      if (seq !== caseResultsSeq) return; // stale — another run's dialog took over
      caseResults.value = res.items;
      caseResultsTotal.value = res.total;
      if (resultsPage.value > resultsPageMax.value) {
        resultsPage.value = resultsPageMax.value;
        res = await evaluationStore.loadRunResults(resultsRun.value.id, {
          limit,
          offset: (resultsPage.value - 1) * limit,
        });
        if (seq !== caseResultsSeq) return;
        caseResults.value = res.items;
        caseResultsTotal.value = res.total;
      }
    } catch (e) {
      if (seq === caseResultsSeq) {
        $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '加载结果失败' });
      }
    } finally {
      if (seq === caseResultsSeq) resultsLoading.value = false;
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
      if (res.total > res.items.length) {
        $q.notify({
          type: 'warning',
          message: t('evaluationPage.exportTruncated', { exported: res.items.length, total: res.total }),
        });
      }
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

  onMounted(async () => {
    await Promise.all([loadAgentOptions(), loadDatasets()]);
    // No dataset auto-selected (empty workspace) → trend was never chained off
    // loadRuns; fall back to the agent-wide trend so the panel is not empty.
    if (!selectedDatasetId.value && trendAgentId.value) {
      void loadTrend();
    }
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
    loadDivergence,
    failureGroups,
    failureGroupsTotal,
    failureGroupsLoading,
    loadFailureGroups,
    preferences,
    preferencesLoading,
    preferenceSaving,
    loadPreferences,
    submitPreferenceWinner,
    datasetChanged,
    gateOpen,
    gateLoading,
    gateSaving,
    gateForm,
    openGate,
    saveGate,
    openResultTrace,
  };
}
