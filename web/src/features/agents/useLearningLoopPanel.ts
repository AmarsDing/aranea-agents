import { computed, ref, watch } from "vue";
import { useQuasar } from "quasar";
import { useLearningLoopStore } from "../../stores/learningLoop";
import type { LearningObservation, LearningPattern, LearningProposal } from "./api.learning";

export function useLearningLoopPanel(agentId: () => string) {
  const $q = useQuasar();
  const store = useLearningLoopStore();
  const loading = ref(false);
  const runningLoop = ref(false);
  const approvingId = ref<string | null>(null);
  const rejectingId = ref<string | null>(null);
  const patternStatusFilter = ref<string>("");

  const observations = computed<LearningObservation[]>(() => store.observations);
  const patterns = computed<LearningPattern[]>(() => store.patterns);
  const proposals = computed<LearningProposal[]>(() => store.proposals);

  const pendingProposalsCount = computed(() =>
    proposals.value.filter((p) => p.status === "pending").length
  );
  const registeredKnowledgeCount = computed(() =>
    proposals.value.filter((p) => p.status === "approved").length
  );

  async function fetchAll() {
    const id = agentId();
    if (!id) return;
    loading.value = true;
    try {
      await Promise.all([
        store.fetchObservations(id),
        store.fetchPatterns(id, patternStatusFilter.value || undefined),
        store.fetchProposals(id)
      ]);
    } finally {
      loading.value = false;
    }
  }

  async function onApprove(proposalId: string) {
    const id = agentId();
    if (!id) return;
    $q.dialog({
      title: "审批知识提议",
      message: "确定审批此知识提议？审批后将注册到 Agent 知识库。",
      cancel: true,
      persistent: true
    }).onOk(async () => {
      approvingId.value = proposalId;
      try {
        await store.approveProposal(id, proposalId);
        await fetchAll();
      } finally {
        approvingId.value = null;
      }
    });
  }

  async function onReject(proposalId: string) {
    const id = agentId();
    if (!id) return;
    rejectingId.value = proposalId;
    try {
      await store.rejectProposal(id, proposalId);
      await fetchAll();
    } finally {
      rejectingId.value = null;
    }
  }

  async function onRunLoop() {
    const id = agentId();
    if (!id) return;
    runningLoop.value = true;
    try {
      await store.runLoop(id);
      await fetchAll();
    } finally {
      runningLoop.value = false;
    }
  }

  watch(
    () => [agentId(), patternStatusFilter.value],
    () => {
      void fetchAll();
    },
    { immediate: true }
  );

  return {
    loading,
    runningLoop,
    approvingId,
    rejectingId,
    patternStatusFilter,
    observations,
    patterns,
    proposals,
    pendingProposalsCount,
    registeredKnowledgeCount,
    onApprove,
    onReject,
    onRunLoop,
    fetchAll
  };
}
