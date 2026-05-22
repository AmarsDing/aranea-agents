import { defineStore } from "pinia";
import { ref } from "vue";
import { listAgents } from "../../features/agents/api";
import {
  annotateCaseResult,
  compareEvalRuns,
  createDataset,
  deleteDataset,
  getAgentEvalTrend,
  getRun,
  getRunResults,
  listDatasets,
  listRuns,
  runEvaluation,
  uploadCases
} from "../../features/evaluation/api";
import type {
  AnnotateCaseResultInput,
  CreateDatasetInput,
  EvalCaseResult,
  EvalDataset,
  EvalRun,
  EvalRunComparison,
  EvalTrendPoint,
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
  const agentOptions = ref<{ label: string; value: string }[]>([]);

  async function loadAgentOptions(): Promise<{ label: string; value: string }[]> {
    try {
      const agents = await listAgents({ limit: 200 });
      agentOptions.value = agents.map((a) => ({ label: a.name || a.key || a.id, value: a.id }));
    } catch {
      agentOptions.value = [];
    }
    return agentOptions.value;
  }

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

  async function annotateResult(input: AnnotateCaseResultInput): Promise<EvalCaseResult> {
    return annotateCaseResult(input);
  }

  async function loadAgentTrend(params: {
    agent_id: string;
    metric?: string;
    limit?: number;
  }): Promise<EvalTrendPoint[]> {
    return getAgentEvalTrend(params);
  }

  async function compareRuns(runIds: string[]): Promise<EvalRunComparison[]> {
    return compareEvalRuns(runIds);
  }

  return {
    datasets,
    datasetsTotal,
    runs,
    runsTotal,
    loading,
    agentOptions,
    loadAgentOptions,
    loadDatasets,
    addDataset,
    removeDataset,
    importCases,
    startRun,
    loadRuns,
    refreshRun,
    loadRunResults,
    annotateResult,
    loadAgentTrend,
    compareRuns
  };
});
