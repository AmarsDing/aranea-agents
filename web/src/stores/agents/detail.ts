import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  getAgent,
  getAgentPromptPreview,
  updateAgent,
  toggleAgentFavorite as toggleAgentFavoriteApi,
  getAgentEvolutionMetrics,
  getAgentEvolutionSuggestions,
  applyEvolutionSuggestion,
  rejectEvolutionSuggestion,
  editPromptFileByAI,
  estimateAgentTokens,
  updateAgentToolPolicy,
  createAgentPromptFile,
  updateAgentPromptFile,
  deleteAgentPromptFile,
  type Agent,
  type EvolutionMetrics,
  type EvolutionSuggestion,
} from '../../features/agents/api';
import type { AgentPromptPreview, AgentPromptFile } from '../../features/agents/types';

/** Agent 详情 / 设置页：HTTP 仅在此 Store actions（aranea-frontend-guide SKILL §3.1）。 */
export const useAgentDetailStore = defineStore('agentDetail', () => {
  const loading = ref(false);
  const saving = ref(false);
  const previewLoading = ref(false);

  async function fetchById(id: string): Promise<Agent> {
    loading.value = true;
    try {
      return await getAgent(id);
    } finally {
      loading.value = false;
    }
  }

  async function patch(id: string, payload: Partial<Agent>): Promise<Agent> {
    saving.value = true;
    try {
      return await updateAgent(id, payload);
    } finally {
      saving.value = false;
    }
  }

  async function fetchPromptPreview(id: string, mode?: string): Promise<AgentPromptPreview> {
    previewLoading.value = true;
    try {
      return await getAgentPromptPreview(id, mode);
    } finally {
      previewLoading.value = false;
    }
  }

  async function fetchEvolutionMetrics(id: string, timeRange?: string): Promise<EvolutionMetrics> {
    return getAgentEvolutionMetrics(id, timeRange);
  }

  async function fetchEvolutionSuggestions(id: string, status?: string): Promise<EvolutionSuggestion[]> {
    return getAgentEvolutionSuggestions(id, status);
  }

  async function applyEvolution(agentId: string, suggestionId: string): Promise<EvolutionSuggestion> {
    return applyEvolutionSuggestion(agentId, suggestionId);
  }

  async function rejectEvolution(agentId: string, suggestionId: string): Promise<EvolutionSuggestion> {
    return rejectEvolutionSuggestion(agentId, suggestionId);
  }

  async function toggleFavorite(id: string): Promise<Agent> {
    return toggleAgentFavoriteApi(id);
  }

  async function editPromptFile(agentId: string, fileId: string, instruction: string): Promise<AgentPromptFile> {
    return editPromptFileByAI(agentId, fileId, instruction);
  }

  async function estimateTokens(agentId: string) {
    return estimateAgentTokens(agentId);
  }

  async function updateToolPolicy(agentId: string, payload: { tools_enabled?: boolean; profile?: string; allow?: string[]; deny?: string[] }): Promise<void> {
    return updateAgentToolPolicy(agentId, payload);
  }

  async function createPromptFile(agentId: string, payload: { name: string; body: string; sort_order: number }): Promise<AgentPromptFile> {
    return createAgentPromptFile(agentId, payload);
  }

  async function updatePromptFile(agentId: string, fileId: string, payload: { name?: string; body?: string; sort_order?: number }): Promise<AgentPromptFile> {
    return updateAgentPromptFile(agentId, fileId, payload);
  }

  async function deletePromptFile(agentId: string, fileId: string): Promise<void> {
    return deleteAgentPromptFile(agentId, fileId);
  }

  return {
    loading,
    saving,
    previewLoading,
    fetchById,
    patch,
    toggleFavorite,
    fetchPromptPreview,
    fetchEvolutionMetrics,
    fetchEvolutionSuggestions,
    applyEvolution,
    rejectEvolution,
    editPromptFile,
    estimateTokens,
    updateToolPolicy,
    createPromptFile,
    updatePromptFile,
    deletePromptFile,
  };
});
