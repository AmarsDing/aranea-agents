import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRoute } from 'vue-router';
import type { Agent } from '../agents/types';
import type { Session } from '../session/types';
import { useAgentsCatalogStore } from '../../stores/agents/catalog';
import { useAuthStore } from '../../stores/auth';
import { useSessionStore } from '../../stores/session';
import { useMemoryStore } from '../../stores/memory';
import { resolveMemoryCenterTab } from './memoryCenterTabs';
import { parseMemoryCenterDeepLink } from './memoryCenterDeepLink';
import { useMemoryCenterFacts } from './useMemoryCenterFacts';
import { useMemoryCenterSessionMemory } from './useMemoryCenterSessionMemory';
import { useMemoryCenterTrust } from './useMemoryCenterTrust';

export function useMemoryCenterPage() {
  const { t } = useI18n();
  const route = useRoute();
  const agentsCatalog = useAgentsCatalogStore();
  const authStore = useAuthStore();
  const sessionStore = useSessionStore();
  const memoryStore = useMemoryStore();
  const isPlatformAdmin = computed(() => authStore.isPlatformAdmin);

  const tab = ref('panorama');
  const agents = ref<Agent[]>([]);
  const sessions = ref<Session[]>([]);
  const selectedAgentId = ref<string | null>(null);
  const selectedSessionId = ref<string | null>(null);
  const loadingAgents = ref(false);
  const loadingSessions = ref(false);
  const error = ref('');
  const pendingFactId = ref<string | null>(null);
  const pendingSessionId = ref<string | null>(null);
  const pendingAgentKey = ref<string | null>(null);
  const workerStatus = computed(() => memoryStore.workerStatus);
  const loadingWorkerStatus = computed(() => memoryStore.loadingWorkerStatus);

  const factOpts = {
    selectedAgentId,
    pendingFactId,
    onAfterFactsLoad: undefined as (() => Promise<void> | void) | undefined,
  };
  const facts = useMemoryCenterFacts(factOpts);
  const sessionMemory = useMemoryCenterSessionMemory({ selectedSessionId });
  const trust = useMemoryCenterTrust({
    selectedAgentId,
    agents,
    loadFacts: facts.loadFacts,
  });
  factOpts.onAfterFactsLoad = () => trust.loadConflictingFacts();

  function applyRouteDeepLink() {
    const link = parseMemoryCenterDeepLink(route.query as Record<string, unknown>, isPlatformAdmin.value);
    tab.value = link.tab;
    if (link.agentId) selectedAgentId.value = link.agentId;
    if (link.agentKey) pendingAgentKey.value = link.agentKey;
    if (link.sessionId) pendingSessionId.value = link.sessionId;
    if (link.factId) pendingFactId.value = link.factId;
    if (link.keyword) facts.factKeyword.value = link.keyword;
    if (link.clearFactStatus) facts.factStatus.value = null;
  }

  const loading = computed(
    () =>
      loadingAgents.value ||
      loadingSessions.value ||
      facts.loadingFacts.value ||
      sessionMemory.loadingSnapshots.value ||
      sessionMemory.loadingTasks.value ||
      trust.loadingEvolution.value ||
      trust.loadingCascade.value,
  );
  const agentOptions = computed(() =>
    agents.value.map((agent) => ({ label: agent.display_name || agent.agent_key, value: agent.id })),
  );

  const deepLinkLayer = computed(() => {
    const layerQ = typeof route.query.layer === 'string' ? route.query.layer.trim().toUpperCase() : '';
    if (layerQ === 'L0' || layerQ === 'L1' || layerQ === 'L2' || layerQ === 'L3') return layerQ;
    return '';
  });

  onMounted(() => {
    applyRouteDeepLink();
    void loadAll();
  });

  watch(selectedAgentId, async () => {
    if (!pendingSessionId.value) {
      selectedSessionId.value = null;
    }
    try {
      await Promise.all([
        loadSessions(),
        facts.reloadFactsFromFirstPage(),
        trust.loadEvolution(),
        trust.loadCascade(),
        trust.loadPIIFacts(),
      ]);
    } catch (err) {
      error.value = err instanceof Error ? err.message : t('memory.error.loadFailed');
    }
  });

  watch(isPlatformAdmin, (admin) => {
    if (!admin && tab.value === 'ops') {
      tab.value = 'governance';
      return;
    }
    const tabQ = typeof route.query.tab === 'string' ? route.query.tab.trim() : '';
    if (admin && tabQ === 'ops') {
      tab.value = 'ops';
    }
  });

  function selectMemoryTab(target: string) {
    tab.value = resolveMemoryCenterTab(target, isPlatformAdmin.value);
  }

  async function loadAll() {
    error.value = '';
    try {
      const prevAgentId = selectedAgentId.value;
      await loadAgents();
      if (selectedAgentId.value === prevAgentId) {
        await Promise.all([
          loadSessions(),
          facts.reloadFactsFromFirstPage(),
          trust.loadEvolution(),
          trust.loadCascade(),
          trust.loadPIIFacts(),
        ]);
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : t('memory.error.loadFailed');
    }
  }

  async function loadAgents() {
    loadingAgents.value = true;
    try {
      agents.value = await agentsCatalog.fetchAgents({ limit: 200 });
      if (pendingAgentKey.value) {
        const matched = agents.value.find((agent) => agent.agent_key === pendingAgentKey.value);
        if (matched) selectedAgentId.value = matched.id;
        pendingAgentKey.value = null;
      }
      if (!selectedAgentId.value && agents.value.length) {
        selectedAgentId.value = agents.value[0].id;
      }
    } finally {
      loadingAgents.value = false;
    }
  }

  async function loadSessions() {
    loadingSessions.value = true;
    try {
      await sessionStore.loadSessions({ agent_id: selectedAgentId.value || undefined, limit: 30 });
      sessions.value = sessionStore.sessions;
      if (pendingSessionId.value) {
        const found = sessions.value.some((session) => session.id === pendingSessionId.value);
        if (found) selectedSessionId.value = pendingSessionId.value;
        pendingSessionId.value = null;
      }
      if (!selectedSessionId.value && sessions.value.length) {
        selectedSessionId.value = sessions.value[0].id;
      }
    } finally {
      loadingSessions.value = false;
    }
  }

  async function handleDeadLetterReplay(id: number) {
    await memoryStore.replayDeadLetter(id);
  }

  async function handleDeadLetterAbandon(id: number) {
    await memoryStore.abandonDeadLetter(id);
  }

  async function loadWorkerStatus() {
    await memoryStore.loadWorkerStatus();
  }

  return {
    tab,
    isPlatformAdmin,
    selectMemoryTab,
    selectedAgentId,
    selectedSessionId,
    selectedSnapshot: sessionMemory.selectedSnapshot,
    selectedFact: facts.selectedFact,
    factKeyword: facts.factKeyword,
    factScope: facts.factScope,
    factStatus: facts.factStatus,
    snapshotDrawer: sessionMemory.snapshotDrawer,
    factDrawer: facts.factDrawer,
    factEditOpen: facts.factEditOpen,
    factEditMode: facts.factEditMode,
    factReviewActing: facts.factReviewActing,
    conflictFacts: trust.conflictFacts,
    loadingConflicts: trust.loadingConflicts,
    conflictActingId: trust.conflictActingId,
    selectedTaskId: sessionMemory.selectedTaskId,
    fieldRows: sessionMemory.fieldRows,
    loadingFields: sessionMemory.loadingFields,
    piiFacts: trust.piiFacts,
    loadingPII: trust.loadingPII,
    piiActingId: trust.piiActingId,
    evolutionActingId: trust.evolutionActingId,
    evolutionProposals: trust.evolutionProposals,
    evolutionEvents: trust.evolutionEvents,
    loadPIIFacts: trust.loadPIIFacts,
    reviewPIIFactRow: trust.reviewPIIFactRow,
    reviewEvolutionProposal: trust.reviewEvolutionProposal,
    revertEvolutionEvent: trust.revertEvolutionEvent,
    error,
    loading,
    loadingFacts: facts.loadingFacts,
    loadingSessions,
    loadingSnapshots: sessionMemory.loadingSnapshots,
    loadingTasks: sessionMemory.loadingTasks,
    agentOptions,
    sessionRows: sessions,
    factRows: facts.facts,
    snapshotRows: sessionMemory.snapshots,
    taskRows: sessionMemory.tasks,
    deepLinkLayer,
    evolutionPanels: trust.evolutionPanels,
    entities: trust.entities,
    loadingEvolution: trust.loadingEvolution,
    cascadeProposals: trust.cascadeProposals,
    loadingCascade: trust.loadingCascade,
    cascadeActingId: trust.cascadeActingId,
    cascadePreviewOpen: trust.cascadePreviewOpen,
    cascadePreviewData: trust.cascadePreviewData,
    cascadePreviewProposalId: trust.cascadePreviewProposalId,
    loadingCascadePreview: trust.loadingCascadePreview,
    cascadeSagaDrawerOpen: trust.cascadeSagaDrawerOpen,
    cascadeSagaProposalId: trust.cascadeSagaProposalId,
    sagaSteps: trust.sagaSteps,
    loadingCascadeSaga: trust.loadingCascadeSaga,
    scopeOptions: facts.scopeOptions,
    factStatusOptions: facts.factStatusOptions,
    factColumns: facts.factColumns,
    snapshotColumns: sessionMemory.snapshotColumns,
    factsEndpointReady: facts.factsEndpointReady,
    factsActiveCount: facts.factsActiveCount,
    factsArchivedCount: facts.factsArchivedCount,
    factsFilteredCount: facts.factsFilteredCount,
    factPage: facts.factPage,
    factPageSize: facts.factPageSize,
    factPageMax: facts.factPageMax,
    loadAll,
    loadSessions,
    loadFacts: facts.loadFacts,
    reloadFactsFromFirstPage: facts.reloadFactsFromFirstPage,
    loadSessionMemory: sessionMemory.loadSessionMemory,
    loadCascade: trust.loadCascade,
    approveCascade: trust.approveCascade,
    rejectCascade: trust.rejectCascade,
    previewCascade: trust.previewCascade,
    openSagaDrawer: trust.openSagaDrawer,
    retryCascade: trust.retryCascade,
    compensateCascade: trust.compensateCascade,
    confirmPreviewCascade: trust.confirmPreviewCascade,
    resetFactFilters: facts.resetFactFilters,
    openSnapshot: sessionMemory.openSnapshot,
    openFact: facts.openFact,
    reviewSelectedFact: facts.reviewSelectedFact,
    openRefineFact: facts.openRefineFact,
    openCreateFact: facts.openCreateFact,
    submitFactEdit: facts.submitFactEdit,
    reviewConflictFact: trust.reviewConflictFact,
    loadEvolution: trust.loadEvolution,
    handleDeadLetterReplay,
    handleDeadLetterAbandon,
    workerStatus,
    loadingWorkerStatus,
    loadWorkerStatus,
  };
}
