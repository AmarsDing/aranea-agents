export type NodeType = "function" | "llm" | "tool" | "agent" | "router" | "join" | "hitl";

export type ReducerType = "append" | "replace" | "merge" | "custom";

export type StateFieldDef = {
  name: string;
  type: string;
  reducer: ReducerType;
  defaultValue?: unknown;
  required: boolean;
  disableDeepCopy: boolean;
};

export type NodeDef = {
  id: string;
  funcRef: string;
  interruptBefore: boolean;
  interruptAfter: boolean;
  type: NodeType;
  description: string;
  instruction: string;
  modelName: string;
  toolNames: string[];
  agentName: string;
  destinations: string[];
  requiredRole: string;
  assignmentMode: string;
  assignmentStrategy: string;
  reviewerAgent: string;
  reviewRules: string;
  timeoutSeconds: number;
  heartbeatIntervalSeconds: number;
  enableLeaseExtension: boolean;
  retryMaxAttempts: number;
  failureAction: string;
  fallbackAgent: string;
  inputMapperJson: string;
  outputMapperJson: string;
  isolatedMessages: boolean;
  inputFromLastResponse: boolean;
  cacheEnabled: boolean;
  cacheTtlSeconds: number;
};

export type EdgeDef = {
  from: string;
  to: string;
  kind?: string;
};

export type ConditionalEdgeDef = {
  from: string;
  condFuncRef: string;
  pathMap: Record<string, string>;
};

export type SubgraphDef = {
  id: string;
  graphId: string;
  interruptBefore: boolean;
  interruptAfter: boolean;
};

export type GraphDefinition = {
  id: string;
  name: string;
  description: string;
  stateFields: StateFieldDef[];
  nodes: NodeDef[];
  edges: EdgeDef[];
  conditionalEdges: ConditionalEdgeDef[];
  subgraphs: SubgraphDef[];
  entryPoint: string;
  finishPoint: string;
  enableCheckpoint: boolean;
  executionEngine: string;
  interruptBefore: string[];
  interruptAfter: string[];
  metadata: Record<string, unknown>;
  version: number;
  createdAt: string;
  updatedAt: string;
};

export type GraphExecutionSummary = {
  executionId: string;
  graphId: string;
  sessionId: string;
  status: string;
  currentNode: string;
  lineageId: string;
  errorMessage: string;
  startedAt: string;
  finishedAt: string;
};

/** WS `graph_execution_done` metadata.execution_summary */
export type GraphRunExecutionSummary = {
  executionId: string;
  graphId: string;
  totalSteps: number;
  durationMs: number;
  finalStateKeys: number;
  nodes: GraphRunNodeSummary[];
};

export type GraphRunNodeSummary = {
  nodeId: string;
  nodeType: string;
  status: string;
  durationMs: number;
  error: string;
  stepNumber: number;
};

export type GraphLayoutMetadata = Record<string, { x: number; y: number }>;

export const GRAPH_LAYOUT_METADATA_KEY = "layout";

export type GraphStepSnapshot = {
  nodeId: string;
  stepIndex: number;
  inputState: Record<string, unknown>;
  outputState: Record<string, unknown>;
  status: string;
  error: string;
  timestamp: string;
};

export type GraphExecution = {
  executionId: string;
  graphId: string;
  sessionId: string;
  status: string;
  currentState: Record<string, unknown>;
  steps: GraphStepSnapshot[];
  interruptNode: string;
  startedAt: string;
  finishedAt: string;
};

export type VisualGraphNode = {
  id: string;
  label: string;
  type: string;
  shape: string;
  fillColor: string;
  borderColor: string;
};

export type VisualGraphEdge = {
  from: string;
  to: string;
  type: string;
  label: string;
};

export type CheckpointInfo = {
  lineageId: string;
  namespace: string;
  checkpointId: string;
  parentCheckpointId: string;
  source: string;
  step: number;
  timestamp: string;
};

export type NodeStyleConfig = {
  shape: string;
  fillColor: string;
  borderColor: string;
  icon: string;
  label: string;
};

export const NODE_TYPE_STYLES: Record<NodeType, NodeStyleConfig> = {
  function: { shape: "box", fillColor: "var(--graph-node-function-fill)", borderColor: "var(--graph-node-function-border)", icon: "functions", label: "Function" },
  llm: { shape: "box", fillColor: "var(--graph-node-llm-fill)", borderColor: "var(--graph-node-llm-border)", icon: "psychology", label: "LLM" },
  tool: { shape: "box", fillColor: "var(--graph-node-tool-fill)", borderColor: "var(--graph-node-tool-border)", icon: "handyman", label: "Tool" },
  agent: { shape: "box", fillColor: "var(--graph-node-agent-fill)", borderColor: "var(--graph-node-agent-border)", icon: "smart_toy", label: "Agent" },
  router: { shape: "diamond", fillColor: "var(--graph-node-router-fill)", borderColor: "var(--graph-node-router-border)", icon: "alt_route", label: "Router" },
  join: { shape: "diamond", fillColor: "var(--graph-node-join-fill)", borderColor: "var(--graph-node-join-border)", icon: "merge_type", label: "Join" },
  hitl: { shape: "box", fillColor: "var(--graph-node-hitl-fill)", borderColor: "var(--graph-node-hitl-border)", icon: "front_hand", label: "HITL" },
};

export const EXECUTION_STATUS_STYLES: Record<string, { color: string; icon: string; label: string }> = {
  idle: { color: "grey", icon: "radio_button_unchecked", label: "等待" },
  running: { color: "blue", icon: "sync", label: "运行中" },
  completed: { color: "green", icon: "check_circle", label: "完成" },
  failed: { color: "red", icon: "error", label: "失败" },
  interrupted: { color: "amber", icon: "pause_circle", label: "中断" },
  waiting: { color: "grey-6", icon: "schedule", label: "等待" },
};

export const REDUCER_OPTIONS: Array<{ label: string; value: ReducerType }> = [
  { label: "Append（追加）", value: "append" },
  { label: "Replace（替换）", value: "replace" },
  { label: "Merge（合并）", value: "merge" },
  { label: "Custom（自定义）", value: "custom" },
];

export const STATE_FIELD_TYPE_OPTIONS: Array<{ label: string; value: string }> = [
  { label: "String", value: "string" },
  { label: "Integer", value: "integer" },
  { label: "Float", value: "float" },
  { label: "Boolean", value: "boolean" },
  { label: "Array", value: "array" },
  { label: "Object", value: "object" },
];

export const ENGINE_OPTIONS: Array<{ label: string; value: string }> = [
  { label: "BSP（默认）", value: "bsp" },
  { label: "DAG（并行）", value: "dag" },
];

export const FAILURE_ACTION_OPTIONS: Array<{ label: string; value: string }> = [
  { label: "默认（retry_then_block）", value: "" },
  { label: "跳过后续（skip_on_failure）", value: "skip_on_failure" },
  { label: "跳过节点（skip）", value: "skip" },
  { label: "快速失败（fail_fast）", value: "fail_fast" },
  { label: "重试后阻塞（retry_then_block）", value: "retry_then_block" },
];

export type GraphVersionInfo = {
  version: number;
  savedAt: string;
  name: string;
};

export type ValidationError = {
  code: string;
  nodeId: string;
  field: string;
  message: string;
};

export type ValidationWarning = {
  code: string;
  nodeId: string;
  field: string;
  message: string;
};

export type ValidationResult = {
  errors: ValidationError[];
  warnings: ValidationWarning[];
  valid: boolean;
};

export type TemplateNodeInfo = {
  nodeId: string;
  type: string;
  label: string;
  description: string;
};

export type TemplateEdgeInfo = {
  fromNode: string;
  toNode: string;
  type: string;
  label: string;
};

export type GraphTemplateInfo = {
  id: string;
  name: string;
  description: string;
  category: string;
  nodes: TemplateNodeInfo[];
  edges: TemplateEdgeInfo[];
  stateFields: StateFieldDef[];
  entryPoint: string;
  finishPoint: string;
};

export type TaskStatus =
  | "TASK_PENDING"
  | "TASK_CLAIMED"
  | "TASK_COMPLETE"
  | "TASK_BLOCKED"
  | "TASK_REVIEW_REQUIRED"
  | "TASK_FAILED"
  | "TASK_TIMED_OUT"
  | "TASK_CANCELLED"
  | "TASK_CRASHED"
  | "TASK_PENDING_ASSIGNMENT";

export type Task = {
  taskId: string;
  nodeId: string;
  executionId: string;
  assignee: string;
  status: TaskStatus;
  context: string;
  input: string;
  output: string;
  summary: string;
  metadata: string;
  requiredRole: string;
  assignmentMode: string;
  createdAt: string;
  claimedAt: string;
  completedAt: string;
};

export type TaskComment = {
  commentId: string;
  taskId: string;
  author: string;
  content: string;
  type: string;
  createdAt: string;
};

export type TaskLog = {
  logId: string;
  taskId: string;
  stream: string;
  content: string;
  level: string;
  timestamp: string;
};

export type TaskRun = {
  runId: string;
  taskId: string;
  startedAt: string;
  finishedAt: string;
  exitCode: number;
  logRef: string;
};

export type TaskEvent = {
  eventId: string;
  taskId: string;
  eventType: string;
  sourceNode: string;
  description: string;
  timestamp: string;
};

export const TASK_STATUS_LABELS: Record<TaskStatus, string> = {
  TASK_PENDING: "等待中",
  TASK_CLAIMED: "执行中",
  TASK_COMPLETE: "已完成",
  TASK_BLOCKED: "阻塞",
  TASK_REVIEW_REQUIRED: "待审核",
  TASK_FAILED: "失败",
  TASK_TIMED_OUT: "超时",
  TASK_CANCELLED: "已取消",
  TASK_CRASHED: "崩溃",
  TASK_PENDING_ASSIGNMENT: "待指派",
};

export const TASK_STATUS_COLORS: Record<TaskStatus, string> = {
  TASK_PENDING: "grey",
  TASK_CLAIMED: "blue",
  TASK_COMPLETE: "green",
  TASK_BLOCKED: "amber",
  TASK_REVIEW_REQUIRED: "purple",
  TASK_FAILED: "red",
  TASK_TIMED_OUT: "orange",
  TASK_CANCELLED: "grey-5",
  TASK_CRASHED: "red-8",
  TASK_PENDING_ASSIGNMENT: "amber-8",
};
