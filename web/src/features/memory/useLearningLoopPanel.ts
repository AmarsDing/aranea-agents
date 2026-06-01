import { computed, ref, watch } from "vue";
import { useMemoryStore } from "../../stores/memory";
import type { KnowledgeProposal, LearningObservation, LearningPattern } from "../memory/types";

export function useLearningLoopPanel(agentId: () => string) {
  const memoryStore = useMemoryStore();

  const actingProposalId = ref<string | null>(null);

  const observations = computed<LearningObservation[]>(() => memoryStore.learningObservations);
  const patterns = computed<LearningPattern[]>(() => memoryStore.learningPatterns);
  const proposals = computed<KnowledgeProposal[]>(() => memoryStore.learningProposals);
  const loading = computed(() => memoryStore.learningLoading);
  const running = computed(() => memoryStore.learningRunning);

  const pendingProposals = computed(() => proposals.value.filter((p) => p.status === "pending"));
  const approvedProposals = computed(() => proposals.value.filter((p) => p.status === "approved"));
  const patternCount = computed(() => patterns.value.length);
  const pendingCount = computed(() => pendingProposals.value.length);
  const knowledgeCount = computed(() => approvedProposals.value.length);

  async function fetchAll() {
    const id = agentId();
    if (!id) return;
    await memoryStore.loadLearningData(id);
  }

  async function onApprove(proposalId: string) {
    const aid = agentId();
    if (!aid) return;
    actingProposalId.value = proposalId;
    try {
      await memoryStore.approveLearning(aid, proposalId);
    } finally {
      actingProposalId.value = null;
    }
  }

  async function onReject(proposalId: string) {
    const aid = agentId();
    if (!aid) return;
    actingProposalId.value = proposalId;
    try {
      await memoryStore.rejectLearning(aid, proposalId);
    } finally {
      actingProposalId.value = null;
    }
  }

  async function onRun() {
    const aid = agentId();
    if (!aid) return;
    await memoryStore.triggerLearningRun(aid);
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
