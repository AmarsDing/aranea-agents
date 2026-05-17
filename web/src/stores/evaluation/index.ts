import { defineStore } from "pinia";
import { ref } from "vue";
import {
  createDataset,
  deleteDataset,
  getRun,
  getRunResults,
  listDatasets,
  listRuns,
  runEvaluation,
  uploadCases
} from "../../features/evaluation/api";
import type {
  CreateDatasetInput,
  EvalCaseResult,
  EvalDataset,
  EvalRun,
  GetRunResultsResult,
  ListDatasetsParams,
  ListDatasetsResult,
  ListRunsParams,
  ListRunsResult,
  RunEvaluationInput
} from "../../features/evaluation/types";

export const useEvaluationStore = defineStore("evaluation", () => {
  const datasets = ref<EvalDataset[]>([]);
  const datasetsTotal = ref(0);
  const runs = ref<EvalRun[]>([]);
  const runsTotal = ref(0);
  const loading = ref(false);

  async function loadDatasets(params: ListDatasetsParams = {}): Promise<ListDatasetsResult> {
    loading.value = true;
    try {
      const result = await listDatasets(params);
      datasets.value = result.items;
      datasetsTotal.value = result.total;
      return result;
    } finally {
      loading.value = false;
    }
  }

  async function addDataset(input: CreateDatasetInput): Promise<EvalDataset> {
    const created = await createDataset(input);
    datasets.value.unshift(created);
    datasetsTotal.value += 1;
    return created;
  }

  async function removeDataset(id: string): Promise<void> {
    await deleteDataset(id);
    datasets.value = datasets.value.filter((d) => d.id !== id);
    datasetsTotal.value = Math.max(0, datasetsTotal.value - 1);
  }

  async function importCases(datasetId: string, casesJson: string): Promise<number> {
    return uploadCases(datasetId, casesJson);
  }

  async function startRun(input: RunEvaluationInput): Promise<EvalRun> {
    const run = await runEvaluation(input);
    runs.value.unshift(run);
    runsTotal.value += 1;
    return run;
  }

  async function loadRuns(params: ListRunsParams = {}): Promise<ListRunsResult> {
    const result = await listRuns(params);
    runs.value = result.items;
    runsTotal.value = result.total;
    return result;
  }

  async function refreshRun(id: string): Promise<EvalRun> {
    const updated = await getRun(id);
    runs.value = runs.value.map((r) => (r.id === id ? updated : r));
    return updated;
  }

  async function loadRunResults(
    runId: string,
    params: { limit?: number; offset?: number } = {}
  ): Promise<GetRunResultsResult> {
    return getRunResults(runId, params);
  }

  return {
    datasets,
    datasetsTotal,
    runs,
    runsTotal,
    loading,
    loadDatasets,
    addDataset,
    removeDataset,
    importCases,
    startRun,
    loadRuns,
    refreshRun,
    loadRunResults
  };
});
