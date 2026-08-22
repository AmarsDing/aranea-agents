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
  compositeSearchMemories,
  debugMemoryRecall,
  getMemoryNeighborhood,
  getSpreadingActivation,
  getMemoryPlatformSettings,
  listMemoryDeadLetters,
  updateMemoryPlatformSettings,
  getMemoryWorkerStatus,
  replayMemoryDeadLetter,
  abandonMemoryDeadLetter,
  listL1Fields,
  listConflictingFacts,
  upsertMemoryFact,
  reviewMemoryFact,
  appendEvolutionEvent,
  listPIIFlaggedFacts,
  reviewPIIFact,
  getMemoryLayerOverview,
  getMemoryEpisodes,
  getUnifiedMemoryGraph,
} from '../../features/memory/api';
import {
  MEMORY_SNAPSHOT_LIMIT,
  MEMORY_ENTITY_LIMIT,
  MEMORY_EVOLUTION_LIMIT,
  MEMORY_CASCADE_LIMIT,
} from '../../features/constants/queryLimits';
import type {
  L0AssemblySnapshot,
  L1Field,
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
  MemoryWorkerStatus,
  MemoryDeadLetterEntry,
  FactReviewPayload,
  MemoryEpisodeListResult,
  MemoryLayerOverview,
  UnifiedMemoryGraph,
  UnifiedMemoryGraphQuery,
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

  const workerStatus = ref<MemoryWorkerStatus | null>(null);
  const deadLetters = ref<MemoryDeadLetterEntry[]>([]);
  const loadingWorkerStatus = ref(false);
  const loadingDeadLetters = ref(false);

  async function loadSnapshots(sessionID: string, limit = MEMORY_SNAPSHOT_LIMIT): Promise<L0AssemblySnapshot[]> {
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
    const entityQuery = agentID
      ? { scope_type: 'agent', scope_id: agentID, limit: MEMORY_ENTITY_LIMIT }
      : { limit: MEMORY_ENTITY_LIMIT };
    const [entityResult, identity, strategy, proposals, events, metrics] = await Promise.all([
      listMemoryEntities(entityQuery),
      agentID ? getAgentIdentity(agentID).catch(() => null) : Promise.resolve(null),
      agentID ? getAgentStrategy(agentID).catch(() => null) : Promise.resolve(null),
      agentID
        ? listEvolutionProposals(agentID, { status: 'pending', limit: MEMORY_EVOLUTION_LIMIT }).catch(() => [])
        : Promise.resolve([]),
      agentID ? listEvolutionEvents(agentID, { limit: MEMORY_EVOLUTION_LIMIT }).catch(() => []) : Promise.resolve([]),
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
        listCascadeProposals(agentID, { status: 'pending', limit: MEMORY_CASCADE_LIMIT }).catch(() => []),
        listCascadeProposals(agentID, { status: 'partial', limit: MEMORY_CASCADE_LIMIT }).catch(() => []),
        listCascadeProposals(agentID, { status: 'failed', limit: MEMORY_CASCADE_LIMIT }).catch(() => []),
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

  // F-08 fix: wrap ad-hoc memory API calls in Store actions
  async function searchMemoriesComposite(params: Parameters<typeof compositeSearchMemories>[0]) {
    return compositeSearchMemories(params);
  }

  async function recallDebug(params: Parameters<typeof debugMemoryRecall>[0]) {
    return debugMemoryRecall(params);
  }

  async function fetchNeighborhood(centerID: string, params?: Parameters<typeof getMemoryNeighborhood>[1]) {
    return getMemoryNeighborhood(centerID, params);
  }

  async function fetchSpreadingActivation(centerID: string, params?: Parameters<typeof getSpreadingActivation>[1]) {
    return getSpreadingActivation(centerID, params);
  }

  async function fetchPlatformSettings() {
    return getMemoryPlatformSettings();
  }

  async function fetchDeadLetters(state?: string, limit?: number) {
    return listMemoryDeadLetters(state, limit);
  }

  async function savePlatformSettings(input: Parameters<typeof updateMemoryPlatformSettings>[0]) {
    return updateMemoryPlatformSettings(input);
  }

  async function loadWorkerStatus() {
    loadingWorkerStatus.value = true;
    try {
      workerStatus.value = await getMemoryWorkerStatus();
    } finally {
      loadingWorkerStatus.value = false;
    }
  }

  async function loadDeadLetters(status = 'pending') {
    loadingDeadLetters.value = true;
    try {
      deadLetters.value = await listMemoryDeadLetters(status);
    } finally {
      loadingDeadLetters.value = false;
    }
  }

  async function replayDeadLetter(id: number) {
    await replayMemoryDeadLetter(id);
    await loadDeadLetters();
  }

  async function abandonDeadLetter(id: number) {
    await abandonMemoryDeadLetter(id);
    await loadDeadLetters();
  }

  async function loadL1Fields(sessionID: string, taskID: string, includeInternal = true): Promise<L1Field[]> {
    return listL1Fields(sessionID, taskID, includeInternal);
  }

  async function loadConflictingFacts(
    scopeType: string,
    scopeId: string,
    agentId = '',
    limit = 50,
    offset = 0,
  ): Promise<{ items: MemoryFact[]; total: number }> {
    return listConflictingFacts(scopeType, scopeId, agentId, limit, offset);
  }

  async function upsertFact(fact: Parameters<typeof upsertMemoryFact>[0]): Promise<MemoryFact> {
    return upsertMemoryFact(fact);
  }

  /** 治理动作后同步本地列表中的对应行（若已加载）。 */
  async function reviewFact(input: FactReviewPayload): Promise<MemoryFact> {
    const updated = await reviewMemoryFact(input);
    const idx = facts.value.findIndex((f) => f.id === updated.id);
    if (idx >= 0) {
      facts.value[idx] = updated;
    }
    return updated;
  }

  async function appendEvolution(req: Parameters<typeof appendEvolutionEvent>[0]): Promise<EvolutionEvent> {
    return appendEvolutionEvent(req);
  }

  async function loadPIIFlaggedFacts(
    scopeType: string,
    scopeId: string,
    limit = 50,
    offset = 0,
    agentId = '',
  ): Promise<MemoryFact[]> {
    return listPIIFlaggedFacts(scopeType, scopeId, limit, offset, agentId);
  }

  async function reviewPII(factID: string, action: 'approve' | 'reject'): Promise<MemoryFact> {
    return reviewPIIFact(factID, action);
  }

  async function loadLayerOverview(agentID: string, sessionID = ''): Promise<MemoryLayerOverview> {
    return getMemoryLayerOverview(agentID, sessionID);
  }

  async function listEpisodes(
    agentID: string,
    sessionID = '',
    limit = 20,
    offset = 0,
  ): Promise<MemoryEpisodeListResult> {
    return getMemoryEpisodes(agentID, sessionID, limit, offset);
  }

  async function loadUnifiedGraph(agentID: string, query: UnifiedMemoryGraphQuery = {}): Promise<UnifiedMemoryGraph> {
    return getUnifiedMemoryGraph(agentID, query);
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
    searchMemoriesComposite,
    recallDebug,
    fetchNeighborhood,
    fetchSpreadingActivation,
    fetchPlatformSettings,
    fetchDeadLetters,
    savePlatformSettings,
    workerStatus,
    deadLetters,
    loadingWorkerStatus,
    loadingDeadLetters,
    loadWorkerStatus,
    loadDeadLetters,
    replayDeadLetter,
    abandonDeadLetter,
    loadL1Fields,
    loadConflictingFacts,
    upsertFact,
    reviewFact,
    appendEvolution,
    loadPIIFlaggedFacts,
    reviewPII,
    loadLayerOverview,
    listEpisodes,
    loadUnifiedGraph,
  };
});
