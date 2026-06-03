import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listL0Snapshots,
  listMemoryFacts,
  listMemoryEntities,
  listL1Tasks,
  getAgentIdentity,
  getAgentStrategy,
  listEvolutionProposals,
  listEvolutionEvents,
  getEvolutionMetrics,
  listCascadeProposals,
  approveCascadeProposal,
  rejectCascadeProposal,
  previewCascadeApprove,
  getCascadeSagaSteps,
  retryCascadeApprove,
  compensateCascadeApprove,
} from '../../features/memory/api';
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
  EvolutionMetricsReport,
  CascadePreview,
  CascadeProposal,
  CascadeSagaStep,
} from '../../features/memory/types';

export type MemoryEvolutionBundle = {
  entities: MemoryEntity[];
  identity: AgentIdentity | null;
  strategy: AgentStrategyProfile | null;
  proposals: EvolutionProposal[];
  events: EvolutionEvent[];
  metrics: EvolutionMetricsReport | null;
};

export const useMemoryStore = defineStore('memory', () => {
  const snapshots = ref<L0AssemblySnapshot[]>([]);
  const facts = ref<MemoryFact[]>([]);
  const entities = ref<MemoryEntity[]>([]);
  const loading = ref(false);

  const cascadeProposals = ref<CascadeProposal[]>([]);
  const cascadePreview = ref<CascadePreview | null>(null);
  const cascadeSagaSteps = ref<CascadeSagaStep[]>([]);
  const loadingCascade = ref(false);
  const loadingCascadePreview = ref(false);
  const loadingCascadeSaga = ref(false);

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
    const entityQuery = agentID ? { scope_type: 'agent', scope_id: agentID, limit: 50 } : { limit: 50 };
    const [entityResult, identity, strategy, proposals, events, metrics] = await Promise.all([
      listMemoryEntities(entityQuery),
      agentID ? getAgentIdentity(agentID).catch(() => null) : Promise.resolve(null),
      agentID ? getAgentStrategy(agentID).catch(() => null) : Promise.resolve(null),
      agentID ? listEvolutionProposals(agentID, { status: 'pending', limit: 20 }).catch(() => []) : Promise.resolve([]),
      agentID ? listEvolutionEvents(agentID, { limit: 20 }).catch(() => []) : Promise.resolve([]),
      agentID ? getEvolutionMetrics(agentID).catch(() => null) : Promise.resolve(null),
    ]);
    entities.value = entityResult.items;
    return {
      entities: entityResult.items,
      identity,
      strategy,
      proposals,
      events,
      metrics,
    };
  }

  async function loadCascadeProposals(agentID: string) {
    loadingCascade.value = true;
    try {
      const [pending, partial, failed] = await Promise.all([
        listCascadeProposals(agentID, { status: 'pending', limit: 30 }).catch(() => []),
        listCascadeProposals(agentID, { status: 'partial', limit: 30 }).catch(() => []),
        listCascadeProposals(agentID, { status: 'failed', limit: 30 }).catch(() => []),
      ]);
      cascadeProposals.value = [...pending, ...partial, ...failed];
      return cascadeProposals.value;
    } finally {
      loadingCascade.value = false;
    }
  }

  async function approveCascade(proposalID: string) {
    return approveCascadeProposal(proposalID);
  }

  async function rejectCascade(proposalID: string, reviewer = 'admin', reason = 'rejected from memory center') {
    return rejectCascadeProposal(proposalID, reviewer, reason);
  }

  async function loadCascadePreview(proposalID: string): Promise<CascadePreview> {
    loadingCascadePreview.value = true;
    try {
      cascadePreview.value = await previewCascadeApprove(proposalID);
      return cascadePreview.value;
    } finally {
      loadingCascadePreview.value = false;
    }
  }

  async function loadCascadeSagaSteps(proposalID: string): Promise<CascadeSagaStep[]> {
    loadingCascadeSaga.value = true;
    try {
      cascadeSagaSteps.value = await getCascadeSagaSteps(proposalID);
      return cascadeSagaSteps.value;
    } finally {
      loadingCascadeSaga.value = false;
    }
  }

  async function retryCascade(proposalID: string, reviewer = 'admin') {
    return retryCascadeApprove(proposalID, reviewer);
  }

  async function compensateCascade(proposalID: string, reviewer = 'admin') {
    return compensateCascadeApprove(proposalID, reviewer);
  }

  function clearCascadePreview() {
    cascadePreview.value = null;
  }

  function clearSnapshots() {
    snapshots.value = [];
  }

  function clearFacts() {
    facts.value = [];
  }

  function clearEntities() {
    entities.value = [];
  }

  return {
    snapshots,
    facts,
    entities,
    loading,
    cascadeProposals,
    cascadePreview,
    cascadeSagaSteps,
    loadingCascade,
    loadingCascadePreview,
    loadingCascadeSaga,
    loadSnapshots,
    loadFacts,
    loadEntities,
    loadL1Tasks,
    loadEvolutionForAgent,
    loadCascadeProposals,
    approveCascade,
    rejectCascade,
    loadCascadePreview,
    loadCascadeSagaSteps,
    retryCascade,
    compensateCascade,
    clearCascadePreview,
    clearSnapshots,
    clearFacts,
    clearEntities,
  };
});
