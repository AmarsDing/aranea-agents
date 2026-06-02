import { computed, ref, watch } from "vue";
import { useLearningLoopStore } from "../../stores/learningLoop";
import type { LearningProposal as KnowledgeProposal, LearningObservation, LearningPattern } from "../agents/api.learning";

export function useLearningLoopPanel(agentId: () => string) {
  const store = useLearningLoopStore();

  const actingProposalId = ref<string | null>(null);
  const running = ref(false);

  const observations = computed<LearningObservation[]>(() => store.observations);
  const patterns = computed<LearningPattern[]>(() => store.patterns);
  const proposals = computed<KnowledgeProposal[]>(() => store.proposals);
  const loading = computed(() => store.loading);

  const pendingProposals = computed(() => proposals.value.filter((p) => p.status === "pending"));
  const approvedProposals = computed(() => proposals.value.filter((p) => p.status === "approved"));
  const patternCount = computed(() => patterns.value.length);
  const pendingCount = computed(() => pendingProposals.value.length);
  const knowledgeCount = computed(() => approvedProposals.value.length);

  async function fetchAll() {
    const id = agentId();
    if (!id) return;
    await Promise.all([
      store.fetchObservations(id),
      store.fetchPatterns(id),
      store.fetchProposals(id)
    ]);
  }

  async function onApprove(proposalId: string) {
    const aid = agentId();
    if (!aid) return;
    actingProposalId.value = proposalId;
    try {
      await store.approveProposal(aid, proposalId);
      await fetchAll();
    } finally {
      actingProposalId.value = null;
    }
  }

  async function onReject(proposalId: string) {
    const aid = agentId();
    if (!aid) return;
    actingProposalId.value = proposalId;
    try {
      await store.rejectProposal(aid, proposalId);
      await fetchAll();
    } finally {
      actingProposalId.value = null;
    }
  }

  async function onRun() {
    const aid = agentId();
    if (!aid) return;
    running.value = true;
    try {
      await store.runLoop(aid);
      await fetchAll();
    } finally {
      running.value = false;
    }
  }

  watch(
    () => agentId(),
    () => { void fetchAll(); },
    { immediate: true }
  );

  return {
    observations,
    patterns,
    proposals,
    loading,
    running,
    actingProposalId,
    pendingProposals,
    approvedProposals,
    patternCount,
    pendingCount,
    knowledgeCount,
    fetchAll,
    onApprove,
    onReject,
    onRun
  };
}
