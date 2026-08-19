import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import {
  listLearningObservations,
  listLearningPatterns,
  listLearningProposals,
  approveLearningProposal,
  rejectLearningProposal,
  applyLearningProposal,
  updateLearningPatternStatus,
  runLearningLoop,
} from '../../features/agents/api.learning';
import type { LearningObservation, LearningPattern, LearningProposal } from '../../features/agents/learning.types';

export const useLearningLoopStore = defineStore('learningLoop', () => {
  const observations = ref<LearningObservation[]>([]);
  const patterns = ref<LearningPattern[]>([]);
  const proposals = ref<LearningProposal[]>([]);
  const pendingCount = ref(0);
  const loading = computed(() => pendingCount.value > 0);
  const error = ref<string | null>(null);

  async function fetchObservations(agentId: string, since?: string): Promise<LearningObservation[]> {
    pendingCount.value++;
    error.value = null;
    try {
      const result = await listLearningObservations(agentId, since);
      observations.value = result;
      return result;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      pendingCount.value--;
    }
  }

  async function fetchPatterns(agentId: string, status?: string): Promise<LearningPattern[]> {
    pendingCount.value++;
    error.value = null;
    try {
      const result = await listLearningPatterns(agentId, status);
      patterns.value = result;
      return result;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      pendingCount.value--;
    }
  }

  async function fetchProposals(agentId: string, status?: string): Promise<LearningProposal[]> {
    pendingCount.value++;
    error.value = null;
    try {
      const result = await listLearningProposals(agentId, status);
      proposals.value = result;
      return result;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      pendingCount.value--;
    }
  }

  async function approveProposal(agentId: string, proposalId: string): Promise<LearningProposal> {
    error.value = null;
    try {
      const result = await approveLearningProposal(agentId, proposalId);
      const idx = proposals.value.findIndex((p) => p.id === proposalId);
      if (idx !== -1) proposals.value[idx] = result;
      return result;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  async function rejectProposal(agentId: string, proposalId: string): Promise<LearningProposal> {
    error.value = null;
    try {
      const result = await rejectLearningProposal(agentId, proposalId);
      const idx = proposals.value.findIndex((p) => p.id === proposalId);
      if (idx !== -1) proposals.value[idx] = result;
      return result;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  async function applyProposal(agentId: string, proposalId: string): Promise<LearningProposal> {
    error.value = null;
    try {
      const result = await applyLearningProposal(agentId, proposalId);
      const idx = proposals.value.findIndex((p) => p.id === proposalId);
      if (idx !== -1) proposals.value[idx] = result;
      return result;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  async function updatePatternStatus(
    agentId: string,
    patternId: string,
    status: 'confirmed' | 'dismissed',
  ): Promise<LearningPattern> {
    error.value = null;
    try {
      const result = await updateLearningPatternStatus(agentId, patternId, status);
      const idx = patterns.value.findIndex((p) => p.id === patternId);
      if (idx !== -1) patterns.value[idx] = result;
      return result;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  async function runLoop(agentId: string): Promise<void> {
    error.value = null;
    try {
      return await runLearningLoop(agentId);
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  return {
    observations,
    patterns,
    proposals,
    loading,
    error,
    fetchObservations,
    fetchPatterns,
    fetchProposals,
    approveProposal,
    rejectProposal,
    applyProposal,
    updatePatternStatus,
    runLoop,
  };
});
