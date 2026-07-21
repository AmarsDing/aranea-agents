export type NodeType = 'function' | 'llm' | 'tool' | 'agent' | 'router' | 'join' | 'hitl';

export type ReducerType = 'append' | 'cover' | 'merge' | 'default';

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
  kind: string;
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
  teamId?: string;
  isTemplate?: boolean;
  verificationGates?: string;
  version: number;
  sortOrder: number;
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

export const GRAPH_LAYOUT_METADATA_KEY = 'layout';

export const NODE_DEFAULT_WIDTH = 180;
export const NODE_DEFAULT_HEIGHT = 80;

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
  labelKey: string;
};

export const NODE_TYPE_STYLES: Record<NodeType, NodeStyleConfig> = {
  function: {
    shape: 'box',
    fillColor: 'var(--graph-node-function-fill)',
    borderColor: 'var(--graph-node-function-border)',
    icon: 'functions',
    labelKey: 'graphs.nodeTypeFunction',
  },
  llm: {
    shape: 'box',
    fillColor: 'var(--graph-node-llm-fill)',
    borderColor: 'var(--graph-node-llm-border)',
    icon: 'psychology',
    labelKey: 'graphs.nodeTypeLLM',
  },
  tool: {
    shape: 'box',
    fillColor: 'var(--graph-node-tool-fill)',
    borderColor: 'var(--graph-node-tool-border)',
    icon: 'build',
    labelKey: 'graphs.nodeTypeTool',
  },
  agent: {
    shape: 'box',
    fillColor: 'var(--graph-node-agent-fill)',
    borderColor: 'var(--graph-node-agent-border)',
    icon: 'smart_toy',
    labelKey: 'graphs.nodeTypeAgent',
  },
  router: {
    shape: 'diamond',
    fillColor: 'var(--graph-node-router-fill)',
    borderColor: 'var(--graph-node-router-border)',
    icon: 'alt_route',
    labelKey: 'graphs.nodeTypeRouter',
  },
  join: {
    shape: 'diamond',
    fillColor: 'var(--graph-node-join-fill)',
    borderColor: 'var(--graph-node-join-border)',
    icon: 'merge_type',
    labelKey: 'graphs.nodeTypeJoin',
  },
  hitl: {
    shape: 'box',
    fillColor: 'var(--graph-node-hitl-fill)',
    borderColor: 'var(--graph-node-hitl-border)',
    icon: 'front_hand',
    labelKey: 'graphs.nodeTypeHITL',
  },
};

export const EXECUTION_STATUS_STYLES: Record<string, { color: string; icon: string; labelKey: string }> = {
  idle: { color: 'grey', icon: 'radio_button_unchecked', labelKey: 'graphs.executionStatusIdle' },
  running: { color: 'cyan', icon: 'sync', labelKey: 'graphs.executionStatusRunning' },
  completed: { color: 'emerald', icon: 'check_circle', labelKey: 'graphs.executionStatusCompleted' },
  failed: { color: 'pink', icon: 'error', labelKey: 'graphs.executionStatusFailed' },
  interrupted: { color: 'amber', icon: 'pause_circle', labelKey: 'graphs.executionStatusInterrupted' },
  waiting: { color: 'grey-6', icon: 'schedule', labelKey: 'graphs.executionStatusWaiting' },
};

export const REDUCER_OPTIONS: Array<{ labelKey: string; value: ReducerType }> = [
  { labelKey: 'graphs.reducerAppend', value: 'append' },
  { labelKey: 'graphs.reducerCover', value: 'cover' },
  { labelKey: 'graphs.reducerMerge', value: 'merge' },
  { labelKey: 'graphs.reducerDefault', value: 'default' },
];

export const STATE_FIELD_TYPE_OPTIONS: Array<{ labelKey: string; value: string }> = [
  { labelKey: 'graphs.stateTypeString', value: 'string' },
  { labelKey: 'graphs.stateTypeInteger', value: 'integer' },
  { labelKey: 'graphs.stateTypeFloat', value: 'float' },
  { labelKey: 'graphs.stateTypeBoolean', value: 'boolean' },
  { labelKey: 'graphs.stateTypeArray', value: 'array' },
  { labelKey: 'graphs.stateTypeObject', value: 'object' },
];

export const ENGINE_OPTIONS: Array<{ labelKey: string; value: string }> = [
  { labelKey: 'graphs.engineBSP', value: 'bsp' },
  { labelKey: 'graphs.engineDAG', value: 'dag' },
];

export const FAILURE_ACTION_OPTIONS: Array<{ labelKey: string; value: string }> = [
  { labelKey: 'graphs.failureActionDefault', value: '' },
  { labelKey: 'graphs.failureActionSkipOnFailure', value: 'skip_on_failure' },
  { labelKey: 'graphs.failureActionSkip', value: 'skip' },
  { labelKey: 'graphs.failureActionFailFast', value: 'fail_fast' },
  { labelKey: 'graphs.failureActionRetryThenBlock', value: 'retry_then_block' },
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
  | 'TASK_PENDING'
  | 'TASK_CLAIMED'
  | 'TASK_COMPLETE'
  | 'TASK_BLOCKED'
  | 'TASK_REVIEW_REQUIRED'
  | 'TASK_FAILED'
  | 'TASK_TIMED_OUT'
  | 'TASK_CANCELLED'
  | 'TASK_CRASHED'
  | 'TASK_PENDING_ASSIGNMENT';

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

export const TASK_STATUS_LABEL_KEYS: Record<TaskStatus, string> = {
  TASK_PENDING: 'graphs.taskStatusPending',
  TASK_CLAIMED: 'graphs.taskStatusClaimed',
  TASK_COMPLETE: 'graphs.taskStatusComplete',
  TASK_BLOCKED: 'graphs.taskStatusBlocked',
  TASK_REVIEW_REQUIRED: 'graphs.taskStatusReviewRequired',
  TASK_FAILED: 'graphs.taskStatusFailed',
  TASK_TIMED_OUT: 'graphs.taskStatusTimedOut',
  TASK_CANCELLED: 'graphs.taskStatusCancelled',
  TASK_CRASHED: 'graphs.taskStatusCrashed',
  TASK_PENDING_ASSIGNMENT: 'graphs.taskStatusPendingAssignment',
};

/**
 * @deprecated Use {@link TASK_STATUS_LABEL_KEYS} with `t()` for i18n support.
 * Kept as fallback returning the i18n key when runtime translation is unavailable.
 */
export const TASK_STATUS_LABELS: Record<TaskStatus, string> = {
  TASK_PENDING: 'graphs.taskStatusPending',
  TASK_CLAIMED: 'graphs.taskStatusClaimed',
  TASK_COMPLETE: 'graphs.taskStatusComplete',
  TASK_BLOCKED: 'graphs.taskStatusBlocked',
  TASK_REVIEW_REQUIRED: 'graphs.taskStatusReviewRequired',
  TASK_FAILED: 'graphs.taskStatusFailed',
  TASK_TIMED_OUT: 'graphs.taskStatusTimedOut',
  TASK_CANCELLED: 'graphs.taskStatusCancelled',
  TASK_CRASHED: 'graphs.taskStatusCrashed',
  TASK_PENDING_ASSIGNMENT: 'graphs.taskStatusPendingAssignment',
};

export const TASK_STATUS_COLORS: Record<TaskStatus, string> = {
  TASK_PENDING: 'grey',
  TASK_CLAIMED: 'blue',
  TASK_COMPLETE: 'green',
  TASK_BLOCKED: 'amber',
  TASK_REVIEW_REQUIRED: 'purple',
  TASK_FAILED: 'red',
  TASK_TIMED_OUT: 'orange',
  TASK_CANCELLED: 'grey-5',
  TASK_CRASHED: 'red-8',
  TASK_PENDING_ASSIGNMENT: 'amber-8',
};
