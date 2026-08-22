import { computed, ref, type Ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import type { Agent } from '../agents/types';
import type {
  AgentIdentity,
  AgentStrategyProfile,
  CascadeProposal,
  EvolutionEvent,
  EvolutionMetricsReport,
  EvolutionProposal,
  FactReviewAction,
  MemoryFact,
} from './types';
import { useMemoryStore } from '../../stores/memory';

export function useMemoryCenterTrust(opts: {
  selectedAgentId: Ref<string | null>;
  agents: Ref<Agent[]>;
  loadFacts: () => Promise<void>;
}) {
  const { notify } = useQuasar();
  const { t } = useI18n();
  const memoryStore = useMemoryStore();
  const {
    entities,
    cascadePreview,
    cascadeSagaSteps,
    cascadeProposals,
    loadingCascade,
    loadingCascadePreview,
    loadingCascadeSaga,
  } = storeToRefs(memoryStore);

  const evolutionProposals = ref<EvolutionProposal[]>([]);
  const evolutionEvents = ref<EvolutionEvent[]>([]);
  const agentIdentity = ref<AgentIdentity | null>(null);
  const agentStrategy = ref<AgentStrategyProfile | null>(null);
  const evolutionMetrics = ref<EvolutionMetricsReport | null>(null);
  const loadingEvolution = ref(false);
  const cascadeActingId = ref<string | null>(null);
  const conflictFacts = ref<MemoryFact[]>([]);
  const loadingConflicts = ref(false);
  const conflictActingId = ref<string | null>(null);
  const piiFacts = ref<MemoryFact[]>([]);
  const loadingPII = ref(false);
  const piiActingId = ref<string | null>(null);
  const evolutionActingId = ref<string | null>(null);
  const cascadePreviewOpen = ref(false);
  const cascadeSagaDrawerOpen = ref(false);
  const cascadePreviewProposalId = ref<string | null>(null);
  const cascadeSagaProposalId = ref<string | null>(null);

  function resolveAgentID() {
    return opts.selectedAgentId.value || opts.agents.value[0]?.id || '';
  }

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

  async function loadCascade() {
    const agentID = resolveAgentID();
    if (!agentID) {
      return;
    }
    await memoryStore.loadCascadeProposals(agentID);
  }

  async function approveCascade(row: CascadeProposal) {
    cascadeActingId.value = row.id;
    try {
      await memoryStore.approveCascade(row.id);
      await Promise.all([loadCascade(), opts.loadFacts(), loadEvolution()]);
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
      await Promise.all([loadCascade(), opts.loadFacts(), loadEvolution()]);
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
      await Promise.all([loadCascade(), opts.loadFacts(), loadEvolution()]);
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
      await Promise.all([loadCascade(), opts.loadFacts(), loadEvolution()]);
    } finally {
      cascadeActingId.value = null;
    }
  }

  async function loadEvolution() {
    loadingEvolution.value = true;
    try {
      const bundle = await memoryStore.loadEvolutionForAgent(resolveAgentID());
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

  async function loadConflictingFacts() {
    const agentID = resolveAgentID();
    if (!agentID) {
      conflictFacts.value = [];
      return;
    }
    loadingConflicts.value = true;
    try {
      const result = await memoryStore.loadConflictingFacts('', '', agentID, 50, 0);
      conflictFacts.value = result.items;
    } catch {
      conflictFacts.value = [];
    } finally {
      loadingConflicts.value = false;
    }
  }

  async function reviewConflictFact(fact: MemoryFact, action: FactReviewAction) {
    if (conflictActingId.value) return;
    conflictActingId.value = fact.id;
    try {
      await memoryStore.reviewFact({ fact_id: fact.id, action });
      notify({ type: 'positive', message: t(`memory.factDrawer.reviewDone.${action}`) });
      await Promise.all([loadConflictingFacts(), opts.loadFacts()]);
    } catch (e: unknown) {
      notify({ type: 'negative', message: e instanceof Error ? e.message : t('memory.factDrawer.reviewFailed') });
    } finally {
      conflictActingId.value = null;
    }
  }

  async function loadPIIFacts() {
    const agentID = opts.selectedAgentId.value || '';
    if (!agentID) {
      piiFacts.value = [];
      return;
    }
    loadingPII.value = true;
    try {
      piiFacts.value = await memoryStore.loadPIIFlaggedFacts('', '', 50, 0, agentID);
    } catch {
      piiFacts.value = [];
    } finally {
      loadingPII.value = false;
    }
  }

  async function reviewPIIFactRow(fact: MemoryFact, action: 'approve' | 'reject') {
    if (piiActingId.value) return;
    piiActingId.value = fact.id;
    try {
      await memoryStore.reviewPII(fact.id, action);
      notify({ type: 'positive', message: t(`memory.pii.reviewDone.${action}`) });
      await Promise.all([loadPIIFacts(), opts.loadFacts()]);
    } catch (e: unknown) {
      notify({ type: 'negative', message: e instanceof Error ? e.message : t('memory.pii.reviewFailed') });
    } finally {
      piiActingId.value = null;
    }
  }

  async function reviewEvolutionProposal(item: EvolutionProposal, action: 'approve' | 'reject') {
    const agentID = opts.selectedAgentId.value || item.agent_id;
    if (!agentID || evolutionActingId.value) return;
    evolutionActingId.value = item.id;
    try {
      await memoryStore.appendEvolution({
        agentId: agentID,
        workspaceId: '',
        eventKind: action === 'approve' ? 'proposal_approved' : 'proposal_rejected',
        kind: action === 'approve' ? 'proposal_approved' : 'proposal_rejected',
        targetField: item.id,
        reason: action,
        triggerKind: 'user',
        triggerSource: 'memory_center',
        metadataJson: JSON.stringify({ proposal_id: item.id }),
      });
      notify({ type: 'positive', message: t(`memory.evolution.reviewDone.${action}`) });
      await loadEvolution();
    } catch (e: unknown) {
      notify({ type: 'negative', message: e instanceof Error ? e.message : t('memory.evolution.reviewFailed') });
    } finally {
      evolutionActingId.value = null;
    }
  }

  async function revertEvolutionEvent(item: EvolutionEvent) {
    const agentID = opts.selectedAgentId.value || item.agent_id;
    if (!agentID || evolutionActingId.value) return;
    evolutionActingId.value = item.id;
    try {
      await memoryStore.appendEvolution({
        agentId: agentID,
        workspaceId: '',
        eventKind: 'event_reverted',
        kind: 'event_reverted',
        targetField: item.id,
        reason: 'revert',
        triggerKind: 'user',
        triggerSource: 'memory_center',
        metadataJson: JSON.stringify({ event_id: item.id }),
      });
      notify({ type: 'positive', message: t('memory.evolution.revertDone') });
      await loadEvolution();
    } catch (e: unknown) {
      notify({ type: 'negative', message: e instanceof Error ? e.message : t('memory.evolution.reviewFailed') });
    } finally {
      evolutionActingId.value = null;
    }
  }

  function formatScore(value?: number) {
    return (Number(value) || 0).toFixed(2);
  }

  return {
    entities,
    evolutionProposals,
    evolutionEvents,
    evolutionMetrics,
    loadingEvolution,
    evolutionPanels,
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
    conflictFacts,
    loadingConflicts,
    conflictActingId,
    piiFacts,
    loadingPII,
    piiActingId,
    evolutionActingId,
    loadCascade,
    approveCascade,
    rejectCascade,
    previewCascade,
    openSagaDrawer,
    retryCascade,
    compensateCascade,
    confirmPreviewCascade,
    loadEvolution,
    loadConflictingFacts,
    reviewConflictFact,
    loadPIIFacts,
    reviewPIIFactRow,
    reviewEvolutionProposal,
    revertEvolutionEvent,
  };
}
