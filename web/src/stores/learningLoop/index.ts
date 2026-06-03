import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listLearningObservations,
  listLearningPatterns,
  listLearningProposals,
  approveLearningProposal,
  rejectLearningProposal,
  runLearningLoop,
} from '../../features/agents/api.learning';
import type { LearningObservation, LearningPattern, LearningProposal } from '../../features/agents/learning.types';

export const useLearningLoopStore = defineStore('learningLoop', () => {
  const observations = ref<LearningObservation[]>([]);
  const patterns = ref<LearningPattern[]>([]);
  const proposals = ref<LearningProposal[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);

  async function fetchObservations(agentId: string, since?: string): Promise<LearningObservation[]> {
    loading.value = true;
    error.value = null;
    try {
      const result = await listLearningObservations(agentId, since);
      observations.value = result;
      return result;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      loading.value = false;
    }
  }

  async function fetchPatterns(agentId: string, status?: string): Promise<LearningPattern[]> {
    loading.value = true;
    error.value = null;
    try {
      const result = await listLearningPatterns(agentId, status);
      patterns.value = result;
      return result;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      loading.value = false;
    }
  }

  async function fetchProposals(agentId: string, status?: string): Promise<LearningProposal[]> {
    loading.value = true;
    error.value = null;
    try {
      const result = await listLearningProposals(agentId, status);
      proposals.value = result;
      return result;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      loading.value = false;
    }
  }

  async function approveProposal(agentId: string, proposalId: string): Promise<LearningProposal> {
    const result = await approveLearningProposal(agentId, proposalId);
    const idx = proposals.value.findIndex((p) => p.id === proposalId);
    if (idx !== -1) proposals.value[idx] = result;
    return result;
  }

  async function rejectProposal(agentId: string, proposalId: string): Promise<LearningProposal> {
    const result = await rejectLearningProposal(agentId, proposalId);
    const idx = proposals.value.findIndex((p) => p.id === proposalId);
    if (idx !== -1) proposals.value[idx] = result;
    return result;
  }

  async function runLoop(agentId: string): Promise<void> {
    return runLearningLoop(agentId);
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
    runLoop,
  };
});
