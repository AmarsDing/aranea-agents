import { defineStore } from "pinia";
import { ref } from "vue";
import {
  getAgent, getAgentPromptPreview, updateAgent,
  getAgentEvolutionMetrics, getAgentEvolutionSuggestions,
  applyEvolutionSuggestion, rejectEvolutionSuggestion,
  type Agent, type EvolutionMetrics, type EvolutionSuggestion
} from "../../features/agents/api";
import type { AgentPromptPreview } from "../../features/agents/types";

/** Agent 详情 / 设置页：HTTP 仅在此 Store actions（vue-design §0.1）。 */
export const useAgentDetailStore = defineStore("agentDetail", () => {
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

  return {
    loading,
    saving,
    previewLoading,
    fetchById,
    patch,
    fetchPromptPreview,
    fetchEvolutionMetrics,
    fetchEvolutionSuggestions,
    applyEvolution,
    rejectEvolution
  };
});
