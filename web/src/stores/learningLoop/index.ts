import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listLearningObservations,
  listLearningPatterns,
  listLearningProposals,
  approveLearningProposal,
  rejectLearningProposal,
  runLearningLoop,
  type LearningObservation,
  type LearningPattern,
  type LearningProposal,
} from '../../features/agents/api.learning';

export const useLearningLoopStore = defineStore('learningLoop', () => {
  const observations = ref<LearningObservation[]>([]);
  const patterns = ref<LearningPattern[]>([]);
  const proposals = ref<LearningProposal[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);

  async function fetchObservations(agentId: string, since?: string): Promise<LearningObservation[]> {
    const result = await listLearningObservations(agentId, since);
    observations.value = result;
    return result;
  }

  async function fetchPatterns(agentId: string, status?: string): Promise<LearningPattern[]> {
    const result = await listLearningPatterns(agentId, status);
    patterns.value = result;
    return result;
  }

  async function fetchProposals(agentId: string, status?: string): Promise<LearningProposal[]> {
    const result = await listLearningProposals(agentId, status);
    proposals.value = result;
    return result;
  }

  async function approveProposal(agentId: string, proposalId: string): Promise<LearningProposal> {
    return approveLearningProposal(agentId, proposalId);
  }

  async function rejectProposal(agentId: string, proposalId: string): Promise<LearningProposal> {
    return rejectLearningProposal(agentId, proposalId);
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
