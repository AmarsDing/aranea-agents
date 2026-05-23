import { defineStore } from "pinia";
import { ref } from "vue";
import {
  listGraphs,
  getGraph,
  createGraph,
  updateGraph,
  deleteGraph,
  executeGraph,
  getGraphExecution,
  cancelGraphExecution,
  resumeGraph,
  validateGraph,
  listGraphTemplates,
  createGraphFromTemplate,
  listCheckpoints,
  getStateSnapshot,
  editState,
  timeTravelGraph,
  listTasks,
  getTask,
  claimTask,
  submitTaskResult,
  reportBlocked,
  unblockTask,
  reviewTask,
  listTaskComments,
  addTaskComment,
  listTaskLogs,
  listTaskRuns,
  exportGraph,
  importGraph,
  listGraphVersions,
  rollbackGraphVersion,
  saveGraphAsTemplate,
  type GraphDefinition,
  type GraphExecution,
  type GraphTemplateInfo,
  type GraphVersionInfo,
  type ValidationResult,
  type CheckpointInfo,
  type Task,
  type TaskComment,
  type TaskLog,
  type TaskRun,
  type TaskStatus,
} from "../../features/graph/api";

export const useGraphStore = defineStore("graph", () => {
  const graphs = ref<GraphDefinition[]>([]);
  const activeGraph = ref<GraphDefinition | null>(null);
  const loading = ref(false);
  const total = ref(0);
  const templates = ref<GraphTemplateInfo[]>([]);
  const templatesLoading = ref(false);
  const lastValidation = ref<ValidationResult | null>(null);

  async function loadGraphs(pageSize = 50, pageToken = "") {
    loading.value = true;
    try {
      const result = await listGraphs(pageSize, pageToken);
      graphs.value = result.items ?? [];
      total.value = graphs.value.length;
    } finally {
      loading.value = false;
    }
  }

  async function fetchGraph(id: string) {
    const g = await getGraph(id);
    activeGraph.value = g;
    return g;
  }

  async function addGraph(payload: Partial<GraphDefinition>) {
    const created = await createGraph(payload);
    graphs.value.unshift(created);
    activeGraph.value = created;
    return created;
  }

  async function editGraph(id: string, payload: Partial<GraphDefinition>) {
    const updated = await updateGraph(id, payload);
    graphs.value = graphs.value.map((g) => (g.id === id ? updated : g));
    if (activeGraph.value?.id === id) activeGraph.value = updated;
    return updated;
  }

  async function removeGraph(id: string) {
    await deleteGraph(id);
    graphs.value = graphs.value.filter((g) => g.id !== id);
    if (activeGraph.value?.id === id) activeGraph.value = null;
  }

  async function runGraph(id: string, sessionId: string, initialState?: Record<string, unknown>) {
    return executeGraph(id, sessionId, initialState);
  }

  async function fetchExecution(executionId: string): Promise<GraphExecution> {
    return getGraphExecution(executionId);
  }

  async function cancelExecution(executionId: string) {
    return cancelGraphExecution(executionId);
  }

  async function resumeExecution(executionId: string, resumeValue?: Record<string, unknown>) {
    return resumeGraph(executionId, resumeValue);
  }

  async function validateGraphDefinition(graphId: string): Promise<ValidationResult> {
    const result = await validateGraph(graphId);
    lastValidation.value = result;
    return result;
  }

  async function loadTemplates() {
    templatesLoading.value = true;
    try {
      templates.value = await listGraphTemplates();
    } finally {
      templatesLoading.value = false;
    }
  }

  async function instantiateTemplate(templateId: string, name: string, description: string) {
    const created = await createGraphFromTemplate(templateId, name, description);
    graphs.value.unshift(created);
    activeGraph.value = created;
    return created;
  }

  async function fetchCheckpoints(executionId: string, limit = 50): Promise<CheckpointInfo[]> {
    return listCheckpoints(executionId, limit);
  }

  async function fetchStateSnapshot(executionId: string, checkpointId: string, namespace = "") {
    return getStateSnapshot(executionId, checkpointId, namespace);
  }

  async function editStateSnapshot(
    executionId: string,
    checkpointId: string,
    namespace: string,
    patch: Record<string, unknown>,
  ) {
    return editState(executionId, checkpointId, namespace, patch);
  }

  async function timeTravelExecution(executionId: string, stepIndex: number) {
    return timeTravelGraph(executionId, stepIndex);
  }

  async function fetchTasks(executionId: string, statusFilter?: TaskStatus) {
    return listTasks(executionId, statusFilter);
  }

  async function fetchTask(taskId: string) {
    return getTask(taskId);
  }

  async function claimTaskByAgent(taskId: string, agentKey: string) {
    return claimTask(taskId, agentKey);
  }

  async function submitTask(taskId: string, output: string, summary: string, metadata = "") {
    return submitTaskResult(taskId, output, summary, metadata);
  }

  async function reportTaskBlocked(taskId: string, reason: string, metadata = "") {
    return reportBlocked(taskId, reason, metadata);
  }

  async function unblockTaskByOperator(taskId: string, comment = "") {
    return unblockTask(taskId, comment);
  }

  async function reviewTaskResult(taskId: string, reviewerAgent: string, approved: boolean, comment = "") {
    return reviewTask(taskId, reviewerAgent, approved, comment);
  }

  async function fetchTaskComments(taskId: string): Promise<TaskComment[]> {
    return listTaskComments(taskId);
  }

  async function postTaskComment(taskId: string, author: string, content: string, type = "suggestion") {
    return addTaskComment(taskId, author, content, type);
  }

  async function fetchTaskLogs(taskId: string): Promise<TaskLog[]> {
    return listTaskLogs(taskId);
  }

  async function fetchTaskRuns(taskId: string): Promise<TaskRun[]> {
    return listTaskRuns(taskId);
  }

  async function exportGraphDefinition(graphId: string) {
    return exportGraph(graphId);
  }

  async function importGraphDefinition(json: string, name = "", description = "") {
    const created = await importGraph(json, name, description);
    graphs.value.unshift(created);
    activeGraph.value = created;
    return created;
  }

  async function fetchGraphVersions(graphId: string): Promise<GraphVersionInfo[]> {
    return listGraphVersions(graphId);
  }

  async function rollbackGraph(graphId: string, version: number) {
    const updated = await rollbackGraphVersion(graphId, version);
    graphs.value = graphs.value.map((g) => (g.id === graphId ? updated : g));
    if (activeGraph.value?.id === graphId) activeGraph.value = updated;
    return updated;
  }

  async function saveAsTemplate(graphId: string, templateName: string, category = "custom", description = "") {
    const result = await saveGraphAsTemplate(graphId, templateName, category, description);
    await loadTemplates();
    return result;
  }

  return {
    graphs,
    activeGraph,
    loading,
    total,
    templates,
    templatesLoading,
    lastValidation,
    loadGraphs,
    fetchGraph,
    addGraph,
    editGraph,
    removeGraph,
    runGraph,
    fetchExecution,
    cancelExecution,
    resumeExecution,
    validateGraphDefinition,
    loadTemplates,
    instantiateTemplate,
    fetchCheckpoints,
    fetchStateSnapshot,
    editStateSnapshot,
    timeTravelExecution,
    fetchTasks,
    fetchTask,
    claimTaskByAgent,
    submitTask,
    reportTaskBlocked,
    unblockTaskByOperator,
    reviewTask: reviewTaskResult,
    fetchTaskComments,
    postTaskComment,
    fetchTaskLogs,
    fetchTaskRuns,
    exportGraphDefinition,
    importGraphDefinition,
    fetchGraphVersions,
    rollbackGraph,
    saveAsTemplate,
  };
});
