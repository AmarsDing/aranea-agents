import { defineStore } from "pinia";
import { ref } from "vue";
import {
  listL0Snapshots,
  listMemoryFacts,
  listMemoryEntities,
  listL1Tasks,
  getAgentIdentity,
  getAgentStrategy,
  listEvolutionProposals,
  listEvolutionEvents,
  getEvolutionMetrics
} from "../../features/memory/api";
import type {
  L0AssemblySnapshot,
  L1Task,
  MemoryEntity,
  MemoryFact,
  MemoryFactListQuery,
  MemoryFactListResult,
  AgentIdentity,
  AgentStrategyProfile,
  EvolutionProposal,
  EvolutionEvent,
  EvolutionMetricsReport
} from "../../features/memory/types";

export type MemoryEvolutionBundle = {
  entities: MemoryEntity[];
  identity: AgentIdentity | null;
  strategy: AgentStrategyProfile | null;
  proposals: EvolutionProposal[];
  events: EvolutionEvent[];
  metrics: EvolutionMetricsReport | null;
};

export const useMemoryStore = defineStore("memory", () => {
  const snapshots = ref<L0AssemblySnapshot[]>([]);
  const facts = ref<MemoryFact[]>([]);
  const entities = ref<MemoryEntity[]>([]);
  const loading = ref(false);

  async function loadSnapshots(sessionID: string, limit = 20): Promise<L0AssemblySnapshot[]> {
    loading.value = true;
    try {
      snapshots.value = await listL0Snapshots(sessionID, limit);
      return snapshots.value;
    } finally {
      loading.value = false;
    }
  }

  async function loadFacts(query?: MemoryFactListQuery): Promise<MemoryFactListResult> {
    const result = await listMemoryFacts(query);
    facts.value = result.items ?? [];
    return result;
  }

  async function loadEntities(query?: Parameters<typeof listMemoryEntities>[0]) {
    const result = await listMemoryEntities(query);
    entities.value = result.items ?? [];
    return result;
  }

  async function loadL1Tasks(sessionID: string, opts?: { include_ended?: boolean }): Promise<L1Task[]> {
    return listL1Tasks(sessionID, opts);
  }

  async function loadEvolutionForAgent(agentID: string): Promise<MemoryEvolutionBundle> {
    const entityQuery = agentID ? { scope_type: "agent", scope_id: agentID, limit: 50 } : { limit: 50 };
    const [entityResult, identity, strategy, proposals, events, metrics] = await Promise.all([
      listMemoryEntities(entityQuery),
      agentID ? getAgentIdentity(agentID).catch(() => null) : Promise.resolve(null),
      agentID ? getAgentStrategy(agentID).catch(() => null) : Promise.resolve(null),
      agentID ? listEvolutionProposals(agentID, { status: "pending", limit: 20 }).catch(() => []) : Promise.resolve([]),
      agentID ? listEvolutionEvents(agentID, { limit: 20 }).catch(() => []) : Promise.resolve([]),
      agentID ? getEvolutionMetrics(agentID).catch(() => null) : Promise.resolve(null)
    ]);
    entities.value = entityResult.items;
    return {
      entities: entityResult.items,
      identity,
      strategy,
      proposals,
      events,
      metrics
    };
  }

  return {
    snapshots,
    facts,
    entities,
    loading,
    loadSnapshots,
    loadFacts,
    loadEntities,
    loadL1Tasks,
    loadEvolutionForAgent
  };
});
