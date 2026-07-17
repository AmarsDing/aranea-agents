import { computed, onMounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import type { Agent } from '../agents/types';
import type { Session } from '../session/types';
import type {
  AgentIdentity,
  AgentStrategyProfile,
  CascadeProposal,
  EvolutionEvent,
  EvolutionMetricsReport,
  EvolutionProposal,
  L0AssemblySnapshot,
  L1Task,
  MemoryFact,
} from './types';
import { buildMemoryAssemblyTableColumns, buildMemoryFactTableColumns } from './memoryTableUi';
import { useAgentsCatalogStore } from '../../stores/agents/catalog';
import { useSessionStore } from '../../stores/session';
import { useMemoryStore } from '../../stores/memory';

export function useMemoryCenterPage() {
  const { notify } = useQuasar();
  const { t } = useI18n();
  const agentsCatalog = useAgentsCatalogStore();
  const sessionStore = useSessionStore();
  const memoryStore = useMemoryStore();
  const {
    facts: storeFacts,
    snapshots: storeSnapshots,
    entities: storeEntities,
    cascadePreview,
    cascadeSagaSteps,
    cascadeProposals: storeCascadeProposals,
    loadingCascade: storeLoadingCascade,
    loadingCascadePreview,
    loadingCascadeSaga,
  } = storeToRefs(memoryStore);

  const tab = ref('overview');
  const agents = ref<Agent[]>([]);
  const sessions = ref<Session[]>([]);
  const facts = storeFacts;
  const entities = storeEntities;
  const evolutionProposals = ref<EvolutionProposal[]>([]);
  const cascadeProposals = storeCascadeProposals;
  const evolutionEvents = ref<EvolutionEvent[]>([]);
  const agentIdentity = ref<AgentIdentity | null>(null);
  const agentStrategy = ref<AgentStrategyProfile | null>(null);
  const evolutionMetrics = ref<EvolutionMetricsReport | null>(null);
  const snapshots = storeSnapshots;
  const tasks = ref<L1Task[]>([]);
  const selectedAgentId = ref<string | null>(null);
  const selectedSessionId = ref<string | null>(null);
  const selectedSnapshot = ref<L0AssemblySnapshot | null>(null);
  const selectedFact = ref<MemoryFact | null>(null);
  const factKeyword = ref('');
  const factScope = ref<string | null>(null);
  const factStatus = ref<string | null>('active');
  const loadingAgents = ref(false);
  const loadingSessions = ref(false);
  const loadingFacts = ref(false);
  const loadingSnapshots = ref(false);
  const loadingTasks = ref(false);
  const loadingEvolution = ref(false);
  const loadingCascade = storeLoadingCascade;
  const cascadeActingId = ref<string | null>(null);
  const factsEndpointReady = ref(true);
  const factsTotal = ref(0);
  const snapshotDrawer = ref(false);
  const factDrawer = ref(false);
  const cascadePreviewOpen = ref(false);
  const cascadeSagaDrawerOpen = ref(false);
  const cascadePreviewProposalId = ref<string | null>(null);
  const cascadeSagaProposalId = ref<string | null>(null);
  const error = ref('');
  const workerStatus = computed(() => memoryStore.workerStatus);
  const loadingWorkerStatus = computed(() => memoryStore.loadingWorkerStatus);

  const loading = computed(
    () =>
      loadingAgents.value ||
      loadingSessions.value ||
      loadingFacts.value ||
      loadingSnapshots.value ||
      loadingTasks.value ||
      loadingEvolution.value ||
      loadingCascade.value,
  );
  const agentOptions = computed(() =>
    agents.value.map((agent) => ({ label: agent.display_name || agent.agent_key, value: agent.id })),
  );

  const overviewCards = computed(() => {
    const avgContext = sessions.value.length
      ? sessions.value.reduce((sum, session) => sum + (session.context_used_ratio || 0), 0) / sessions.value.length
      : 0;
    const riskySessions = sessions.value.filter((session) =>
      ['warning', 'critical', 'exceeded'].includes(session.context_status),
    ).length;
    const activeTasks = tasks.value.filter((task) => task.status === 'active' || task.status === 'paused').length;
    return [
      {
        label: t('memory.metrics.contextRisk'),
        value: riskySessions,
        hint: t('memory.metrics.avgUsage', { percent: formatPercent(avgContext) }),
        icon: 'speed',
        color: contextRatioColor(avgContext),
      },
      {
        label: t('memory.metrics.activeTasks'),
        value: activeTasks,
        hint: t('memory.metrics.l1TasksHint'),
        icon: 'assignment',
        color: 'primary',
      },
      {
        label: t('memory.metrics.longTermKnowledge'),
        value: factsTotal.value,
        hint: factsEndpointReady.value ? t('memory.metrics.l3FactsLoaded') : t('memory.metrics.l3FactsUnavailable'),
        icon: 'psychology',
        color: 'deep-purple',
      },
      {
        label: t('memory.metrics.graphEntities'),
        value: entities.value.length,
        hint: t('memory.metrics.l4EntitiesHint'),
        icon: 'device_hub',
        color: 'teal',
      },
      {
        label: t('memory.metrics.promptSnapshots'),
        value: snapshots.value.length,
        hint: t('memory.metrics.l0SnapshotsHint'),
        icon: 'preview',
        color: 'blue-grey',
      },
    ];
  });

  const actionItems = computed(() => [
    {
      title: t('memory.overview.actions.contextNearLimit'),
      caption: t('memory.overview.actions.contextNearLimitCaption'),
      count: sessions.value.filter((s) => ['warning', 'critical', 'exceeded'].includes(s.context_status)).length,
      icon: 'report',
      color: 'warning',
    },
    {
      title: t('memory.overview.actions.knowledgeConflict'),
      caption: t('memory.overview.actions.knowledgeConflictCaption'),
      count: facts.value.reduce((sum, fact) => sum + (fact.conflict_count || 0), 0),
      icon: 'rule',
      color: 'negative',
    },
    {
      title: t('memory.overview.actions.pendingEvolution'),
      caption: t('memory.overview.actions.pendingEvolutionCaption'),
      count: evolutionProposals.value.length,
      icon: 'auto_awesome',
      color: 'info',
    },
    {
      title: t('memory.overview.actions.cascadeRename'),
      caption: t('memory.overview.actions.cascadeRenameCaption'),
      count: cascadeProposals.value.filter((p) => p.status === 'pending').length,
      icon: 'sync_alt',
      color: 'deep-orange',
    },
  ]);

  const memoryLayers = computed(() => [
    {
      key: 'l0',
      title: t('memory.overview.layers.l0Title'),
      caption: t('memory.overview.layers.l0Caption'),
      icon: 'preview',
      color: 'primary',
      status: t('memory.overview.status.connected'),
      statusColor: 'positive',
    },
    {
      key: 'l1',
      title: t('memory.overview.layers.l1Title'),
      caption: t('memory.overview.layers.l1Caption'),
      icon: 'assignment',
      color: 'indigo',
      status: t('memory.overview.status.connected'),
      statusColor: 'positive',
    },
    {
      key: 'l2',
      title: t('memory.overview.layers.l2Title'),
      caption: t('memory.overview.layers.l2Caption'),
      icon: 'timeline',
      color: 'teal',
      status: t('memory.overview.status.connected'),
      statusColor: 'positive',
    },
    {
      key: 'l3',
      title: t('memory.overview.layers.l3Title'),
      caption: t('memory.overview.layers.l3Caption'),
      icon: 'psychology',
      color: 'deep-purple',
      status: factsEndpointReady.value ? t('memory.overview.status.connected') : t('memory.overview.status.unavailable'),
      statusColor: factsEndpointReady.value ? 'positive' : 'warning',
    },
    {
      key: 'l4',
      title: t('memory.overview.layers.l4Title'),
      caption: t('memory.overview.layers.l4Caption'),
      icon: 'auto_awesome',
      color: 'orange',
      status:
        entities.value.length || agentIdentity.value
          ? t('memory.overview.status.connected')
          : t('memory.overview.status.registered'),
      statusColor: 'positive',
    },
  ]);

  const evolutionPanels = computed(() => [
    {
      title: t('memory.evolution.graphTitle'),
      caption: t('memory.evolution.graphCaption'),
      state: t('memory.evolution.entitiesLoaded', { count: entities.value.length }),
      icon: 'device_hub',
      color: 'teal',
      items: entities.value
        .slice(0, 5)
        .map((entity) => `${entity.name} · ${entity.entity_type} · ${entity.scope_type}`),
    },
    {
      title: t('memory.evolution.identityTitle'),
      caption: t('memory.evolution.identityCaption'),
      state: agentIdentity.value
        ? t('memory.evolution.identityState', {
            phase: agentIdentity.value.current_phase || 'active',
            tone: agentIdentity.value.tone || 'tone unset',
          })
        : t('memory.evolution.identityEmpty'),
      icon: 'badge',
      color: 'primary',
      items: agentIdentity.value
        ? [
            agentIdentity.value.persona || t('memory.evolution.personaEmpty'),
            ...(agentIdentity.value.domains || [])
              .slice(0, 4)
              .map((domain) => t('memory.evolution.domainLabel', { domain })),
          ]
        : [],
    },
    {
      title: t('memory.evolution.strategyTitle'),
      caption: t('memory.evolution.strategyCaption'),
      state: agentStrategy.value
        ? t('memory.evolution.strategyState', {
            exploration: formatScore(agentStrategy.value.exploration),
            caution: formatScore(agentStrategy.value.caution),
          })
        : t('memory.evolution.strategyEmpty'),
      icon: 'tune',
      color: 'deep-purple',
      items: agentStrategy.value
        ? [
            t('memory.evolution.concisenessLabel', { value: formatScore(agentStrategy.value.conciseness) }),
            t('memory.evolution.delegationLabel', { value: formatScore(agentStrategy.value.delegation) }),
            t('memory.evolution.blacklistLabel', {
              value: (agentStrategy.value.tool_blacklist || []).join(', ') || t('memory.evolution.blacklistEmpty'),
            }),
          ]
        : [],
    },
    {
      title: t('memory.evolution.proposalsTitle'),
      caption: t('memory.evolution.proposalsCaption'),
      state: t('memory.evolution.proposalsState', {
        pending: evolutionProposals.value.length,
        events: evolutionMetrics.value?.events_total ?? evolutionEvents.value.length,
      }),
      icon: 'rule',
      color: 'orange',
      items: evolutionProposals.value
        .slice(0, 5)
        .map(
          (proposal) =>
            `${proposal.target_field}: ${proposal.rationale || proposal.expected_impact || proposal.status}`,
        ),
    },
  ]);

  const settingChecklist = computed(() => [
    { label: t('memory.checklist.basicSettings'), caption: t('memory.checklist.basicSettingsCaption'), done: true },
    { label: t('memory.checklist.l0Strategy'), caption: t('memory.checklist.l0StrategyCaption'), done: true },
    { label: t('memory.checklist.l1Budget'), caption: t('memory.checklist.l1BudgetCaption'), done: true },
    {
      label: t('memory.checklist.l3Semantic'),
      caption: t('memory.checklist.l3SemanticCaption'),
      done: factsEndpointReady.value,
    },
    { label: t('memory.checklist.workerModel'), caption: t('memory.checklist.workerModelCaption'), done: true },
    {
      label: t('memory.checklist.platformPolicy'),
      caption: t('memory.checklist.platformPolicyCaption'),
      done: true,
    },
    { label: t('memory.checklist.l4Graph'), caption: t('memory.checklist.l4GraphCaption'), done: true },
  ]);

  const scopeOptions = computed(() =>
    ['user', 'agent', 'team', 'workspace', 'global'].map((value) => ({
      label: t(`memory.knowledge.scope.${value}`),
      value,
    })),
  );
  const factStatusOptions = computed(() =>
    ['active', 'archived', 'disputed', 'deprecated', 'deleted'].map((value) => ({
      label: t(`memory.knowledge.status.${value}`),
      value,
    })),
  );

  const factColumns = computed(() => buildMemoryFactTableColumns(formatDate, t));
  const snapshotColumns = computed(() => buildMemoryAssemblyTableColumns(formatDate, t));

  onMounted(loadAll);

  watch(selectedAgentId, async () => {
    selectedSessionId.value = null;
    await Promise.all([loadSessions(), loadEvolution(), loadCascade()]);
  });

  watch(selectedSessionId, () => {
    void loadSessionMemory();
  });

  async function loadAll() {
    error.value = '';
    try {
      await loadAgents();
      await Promise.all([loadSessions(), loadFacts(), loadEvolution(), loadCascade()]);
    } catch (err) {
      error.value = err instanceof Error ? err.message : t('memory.error.loadFailed');
    }
  }

  async function loadAgents() {
    loadingAgents.value = true;
    try {
      agents.value = await agentsCatalog.fetchAgents({ limit: 200 });
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
      if (!selectedSessionId.value && sessions.value.length) {
        selectedSessionId.value = sessions.value[0].id;
      }
    } finally {
      loadingSessions.value = false;
    }
  }

  async function loadCascade() {
    const agentID = selectedAgentId.value || agents.value[0]?.id || '';
    if (!agentID) {
      return;
    }
    await memoryStore.loadCascadeProposals(agentID);
  }

  async function approveCascade(row: CascadeProposal) {
    cascadeActingId.value = row.id;
    try {
      await memoryStore.approveCascade(row.id);
      await Promise.all([loadCascade(), loadFacts(), loadEvolution()]);
    } catch (e: unknown) {
      notify({ type: 'negative', message: e instanceof Error ? e.message : t('memory.error.cascadeApproveFailed') });
    } finally {
      cascadeActingId.value = null;
    }
  }

  async function rejectCascade(row: CascadeProposal) {
    cascadeActingId.value = row.id;
    try {
      await memoryStore.rejectCascade(row.id);
      await loadCascade();
    } catch (e: unknown) {
      notify({ type: 'negative', message: e instanceof Error ? e.message : t('memory.error.cascadeRejectFailed') });
    } finally {
      cascadeActingId.value = null;
    }
  }

  async function previewCascade(row: CascadeProposal) {
    cascadePreviewProposalId.value = row.id;
    cascadePreviewOpen.value = true;
    memoryStore.clearCascadePreview();
    await memoryStore.loadCascadePreview(row.id);
  }

  async function openSagaDrawer(row: CascadeProposal) {
    cascadeSagaProposalId.value = row.id;
    cascadeSagaDrawerOpen.value = true;
    await memoryStore.loadCascadeSagaSteps(row.id);
  }

  async function retryCascade(row: CascadeProposal) {
    cascadeActingId.value = row.id;
    try {
      await memoryStore.retryCascade(row.id);
      await Promise.all([loadCascade(), loadFacts(), loadEvolution()]);
    } catch (e: unknown) {
      notify({ type: 'negative', message: e instanceof Error ? e.message : t('memory.error.cascadeRetryFailed') });
    } finally {
      cascadeActingId.value = null;
    }
  }

  async function compensateCascade(row: CascadeProposal) {
    cascadeActingId.value = row.id;
    try {
      await memoryStore.compensateCascade(row.id);
      await Promise.all([loadCascade(), loadFacts(), loadEvolution()]);
    } catch (e: unknown) {
      notify({ type: 'negative', message: e instanceof Error ? e.message : t('memory.error.cascadeCompensateFailed') });
    } finally {
      cascadeActingId.value = null;
    }
  }

  async function confirmPreviewCascade(proposalId: string) {
    cascadeActingId.value = proposalId;
    try {
      await memoryStore.approveCascade(proposalId);
      cascadePreviewOpen.value = false;
      await Promise.all([loadCascade(), loadFacts(), loadEvolution()]);
    } finally {
      cascadeActingId.value = null;
    }
  }

  async function loadEvolution() {
    loadingEvolution.value = true;
    try {
      const agentID = selectedAgentId.value || agents.value[0]?.id || '';
      const bundle = await memoryStore.loadEvolutionForAgent(agentID);
      agentIdentity.value = bundle.identity;
      agentStrategy.value = bundle.strategy;
      evolutionProposals.value = bundle.proposals;
      evolutionEvents.value = bundle.events;
      evolutionMetrics.value = bundle.metrics;
    } catch {
      memoryStore.clearEntities();
      agentIdentity.value = null;
      agentStrategy.value = null;
      evolutionProposals.value = [];
      evolutionEvents.value = [];
      evolutionMetrics.value = null;
    } finally {
      loadingEvolution.value = false;
    }
  }

  async function loadFacts() {
    loadingFacts.value = true;
    try {
      const result = await memoryStore.loadFacts({
        keyword: factKeyword.value || undefined,
        scope_type: factScope.value || undefined,
        status: factStatus.value || undefined,
        limit: 50,
      });
      factsTotal.value = result.total;
      factsEndpointReady.value = true;
    } catch {
      memoryStore.clearFacts();
      factsTotal.value = 0;
      factsEndpointReady.value = false;
    } finally {
      loadingFacts.value = false;
    }
  }

  async function loadSessionMemory() {
    if (!selectedSessionId.value) {
      memoryStore.clearSnapshots();
      tasks.value = [];
      return;
    }
    await Promise.all([loadSnapshots(), loadTasks()]);
  }

  async function loadSnapshots() {
    if (!selectedSessionId.value) return;
    loadingSnapshots.value = true;
    try {
      await memoryStore.loadSnapshots(selectedSessionId.value, 20);
    } catch {
      memoryStore.clearSnapshots();
    } finally {
      loadingSnapshots.value = false;
    }
  }

  async function loadTasks() {
    if (!selectedSessionId.value) return;
    loadingTasks.value = true;
    try {
      tasks.value = await memoryStore.loadL1Tasks(selectedSessionId.value, { include_ended: true });
    } catch {
      tasks.value = [];
    } finally {
      loadingTasks.value = false;
    }
  }

  function resetFactFilters() {
    factKeyword.value = '';
    factScope.value = null;
    factStatus.value = 'active';
    void loadFacts();
  }

  function openSnapshot(row: L0AssemblySnapshot) {
    selectedSnapshot.value = row;
    snapshotDrawer.value = true;
  }

  function openFact(row: MemoryFact) {
    selectedFact.value = row;
    factDrawer.value = true;
  }

  function contextRatioColor(value?: number) {
    const ratio = Math.max(0, Math.min(1, Number(value) || 0));
    if (ratio >= 0.85) return 'negative';
    if (ratio >= 0.6) return 'warning';
    return 'positive';
  }

  function formatPercent(value?: number) {
    return `${Math.round((Number(value) || 0) * 100)}%`;
  }

  function formatScore(value?: number) {
    return (Number(value) || 0).toFixed(2);
  }

  function formatDate(value?: string) {
    if (!value) return '-';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
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
    selectedAgentId,
    selectedSessionId,
    selectedSnapshot,
    selectedFact,
    factKeyword,
    factScope,
    factStatus,
    snapshotDrawer,
    factDrawer,
    error,
    loading,
    loadingFacts,
    loadingSessions,
    loadingSnapshots,
    loadingTasks,
    agentOptions,
    sessionRows: sessions,
    factRows: facts,
    snapshotRows: snapshots,
    taskRows: tasks,
    overviewCards,
    actionItems,
    memoryLayers,
    evolutionPanels,
    entities,
    loadingEvolution,
    cascadeProposals,
    loadingCascade,
    cascadeActingId,
    cascadePreviewOpen,
    cascadePreviewData: cascadePreview,
    cascadePreviewProposalId,
    loadingCascadePreview,
    cascadeSagaDrawerOpen,
    cascadeSagaProposalId,
    sagaSteps: cascadeSagaSteps,
    loadingCascadeSaga,
    settingChecklist,
    scopeOptions,
    factStatusOptions,
    factColumns,
    snapshotColumns,
    factsEndpointReady,
    loadAll,
    loadSessions,
    loadFacts,
    loadSessionMemory,
    loadCascade,
    approveCascade,
    rejectCascade,
    previewCascade,
    openSagaDrawer,
    retryCascade,
    compensateCascade,
    confirmPreviewCascade,
    resetFactFilters,
    openSnapshot,
    openFact,
    loadEvolution,
    handleDeadLetterReplay,
    handleDeadLetterAbandon,
    workerStatus,
    loadingWorkerStatus,
    loadWorkerStatus,
  };
}
