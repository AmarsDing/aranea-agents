import { createGraphService } from "../../services";
import type {
  GraphDefinition,
  GraphExecution,
  GraphExecutionSummary,
  GraphStepSnapshot,
  VisualGraphNode,
  VisualGraphEdge,
  CheckpointInfo,
  StateFieldDef,
  NodeDef,
  EdgeDef,
  ConditionalEdgeDef,
  SubgraphDef,
  ValidationResult,
  ValidationError,
  ValidationWarning,
  GraphTemplateInfo,
  GraphVersionInfo,
  TemplateNodeInfo,
  TemplateEdgeInfo,
  Task,
  TaskComment,
  TaskLog,
  TaskRun,
  TaskEvent,
  TaskStatus,
} from "./types";

export type {
  GraphDefinition,
  GraphExecution,
  GraphExecutionSummary,
  GraphStepSnapshot,
  VisualGraphNode,
  VisualGraphEdge,
  CheckpointInfo,
  NodeType,
  ReducerType,
  StateFieldDef,
  NodeDef,
  EdgeDef,
  ConditionalEdgeDef,
  SubgraphDef,
  NodeStyleConfig,
  ValidationResult,
  ValidationError,
  ValidationWarning,
  GraphTemplateInfo,
  GraphVersionInfo,
  TemplateNodeInfo,
  TemplateEdgeInfo,
  Task,
  TaskComment,
  TaskLog,
  TaskRun,
  TaskEvent,
  TaskStatus,
} from "./types";

function wireGraph(g: Record<string, unknown> | null | undefined): GraphDefinition {
  return {
    id: (g?.id as string) ?? "",
    name: (g?.name as string) ?? "",
    description: (g?.description as string) ?? "",
    stateFields: (g?.stateFields as StateFieldDef[]) ?? [],
    nodes: (g?.nodes as NodeDef[])?.map((n) => wireNode(n as Record<string, unknown>)) ?? [],
    edges: (g?.edges as EdgeDef[]) ?? [],
    conditionalEdges: (g?.conditionalEdges as ConditionalEdgeDef[]) ?? [],
    subgraphs: (g?.subgraphs as SubgraphDef[]) ?? [],
    entryPoint: (g?.entryPoint as string) ?? "",
    finishPoint: (g?.finishPoint as string) ?? "",
    enableCheckpoint: (g?.enableCheckpoint as boolean) ?? false,
    executionEngine: (g?.executionEngine as string) ?? "bsp",
    interruptBefore: (g?.interruptBefore as string[]) ?? [],
    interruptAfter: (g?.interruptAfter as string[]) ?? [],
    metadata: (g?.metadata as Record<string, unknown>) ?? {},
    version: (g?.version as number) ?? 0,
    sortOrder: (g?.sortOrder as number) ?? 0,
    createdAt: (g?.createdAt as string) ?? "",
    updatedAt: (g?.updatedAt as string) ?? "",
  };
}

function wireNode(n: Record<string, unknown> | null | undefined): NodeDef {
  return {
    id: (n?.id as string) ?? "",
    funcRef: (n?.funcRef as string) ?? "",
    interruptBefore: (n?.interruptBefore as boolean) ?? false,
    interruptAfter: (n?.interruptAfter as boolean) ?? false,
    type: ((n?.type as NodeDef["type"]) ?? "function") as NodeDef["type"],
    description: (n?.description as string) ?? "",
    instruction: (n?.instruction as string) ?? "",
    modelName: (n?.modelName as string) ?? "",
    toolNames: (n?.toolNames as string[]) ?? [],
    agentName: (n?.agentName as string) ?? "",
    destinations: (n?.destinations as string[]) ?? [],
    requiredRole: (n?.requiredRole as string) ?? "",
    assignmentMode: (n?.assignmentMode as string) ?? "",
    assignmentStrategy: (n?.assignmentStrategy as string) ?? "",
    reviewerAgent: (n?.reviewerAgent as string) ?? "",
    reviewRules: (n?.reviewRules as string) ?? "",
    timeoutSeconds: (n?.timeoutSeconds as number) ?? 0,
    heartbeatIntervalSeconds: (n?.heartbeatIntervalSeconds as number) ?? 0,
    enableLeaseExtension: (n?.enableLeaseExtension as boolean) ?? false,
    retryMaxAttempts: (n?.retryMaxAttempts as number) ?? 0,
    failureAction: (n?.failureAction as string) ?? "",
    fallbackAgent: (n?.fallbackAgent as string) ?? "",
    inputMapperJson: (n?.inputMapperJson as string) ?? "",
    outputMapperJson: (n?.outputMapperJson as string) ?? "",
    isolatedMessages: (n?.isolatedMessages as boolean) ?? false,
    inputFromLastResponse: (n?.inputFromLastResponse as boolean) ?? false,
    cacheEnabled: (n?.cacheEnabled as boolean) ?? false,
    cacheTtlSeconds: (n?.cacheTtlSeconds as number) ?? 0,
  };
}

function wireExecSummary(e: Record<string, unknown> | null | undefined): GraphExecutionSummary {
  return {
    executionId: (e?.executionId as string) ?? "",
    graphId: (e?.graphId as string) ?? "",
    sessionId: (e?.sessionId as string) ?? "",
    status: (e?.status as string) ?? "",
    currentNode: (e?.currentNode as string) ?? "",
    lineageId: (e?.lineageId as string) ?? "",
    errorMessage: (e?.errorMessage as string) ?? "",
    startedAt: (e?.startedAt as string) ?? "",
    finishedAt: (e?.finishedAt as string) ?? "",
  };
}

function wireStep(s: Record<string, unknown> | null | undefined): GraphStepSnapshot {
  return {
    nodeId: (s?.nodeId as string) ?? "",
    stepIndex: (s?.stepIndex as number) ?? 0,
    inputState: (s?.inputState as Record<string, unknown>) ?? {},
    outputState: (s?.outputState as Record<string, unknown>) ?? {},
    status: (s?.status as string) ?? "",
    error: (s?.error as string) ?? "",
    timestamp: (s?.timestamp as string) ?? "",
  };
}

export async function listGraphs(pageSize = 50, pageToken = ""): Promise<{ items: GraphDefinition[]; nextPageToken: string }> {
  const svc = createGraphService();
  const res = await svc.ListGraphs({ pageSize, pageToken });
  return {
    items: (res.items ?? []).map(wireGraph),
    nextPageToken: res.nextPageToken ?? "",
  };
}

export async function getGraph(id: string): Promise<GraphDefinition> {
  const svc = createGraphService();
  const res = await svc.GetGraph({ id });
  return wireGraph(res.graph);
}

export async function createGraph(payload: Partial<GraphDefinition>): Promise<GraphDefinition> {
  const svc = createGraphService();
  const res = await svc.CreateGraph({
    name: payload.name,
    description: payload.description,
    stateFields: payload.stateFields,
    nodes: payload.nodes,
    edges: payload.edges,
    conditionalEdges: payload.conditionalEdges,
    subgraphs: payload.subgraphs,
    entryPoint: payload.entryPoint,
    finishPoint: payload.finishPoint,
    enableCheckpoint: payload.enableCheckpoint,
    executionEngine: payload.executionEngine,
    interruptBefore: payload.interruptBefore,
    interruptAfter: payload.interruptAfter,
    metadata: payload.metadata,
  });
  return wireGraph(res.graph);
}

export async function updateGraph(id: string, payload: Partial<GraphDefinition>): Promise<GraphDefinition> {
  const svc = createGraphService();
  const res = await svc.UpdateGraph({
    id,
    name: payload.name,
    description: payload.description,
    stateFields: payload.stateFields,
    nodes: payload.nodes,
    edges: payload.edges,
    conditionalEdges: payload.conditionalEdges,
    subgraphs: payload.subgraphs,
    entryPoint: payload.entryPoint,
    finishPoint: payload.finishPoint,
    enableCheckpoint: payload.enableCheckpoint,
    executionEngine: payload.executionEngine,
    interruptBefore: payload.interruptBefore,
    interruptAfter: payload.interruptAfter,
    metadata: payload.metadata,
  });
  return wireGraph(res.graph);
}

export async function deleteGraph(id: string): Promise<void> {
  const svc = createGraphService();
  await svc.DeleteGraph({ id });
}

export async function executeGraph(graphId: string, sessionId: string, initialState?: Record<string, unknown>, agentKey?: string): Promise<{ executionId: string; status: string }> {
  const svc = createGraphService();
  const res = await svc.ExecuteGraph({ graphId, sessionId, initialState, agentKey });
  return { executionId: res.executionId ?? "", status: res.status ?? "" };
}

export async function getGraphExecution(executionId: string): Promise<GraphExecution> {
  const svc = createGraphService();
  const res = await svc.GetGraphExecution({ executionId });
  return {
    executionId: res.executionId ?? "",
    graphId: res.graphId ?? "",
    sessionId: res.sessionId ?? "",
    status: res.status ?? "",
    currentState: (res.currentState as Record<string, unknown>) ?? {},
    steps: (res.steps ?? []).map(wireStep),
    interruptNode: res.interruptNode ?? "",
    startedAt: res.startedAt ?? "",
    finishedAt: res.finishedAt ?? "",
  };
}

export async function listGraphExecutions(
  graphId: string,
  pageSize = 30,
  pageToken = "",
  filters?: { status?: string; startedAfter?: string },
): Promise<{ items: GraphExecutionSummary[]; nextPageToken: string }> {
  const svc = createGraphService();
  const res = await svc.ListGraphExecutions({
    graphId,
    pageSize,
    pageToken,
    status: filters?.status || undefined,
    startedAfter: filters?.startedAfter || undefined,
  });
  return {
    items: (res.items ?? []).map(wireExecSummary),
    nextPageToken: res.nextPageToken ?? "",
  };
}

export async function cancelGraphExecution(executionId: string): Promise<string> {
  const svc = createGraphService();
  const res = await svc.CancelGraphExecution({ executionId });
  return res.status ?? "";
}

export async function resumeGraph(executionId: string, resumeValue?: Record<string, unknown>): Promise<{ executionId: string; status: string }> {
  const svc = createGraphService();
  const res = await svc.ResumeGraph({ executionId, resumeValue });
  return { executionId: res.executionId ?? "", status: res.status ?? "" };
}

export async function timeTravelGraph(executionId: string, stepIndex: number): Promise<{ executionId: string; stepIndex: number; stateSnapshot: Record<string, unknown>; nodeId: string }> {
  const svc = createGraphService();
  const res = await svc.TimeTravelGraph({ executionId, stepIndex });
  return {
    executionId: res.executionId ?? "",
    stepIndex: res.stepIndex ?? 0,
    stateSnapshot: (res.stateSnapshot as Record<string, unknown>) ?? {},
    nodeId: res.nodeId ?? "",
  };
}

export async function listCheckpoints(executionId: string, limit = 50): Promise<CheckpointInfo[]> {
  const svc = createGraphService();
  const res = await svc.ListCheckpoints({ executionId, limit });
  return (res.items ?? []) as CheckpointInfo[];
}

export async function getStateSnapshot(executionId: string, checkpointId: string, namespace = ""): Promise<Record<string, unknown>> {
  const svc = createGraphService();
  const res = await svc.GetStateSnapshot({ executionId, checkpointId, namespace });
  return (res.snapshot?.state as Record<string, unknown>) ?? {};
}

export async function editState(executionId: string, checkpointId: string, namespace: string, patch: Record<string, unknown>): Promise<{ newCheckpointId: string; lineageId: string }> {
  const svc = createGraphService();
  const res = await svc.EditState({ executionId, checkpointId, namespace, patch });
  return { newCheckpointId: res.newCheckpointId ?? "", lineageId: res.lineageId ?? "" };
}

export async function validateGraph(graphId: string): Promise<ValidationResult> {
  const svc = createGraphService();
  const res = await svc.ValidateGraph({ graphId });
  return {
    errors: (res.errors ?? []).map((e) => ({
      code: e.code ?? "",
      nodeId: e.nodeId ?? "",
      field: e.field ?? "",
      message: e.message ?? "",
    })),
    warnings: (res.warnings ?? []).map((w) => ({
      code: w.code ?? "",
      nodeId: w.nodeId ?? "",
      field: w.field ?? "",
      message: w.message ?? "",
    })),
    valid: res.valid ?? false,
  };
}

export async function listGraphTemplates(): Promise<GraphTemplateInfo[]> {
  const svc = createGraphService();
  const res = await svc.ListGraphTemplates({});
  return (res.templates ?? []).map((t) => ({
    id: t.id ?? "",
    name: t.name ?? "",
    description: t.description ?? "",
    category: t.category ?? "",
    nodes: (t.nodes ?? []).map((n) => ({
      nodeId: n.nodeId ?? "",
      type: n.type ?? "",
      label: n.label ?? "",
      description: n.description ?? "",
    })),
    edges: (t.edges ?? []).map((e) => ({
      fromNode: e.fromNode ?? "",
      toNode: e.toNode ?? "",
      type: e.type ?? "",
      label: e.label ?? "",
    })),
    stateFields: (t.stateFields ?? []) as StateFieldDef[],
    entryPoint: t.entryPoint ?? "",
    finishPoint: t.finishPoint ?? "",
  }));
}

export async function createGraphFromTemplate(templateId: string, name: string, description: string): Promise<GraphDefinition> {
  const svc = createGraphService();
  const res = await svc.CreateGraphFromTemplate({ templateId, name, description });
  return wireGraph(res.graph as Record<string, unknown>);
}

function wireTask(t: Record<string, unknown> | null | undefined): Task {
  return {
    taskId: (t?.taskId as string) ?? "",
    nodeId: (t?.nodeId as string) ?? "",
    executionId: (t?.executionId as string) ?? "",
    assignee: (t?.assignee as string) ?? "",
    status: (t?.status as TaskStatus) ?? "TASK_PENDING",
    context: (t?.context as string) ?? "",
    input: (t?.input as string) ?? "",
    output: (t?.output as string) ?? "",
    summary: (t?.summary as string) ?? "",
    metadata: (t?.metadata as string) ?? "",
    requiredRole: (t?.requiredRole as string) ?? "",
    assignmentMode: (t?.assignmentMode as string) ?? "",
    createdAt: (t?.createdAt as string) ?? "",
    claimedAt: (t?.claimedAt as string) ?? "",
    completedAt: (t?.completedAt as string) ?? "",
  };
}

export async function listTasks(executionId: string, statusFilter?: TaskStatus, pageSize = 50, pageToken = ""): Promise<{ items: Task[]; nextPageToken: string }> {
  const svc = createGraphService();
  const res = await svc.ListTasks({ executionId, statusFilter, pageSize, pageToken });
  return {
    items: (res.items ?? []).map(wireTask),
    nextPageToken: res.nextPageToken ?? "",
  };
}

export async function getTask(taskId: string): Promise<Task> {
  const svc = createGraphService();
  const res = await svc.GetTask({ taskId });
  return wireTask(res.task as Record<string, unknown>);
}

export async function claimTask(taskId: string, agentKey: string): Promise<Task> {
  const svc = createGraphService();
  const res = await svc.ClaimTask({ taskId, agentKey });
  return wireTask(res.task as Record<string, unknown>);
}

export async function submitTaskResult(taskId: string, output: string, summary: string, metadata: string): Promise<Task> {
  const svc = createGraphService();
  const res = await svc.SubmitTaskResult({ taskId, output, summary, metadata });
  return wireTask(res.task as Record<string, unknown>);
}

export async function reportBlocked(taskId: string, reason: string, metadata = ""): Promise<Task> {
  const svc = createGraphService();
  const res = await svc.ReportBlocked({ taskId, reason, metadata });
  return wireTask(res.task as Record<string, unknown>);
}

export async function unblockTask(taskId: string, comment = ""): Promise<Task> {
  const svc = createGraphService();
  const res = await svc.UnblockTask({ taskId, comment });
  return wireTask(res.task as Record<string, unknown>);
}

export async function reviewTask(
  taskId: string,
  reviewerAgent: string,
  approved: boolean,
  comment = "",
): Promise<Task> {
  const svc = createGraphService();
  const res = await svc.ReviewTask({ taskId, reviewerAgent, approved, comment });
  return wireTask(res.task as Record<string, unknown>);
}

export async function listTaskComments(taskId: string): Promise<TaskComment[]> {
  const svc = createGraphService();
  const res = await svc.ListTaskComments({ taskId });
  return (res.comments ?? []) as TaskComment[];
}

export async function addTaskComment(taskId: string, author: string, content: string, type = "suggestion"): Promise<TaskComment> {
  const svc = createGraphService();
  const res = await svc.AddTaskComment({ taskId, author, content, type });
  return res.comment as TaskComment;
}

export async function listTaskLogs(taskId: string, stream = "", level = "", pageSize = 100): Promise<TaskLog[]> {
  const svc = createGraphService();
  const res = await svc.ListTaskLogs({ taskId, stream, level, pageSize });
  return (res.logs ?? []) as TaskLog[];
}

export async function listTaskRuns(taskId: string): Promise<TaskRun[]> {
  const svc = createGraphService();
  const res = await svc.ListTaskRuns({ taskId });
  return (res.runs ?? []) as TaskRun[];
}

export async function listTaskEvents(executionId: string, taskId = "", eventType = "", pageSize = 100): Promise<TaskEvent[]> {
  const svc = createGraphService();
  const res = await svc.ListTaskEvents({ executionId, taskId, eventType, pageSize });
  return (res.events ?? []) as TaskEvent[];
}

export async function createTask(executionId: string, nodeId: string, requiredRole = "", assignmentMode = "", assignmentStrategy = "", input = "", context = "", parentTaskIds: string[] = []): Promise<Task> {
  const svc = createGraphService();
  const res = await svc.CreateTask({ executionId, nodeId, requiredRole, assignmentMode, assignmentStrategy, input, context, parentTaskIds });
  return wireTask(res.task ?? {});
}

export async function linkTasks(parentTaskId: string, childTaskId: string): Promise<void> {
  const svc = createGraphService();
  await svc.LinkTasks({ parentTaskId, childTaskId });
}

export async function unlinkTasks(parentTaskId: string, childTaskId: string): Promise<void> {
  const svc = createGraphService();
  await svc.UnlinkTasks({ parentTaskId, childTaskId });
}

export async function exportGraph(graphId: string): Promise<{ json: string; graph: GraphDefinition }> {
  const svc = createGraphService();
  const res = await svc.ExportGraph({ graphId, format: "json" });
  return {
    json: res.json ?? "",
    graph: wireGraph(res.graph as Record<string, unknown>),
  };
}

export async function importGraph(json: string, name = "", description = ""): Promise<GraphDefinition> {
  const svc = createGraphService();
  const res = await svc.ImportGraph({ json, name, description });
  return wireGraph(res.graph as Record<string, unknown>);
}

export async function listGraphVersions(graphId: string): Promise<GraphVersionInfo[]> {
  const svc = createGraphService();
  const res = await svc.ListGraphVersions({ graphId });
  return (res.items ?? []).map((item) => ({
    version: item.version ?? 0,
    savedAt: (item.savedAt as string) ?? "",
    name: item.name ?? "",
  }));
}

export async function rollbackGraphVersion(graphId: string, version: number): Promise<GraphDefinition> {
  const svc = createGraphService();
  const res = await svc.RollbackGraphVersion({ graphId, version });
  return wireGraph(res.graph as Record<string, unknown>);
}

export async function saveGraphAsTemplate(
  graphId: string,
  templateName: string,
  category = "custom",
  description = "",
): Promise<{ templateId: string; template: GraphTemplateInfo }> {
  const svc = createGraphService();
  const res = await svc.SaveGraphAsTemplate({ graphId, templateName, category, description });
  const t = res.template;
  return {
    templateId: res.templateId ?? "",
    template: {
      id: t?.id ?? "",
      name: t?.name ?? "",
      description: t?.description ?? "",
      category: t?.category ?? "",
      nodes: (t?.nodes ?? []).map((n) => ({
        nodeId: n.nodeId ?? "",
        type: n.type ?? "",
        label: n.label ?? "",
        description: n.description ?? "",
      })),
      edges: (t?.edges ?? []).map((e) => ({
        fromNode: e.fromNode ?? "",
        toNode: e.toNode ?? "",
        type: e.type ?? "",
        label: e.label ?? "",
      })),
      stateFields: (t?.stateFields ?? []) as StateFieldDef[],
      entryPoint: t?.entryPoint ?? "",
      finishPoint: t?.finishPoint ?? "",
    },
  };
}

export async function reorderGraphs(ids: string[]): Promise<void> {
  const svc = createGraphService();
  await svc.ReorderGraphs({ ids });
}
