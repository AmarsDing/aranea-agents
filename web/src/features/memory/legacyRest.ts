/**
 * 遗留 Memory / 进化 REST：`legacyRestApi` + `/api/v1/sessions/.../memory/...`、`/agents/.../identity` 等；待 `memory/v1`。
 */
import { legacyRestApi as api } from "../../services/axiosHandler";
import type {
  AgentIdentity,
  AgentStrategyProfile,
  EvolutionEvent,
  EvolutionMetricsReport,
  EvolutionProposal,
  GraphNeighborhood,
  L0AssemblySnapshot,
  L1Field,
  L1Task,
  MemoryEntity,
  MemoryFact,
  MemoryFactListQuery,
  MemoryFactListResult
} from "./types";

export async function listL0Snapshots(sessionID: string, limit = 20): Promise<L0AssemblySnapshot[]> {
  const { data } = await api.get(`/sessions/${sessionID}/l0/snapshots`, { params: { limit } });
  return data.items ?? [];
}

export async function listL1Tasks(
  sessionID: string,
  params: { agent_id?: string; status?: string; include_ended?: boolean } = {}
): Promise<L1Task[]> {
  const { data } = await api.get(`/sessions/${sessionID}/l1/tasks`, { params });
  return data.items ?? [];
}

export async function listL1Fields(sessionID: string, taskID: string, includeInternal = true): Promise<L1Field[]> {
  const { data } = await api.get(`/sessions/${sessionID}/l1/tasks/${taskID}/fields`, {
    params: { include_internal: includeInternal ? "true" : "false" }
  });
  return data.items ?? [];
}

export async function listMemoryFacts(query: MemoryFactListQuery = {}): Promise<MemoryFactListResult> {
  const { data } = await api.get("/memory/l3/facts", { params: query });
  const items = (data.items ?? []) as MemoryFact[];
  return {
    items,
    total: data.total ?? items.length,
    limit: data.limit ?? query.limit ?? items.length,
    offset: data.offset ?? query.offset ?? 0
  };
}

export async function listMemoryEntities(
  query: Record<string, string | number | undefined> = {}
): Promise<{ items: MemoryEntity[]; total: number }> {
  const { data } = await api.get("/memory/l4/entities", { params: query });
  const items = (data.items ?? []) as MemoryEntity[];
  return { items, total: data.total ?? items.length };
}

export async function getMemoryNeighborhood(
  centerID: string,
  params: { hops?: number; max_nodes?: number } = {}
): Promise<GraphNeighborhood> {
  const { data } = await api.get(`/memory/l4/entities/${centerID}/neighborhood`, { params });
  return data as GraphNeighborhood;
}

export async function getAgentIdentity(agentID: string): Promise<AgentIdentity> {
  const { data } = await api.get(`/agents/${agentID}/identity`);
  return data as AgentIdentity;
}

export async function getAgentStrategy(agentID: string): Promise<AgentStrategyProfile> {
  const { data } = await api.get(`/agents/${agentID}/strategy`);
  return data as AgentStrategyProfile;
}

export async function listEvolutionProposals(agentID: string, params: { status?: string; limit?: number } = {}): Promise<EvolutionProposal[]> {
  const { data } = await api.get(`/agents/${agentID}/evolution/proposals`, { params });
  return data.items ?? [];
}

export async function listEvolutionEvents(agentID: string, params: { limit?: number } = {}): Promise<EvolutionEvent[]> {
  const { data } = await api.get(`/agents/${agentID}/evolution/events`, { params });
  return data.items ?? [];
}

export async function getEvolutionMetrics(agentID: string, range = "30d"): Promise<EvolutionMetricsReport> {
  const { data } = await api.get(`/agents/${agentID}/evolution/metrics`, { params: { range } });
  return data as EvolutionMetricsReport;
}
