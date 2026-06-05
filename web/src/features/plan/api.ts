import { createPlanService } from '../../services';

export type PlanStatus = 'draft' | 'approved' | 'confirmed' | 'executing' | 'completed' | 'failed';

export type PlanStep = {
  id: string;
  name: string;
  description: string;
  agent_name: string;
  tools: string[];
  depends_on: string[];
};

export type Plan = {
  id: string;
  session_id: string;
  agent_key: string;
  goal: string;
  steps: PlanStep[];
  status: PlanStatus;
  surface_id: string;
  graph_id: string;
  created_at: string;
  updated_at: string;
};

function mapPlan(raw: Record<string, unknown>): Plan {
  return {
    id: String(raw.id ?? ''),
    session_id: String(raw.session_id ?? raw.sessionId ?? ''),
    agent_key: String(raw.agent_key ?? raw.agentKey ?? ''),
    goal: String(raw.goal ?? ''),
    steps: Array.isArray(raw.steps) ? (raw.steps as Record<string, unknown>[]).map(mapStep) : [],
    status: String(raw.status ?? 'draft') as PlanStatus,
    surface_id: String(raw.surface_id ?? raw.surfaceId ?? ''),
    graph_id: String(raw.graph_id ?? raw.graphId ?? ''),
    created_at: String(raw.created_at ?? raw.createdAt ?? ''),
    updated_at: String(raw.updated_at ?? raw.updatedAt ?? ''),
  };
}

function mapStep(raw: Record<string, unknown>): PlanStep {
  return {
    id: String(raw.id ?? ''),
    name: String(raw.name ?? ''),
    description: String(raw.description ?? ''),
    agent_name: String(raw.agent_name ?? raw.agentName ?? ''),
    tools: Array.isArray(raw.tools) ? (raw.tools as string[]) : [],
    depends_on: Array.isArray(raw.depends_on ?? raw.dependsOn) ? ((raw.depends_on ?? raw.dependsOn) as string[]) : [],
  };
}

export async function listPlans(sessionId: string): Promise<Plan[]> {
  const svc = createPlanService();
  const { data } = await svc.listPlans(sessionId);
  const items = Array.isArray(data)
    ? data
    : (((data as Record<string, unknown>)?.items ?? []) as Record<string, unknown>[]);
  return items.map(mapPlan);
}

export async function getPlan(id: string): Promise<Plan> {
  const svc = createPlanService();
  const { data } = await svc.getPlan(id);
  return mapPlan(data as Record<string, unknown>);
}

export async function createPlan(input: {
  sessionId: string;
  agentKey: string;
  goal: string;
  steps?: unknown[];
}): Promise<Plan> {
  const svc = createPlanService();
  const { data } = await svc.createPlan(input);
  return mapPlan(data as Record<string, unknown>);
}

export async function updatePlan(id: string, patch: { status?: string; steps?: unknown[] }): Promise<Plan> {
  const svc = createPlanService();
  const { data } = await svc.updatePlan(id, patch);
  return mapPlan(data as Record<string, unknown>);
}
