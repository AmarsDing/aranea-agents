import { ref, computed, toValue, watch, type MaybeRefOrGetter } from 'vue';
import { useQuasar } from 'quasar';
import { useLearningLoopStore } from '../../stores/learningLoop';
import { i18n } from '../../i18n';

const t = i18n.global.t;

export type PatternFilter = '' | 'detected' | 'confirmed' | 'dismissed';
export type ProposalFilter = '' | 'validated' | 'approved' | 'applied' | 'rejected' | 'conflict';

export function useLearningLoopPanel(agentIdInput: MaybeRefOrGetter<string>) {
  const $q = useQuasar();
  const store = useLearningLoopStore();
  const agentId = () => toValue(agentIdInput);

  const patternStatus = ref<PatternFilter>('');
  const proposalStatus = ref<ProposalFilter>('');
  const runningLoop = ref(false);

  // === Computed filtering (概览独立，列表过滤仅影响展示) ===
  const filteredPatterns = computed(() => {
    if (!patternStatus.value) return store.patterns;
    return store.patterns.filter((p) => p.status === patternStatus.value);
  });

  const filteredProposals = computed(() => {
    if (!proposalStatus.value) return store.proposals;
    return store.proposals.filter((p) => p.status === proposalStatus.value);
  });

  // === Overview counts from full store (not filtered) ===
  const overview = computed(() => ({
    observationCount: store.observations.length,
    patternCount: store.patterns.length,
    pendingCount: store.proposals.filter((p) => p.status === 'validated').length,
    registeredCount: store.proposals.filter((p) => p.status === 'applied').length,
  }));

  // === Fetching ===
  async function fetchAll() {
    const id = agentId();
    if (!id) return;
    await Promise.all([
      store.fetchObservations(id),
      store.fetchPatterns(id),   // full
      store.fetchProposals(id),  // full
    ]);
  }

  // agentId 就绪后自动加载（如 tab 首次进入）。
  watch(agentId, (id) => {
    if (id) void onRefresh();
  }, { immediate: true });

  async function onRefresh() {
    try {
      await fetchAll();
    } catch {
      $q.notify({ type: 'negative', message: t('agents.learning_loop.refresh_failed') });
    }
  }

  async function onRunLoop() {
    runningLoop.value = true;
    try {
      await store.runLoop(agentId());
      await fetchAll();
      $q.notify({ type: 'positive', message: t('agents.learning_loop.run_success') });
    } catch {
      $q.notify({ type: 'negative', message: t('agents.learning_loop.run_failed') });
    } finally {
      runningLoop.value = false;
    }
  }

  // === Pattern actions ===
  async function onConfirmPattern(patternId: string) {
    try {
      await store.updatePatternStatus(agentId(), patternId, 'confirmed');
      $q.notify({ type: 'positive', message: t('agents.learning_loop.pattern_confirmed') });
    } catch {
      $q.notify({ type: 'negative', message: t('agents.learning_loop.pattern_confirm_failed') });
    }
  }

  async function onDismissPattern(patternId: string) {
    try {
      await store.updatePatternStatus(agentId(), patternId, 'dismissed');
      $q.notify({ type: 'positive', message: t('agents.learning_loop.pattern_dismissed') });
    } catch {
      $q.notify({ type: 'negative', message: t('agents.learning_loop.pattern_dismiss_failed') });
    }
  }

  // === Proposal actions ===
  async function onApprove(proposalId: string) {
    $q.dialog({
      title: t('agents.learning_loop.approve_title'),
      message: t('agents.learning_loop.approve_message'),
      persistent: true,
      ok: { label: t('agents.learning_loop.approve'), color: 'positive', flat: true },
      cancel: { label: t('agents.learning_loop.cancel'), color: 'primary', flat: true },
    }).onOk(async () => {
      try {
        await store.approveProposal(agentId(), proposalId);
        await fetchAll();
        $q.notify({ type: 'positive', message: t('agents.learning_loop.approved') });
      } catch {
        $q.notify({ type: 'negative', message: t('agents.learning_loop.approve_failed') });
      }
    });
  }

  async function onReject(proposalId: string) {
    $q.dialog({
      title: t('agents.learning_loop.reject_title'),
      message: t('agents.learning_loop.reject_message'),
      persistent: true,
      ok: { label: t('agents.learning_loop.reject'), color: 'negative', flat: true },
      cancel: { label: t('agents.learning_loop.cancel'), color: 'primary', flat: true },
    }).onOk(async () => {
      try {
        await store.rejectProposal(agentId(), proposalId);
        await fetchAll();
        $q.notify({ type: 'positive', message: t('agents.learning_loop.rejected') });
      } catch {
        $q.notify({ type: 'negative', message: t('agents.learning_loop.reject_failed') });
      }
    });
  }

  async function onApply(proposalId: string) {
    try {
      await store.applyProposal(agentId(), proposalId);
      await fetchAll();
      $q.notify({ type: 'positive', message: t('agents.learning_loop.applied') });
    } catch {
      $q.notify({ type: 'negative', message: t('agents.learning_loop.apply_failed') });
    }
  }

  return {
    // State
    patternStatus,
    proposalStatus,
    runningLoop,
    loading: computed(() => store.loading),
    error: computed(() => store.error),

    // Filtered data
    filteredPatterns,
    filteredProposals,
    observations: computed(() => store.observations),

    // Overview (decoupled from filters)
    overview,

    // Actions
    fetchAll,
    onRefresh,
    onRunLoop,
    onConfirmPattern,
    onDismissPattern,
    onApprove,
    onReject,
    onApply,
  };
}
