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
    nodes: (g?.nodes as NodeDef[]) ?? [],
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
    createdAt: (g?.createdAt as string) ?? "",
    updatedAt: (g?.updatedAt as string) ?? "",
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
    status: res.status ?? "",
    currentState: (res.currentState as Record<string, unknown>) ?? {},
    steps: (res.steps ?? []).map(wireStep),
    interruptNode: res.interruptNode ?? "",
    startedAt: res.startedAt ?? "",
    finishedAt: res.finishedAt ?? "",
  };
}

export async function listGraphExecutions(graphId: string, pageSize = 30, pageToken = ""): Promise<{ items: GraphExecutionSummary[]; nextPageToken: string }> {
  const svc = createGraphService();
  const res = await svc.ListGraphExecutions({ graphId, pageSize, pageToken });
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

export async function visualizeGraph(graphId: string, format = "json"): Promise<{ content: string; nodes: VisualGraphNode[]; edges: VisualGraphEdge[] }> {
  const svc = createGraphService();
  const res = await svc.VisualizeGraph({ graphId, format });
  return {
    content: res.content ?? "",
    nodes: (res.nodes ?? []) as VisualGraphNode[],
    edges: (res.edges ?? []) as VisualGraphEdge[],
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

export async function heartbeat(taskId: string, agentKey: string, metadata = ""): Promise<{ acknowledged: boolean; leaseExtensionSeconds: number }> {
  const svc = createGraphService();
  const res = await svc.Heartbeat({ taskId, agentKey, metadata });
  return { acknowledged: res.acknowledged ?? false, leaseExtensionSeconds: res.leaseExtensionSeconds ?? 0 };
}

export async function reportBlocked(taskId: string, reason: string, metadata = ""): Promise<Task> {
  const svc = createGraphService();
  const res = await svc.ReportBlocked({ taskId, reason, metadata });
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
