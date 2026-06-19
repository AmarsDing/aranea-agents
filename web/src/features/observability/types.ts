export interface TaskPlanSummaryData {
  planId: string;
  sessionId: string;
  traceId: string;
  userMessage: string;
  complexityLevel: string;
  complexityScore: number;
  strategy: string;
  status: string;
  subtaskCount: number;
  createdAt: string;
  updatedAt: string;
}

export interface TaskPlanDetailData {
  planId: string;
  sessionId: string;
  traceId: string;
  userMessage: string;
  intentArtifactJson: string;
  complexityLevel: string;
  complexityScore: number;
  dimensions: DimensionScoresData | null;
  subTasks: SubTaskData[];
  taskDag: PlanTaskDagData | null;
  decomposeReason: string;
  strategy: string;
  strategyReason: string;
  topologyHint: string;
  memoryHit: MemoryHitData | null;
  status: string;
  createdAt: string;
  updatedAt: string;
}

export interface DimensionScoresData {
  semantic: number;
  structural: number;
  domain: number;
  tool: number;
  context: number;
  historical: number;
}

export interface SubTaskData {
  id: string;
  name: string;
  description: string;
  dependsOn: string[];
  requiredCapabilities: string[];
  priority: number;
  estimatedComplexity: number;
}

export interface PlanTaskDagData {
  nodes: SubTaskData[];
  rootIds: string[];
  leafIds: string[];
}

export interface MemoryHitData {
  cacheId: string;
  dqScore: number;
  topologyUsed: string;
  agentKeysUsed: string[];
}
