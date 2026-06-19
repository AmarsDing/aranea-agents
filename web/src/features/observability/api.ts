import { createChatService } from '../../services';
import type {
  TaskPlanSummaryData,
  TaskPlanDetailData,
  DimensionScoresData,
  SubTaskData,
  PlanTaskDagData,
  MemoryHitData,
} from './types';

function mapDimensions(d?: { semantic?: number; structural?: number; domain?: number; tool?: number; context?: number; historical?: number } | null): DimensionScoresData | null {
  if (!d) return null;
  return {
    semantic: d.semantic ?? 0,
    structural: d.structural ?? 0,
    domain: d.domain ?? 0,
    tool: d.tool ?? 0,
    context: d.context ?? 0,
    historical: d.historical ?? 0,
  };
}

function mapSubTask(s: {
  id?: string;
  name?: string;
  description?: string;
  dependsOn?: string[];
  requiredCapabilities?: string[];
  priority?: number;
  estimatedComplexity?: number;
}): SubTaskData {
  return {
    id: s.id ?? '',
    name: s.name ?? '',
    description: s.description ?? '',
    dependsOn: s.dependsOn ?? [],
    requiredCapabilities: s.requiredCapabilities ?? [],
    priority: s.priority ?? 0,
    estimatedComplexity: s.estimatedComplexity ?? 0,
  };
}

function mapDag(dag?: { nodes?: SubTaskData[]; rootIds?: string[]; leafIds?: string[] } | null): PlanTaskDagData | null {
  if (!dag) return null;
  return {
    nodes: (dag.nodes ?? []).map(mapSubTask),
    rootIds: dag.rootIds ?? [],
    leafIds: dag.leafIds ?? [],
  };
}

function mapMemoryHit(m?: { cacheId?: string; dqScore?: number; topologyUsed?: string; agentKeysUsed?: string[] } | null): MemoryHitData | null {
  if (!m) return null;
  return {
    cacheId: m.cacheId ?? '',
    dqScore: m.dqScore ?? 0,
    topologyUsed: m.topologyUsed ?? '',
    agentKeysUsed: m.agentKeysUsed ?? [],
  };
}

export async function listTaskPlans(sessionId: string): Promise<TaskPlanSummaryData[]> {
  const svc = createChatService();
  const res = await svc.ListPlans({ sessionId });
  const plans = res.plans ?? [];
  return plans.map((p) => ({
    planId: p.planId ?? '',
    sessionId: p.sessionId ?? '',
    traceId: p.traceId ?? '',
    userMessage: p.userMessage ?? '',
    complexityLevel: p.complexityLevel ?? '',
    complexityScore: p.complexityScore ?? 0,
    strategy: p.strategy ?? '',
    status: p.status ?? '',
    subtaskCount: p.subtaskCount ?? 0,
    createdAt: p.createdAt ?? '',
    updatedAt: p.updatedAt ?? '',
  }));
}

export async function getTaskPlan(planId: string, sessionId: string): Promise<TaskPlanDetailData | null> {
  const svc = createChatService();
  const res = await svc.GetPlan({ planId, sessionId });
  const p = res.plan;
  if (!p) return null;
  return {
    planId: p.planId ?? '',
    sessionId: p.sessionId ?? '',
    traceId: p.traceId ?? '',
    userMessage: p.userMessage ?? '',
    intentArtifactJson: p.intentArtifactJson ?? '',
    complexityLevel: p.complexityLevel ?? '',
    complexityScore: p.complexityScore ?? 0,
    dimensions: mapDimensions(p.dimensions ?? null),
    subTasks: (p.subTasks ?? []).map(mapSubTask),
    taskDag: mapDag(p.taskDag ?? null),
    decomposeReason: p.decomposeReason ?? '',
    strategy: p.strategy ?? '',
    strategyReason: p.strategyReason ?? '',
    topologyHint: p.topologyHint ?? '',
    memoryHit: mapMemoryHit(p.memoryHit ?? null),
    status: p.status ?? '',
    createdAt: p.createdAt ?? '',
    updatedAt: p.updatedAt ?? '',
  };
}
