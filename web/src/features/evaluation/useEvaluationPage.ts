import { computed, onMounted, ref } from "vue";
import { storeToRefs } from "pinia";
import { useQuasar } from "quasar";
import type { EvalCaseResult, EvalRun, EvalRunComparison, EvalTrendPoint } from "./types";
import { useEvaluationStore } from "../../stores/evaluation";
import { registryColWidth } from "../ui/registryTableColumns";

export function useEvaluationPage() {
  const $q = useQuasar();
  const evaluationStore = useEvaluationStore();
  const { datasets, runs, loading, agentOptions } = storeToRefs(evaluationStore);

  const selectedDatasetId = ref("");
  const runsLoading = ref(false);
  const error = ref("");
  const createOpen = ref(false);
  const createLoading = ref(false);
  const runOpen = ref(false);
  const runLoading = ref(false);
  const resultsOpen = ref(false);
  const resultsLoading = ref(false);
  const resultsRun = ref<EvalRun | null>(null);
  const caseResults = ref<EvalCaseResult[]>([]);
  const savingResultId = ref("");

  const trendAgentId = ref("");
  const trendPoints = ref<EvalTrendPoint[]>([]);
  const trendLoading = ref(false);
  const comparisons = ref<EvalRunComparison[]>([]);
  const compareLoading = ref(false);

  const createForm = ref({ name: "", description: "" });
  const runForm = ref({ agent_id: "", metrics: "", num_runs: 1 });

  const selectedDataset = computed(() => datasets.value.find((d) => d.id === selectedDatasetId.value));

  const runColumns = [
    { name: "id", label: "Run ID", field: "id", align: "left" as const, ...registryColWidth("10%") },
    { name: "agent_id", label: "Agent", field: "agent_id", align: "left" as const, ...registryColWidth("10%") },
    { name: "status", label: "状态", field: "status", align: "left" as const, ...registryColWidth("9%") },
    {
      name: "completed_cases",
      label: "进度",
      field: (r: EvalRun) => `${r.completed_cases}/${r.total_cases}`,
      align: "right" as const,
      ...registryColWidth("72px")
    },
    { name: "exact_match_score", label: "Exact", field: "exact_match_score", align: "right" as const, ...registryColWidth("64px") },
    { name: "actions", label: "", field: "id", align: "right" as const, ...registryColWidth("108px") }
  ];

  const resultColumns = [
    { name: "case_id", label: "Case", field: "case_id", align: "left" as const, ...registryColWidth("10%") },
    { name: "exact_match", label: "Exact", field: "exact_match", align: "center" as const, ...registryColWidth("64px") },
    { name: "contains_match", label: "Contains", field: "contains_match", align: "center" as const, ...registryColWidth("64px") },
    { name: "human_pass", label: "人工", field: "human_pass", align: "center" as const, ...registryColWidth("64px") },
    { name: "human_score", label: "分数", field: "human_score", align: "center" as const, ...registryColWidth("64px") },
    { name: "human_comment", label: "评语", field: "human_comment", align: "left" as const, ...registryColWidth("12%") },
    { name: "annotate", label: "", field: "id", align: "right" as const, ...registryColWidth("108px") }
  ];

  function runStatusColor(status: string) {
    if (status === "completed") return "positive";
    if (status === "failed") return "negative";
    if (status === "running") return "warning";
    return "grey";
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
    error.value = "";
    try {
      const res = await evaluationStore.loadDatasets({ limit: 100 });
      if (!selectedDatasetId.value && res.items.length) {
        selectedDatasetId.value = res.items[0].id;
        await loadRuns();
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : "加载失败";
    }
  }

  async function loadRuns() {
    if (!selectedDatasetId.value) return;
    runsLoading.value = true;
    try {
      await evaluationStore.loadRuns({ dataset_id: selectedDatasetId.value, limit: 50 });
      if (trendAgentId.value) {
        void loadTrend();
      }
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "加载运行记录失败" });
    } finally {
      runsLoading.value = false;
    }
  }

  function selectDataset(id: string) {
    selectedDatasetId.value = id;
    void loadRuns();
  }

  async function submitCreate() {
    if (!createForm.value.name.trim()) {
      $q.notify({ type: "warning", message: "请填写名称" });
      return;
    }
    createLoading.value = true;
    try {
      const ds = await evaluationStore.addDataset({
        name: createForm.value.name.trim(),
        description: createForm.value.description.trim()
      });
      createOpen.value = false;
      createForm.value = { name: "", description: "" };
      await loadDatasets();
      selectedDatasetId.value = ds.id;
      $q.notify({ type: "positive", message: "数据集已创建" });
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "创建失败" });
    } finally {
      createLoading.value = false;
    }
  }

  function confirmDeleteDataset() {
    const ds = selectedDataset.value;
    if (!ds) return;
    $q.dialog({ title: "删除数据集", message: `确定删除「${ds.name}」？`, cancel: true }).onOk(async () => {
      try {
        await evaluationStore.removeDataset(ds.id);
        if (selectedDatasetId.value === ds.id) {
          selectedDatasetId.value = "";
          runs.value = [];
        }
        await loadDatasets();
        $q.notify({ type: "positive", message: "已删除" });
      } catch (e) {
        $q.notify({ type: "negative", message: e instanceof Error ? e.message : "删除失败" });
      }
    });
  }

  async function submitRun() {
    if (!selectedDatasetId.value || !runForm.value.agent_id) {
      $q.notify({ type: "warning", message: "请选择 Agent" });
      return;
    }
    runLoading.value = true;
    try {
      await evaluationStore.startRun({
        dataset_id: selectedDatasetId.value,
        agent_id: runForm.value.agent_id,
        metrics: runForm.value.metrics.trim() || undefined,
        num_runs: runForm.value.num_runs > 1 ? runForm.value.num_runs : undefined
      });
      runOpen.value = false;
      await loadRuns();
      $q.notify({ type: "positive", message: "评估已提交" });
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "启动失败" });
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
        human_comment: row.human_comment
      });
      updateResultRow(updated);
      $q.notify({ type: "positive", message: "标注已保存" });
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "保存失败" });
    } finally {
      savingResultId.value = "";
    }
  }

  async function loadTrend() {
    if (!trendAgentId.value) return;
    trendLoading.value = true;
    try {
      trendPoints.value = await evaluationStore.loadAgentTrend({
        agent_id: trendAgentId.value,
        metric: runForm.value.metrics.trim() || undefined,
        limit: 20
      });
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "加载趋势失败" });
    } finally {
      trendLoading.value = false;
    }
  }

  async function submitCompare(runIds: string[]) {
    compareLoading.value = true;
    try {
      comparisons.value = await evaluationStore.compareRuns(runIds);
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "对比失败" });
    } finally {
      compareLoading.value = false;
    }
  }

  async function openResults(run: EvalRun) {
    resultsRun.value = run;
    resultsOpen.value = true;
    resultsLoading.value = true;
    try {
      const res = await evaluationStore.loadRunResults(run.id, { limit: 100 });
      caseResults.value = res.items;
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "加载结果失败" });
    } finally {
      resultsLoading.value = false;
    }
  }

  onMounted(() => {
    void loadAgentOptions();
    void loadDatasets();
  });

  return {
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
  };
}
