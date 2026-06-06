import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listEvolutionSuggestions,
  approveEvolutionSuggestion,
  rejectEvolutionSuggestion,
  triggerCuratorFlow,
} from '../../features/skills/api';
import type { EvolutionSuggestionView } from '../../features/skills/types';

export const useSkillEvolutionSuggestionStore = defineStore('skillEvolutionSuggestion', () => {
  const suggestions = ref<EvolutionSuggestionView[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const error = ref<string | null>(null);

  async function loadSuggestions(params: {
    skillId?: string;
    status?: string;
    page?: number;
    pageSize?: number;
  }): Promise<{ items: EvolutionSuggestionView[]; total: number }> {
    loading.value = true;
    error.value = null;
    try {
      const res = await listEvolutionSuggestions(params);
      suggestions.value = res.items;
      total.value = res.total;
      return { items: suggestions.value, total: total.value };
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      error.value = msg;
      throw e;
    } finally {
      loading.value = false;
    }
  }

  async function approveSuggestion(id: string, approvedBy: string): Promise<void> {
    error.value = null;
    try {
      await approveEvolutionSuggestion(id, approvedBy);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      error.value = msg;
      throw e;
    }
  }

  async function rejectSuggestion(id: string, rejectedBy: string, rejectionReason?: string): Promise<void> {
    error.value = null;
    try {
      await rejectEvolutionSuggestion(id, rejectedBy, rejectionReason);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      error.value = msg;
      throw e;
    }
  }

  async function runCuratorFlow(skillId: string): Promise<EvolutionSuggestionView | null> {
    error.value = null;
    try {
      return await triggerCuratorFlow(skillId);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      error.value = msg;
      throw e;
    }
  }

  return {
    suggestions,
    total,
    loading,
    error,
    loadSuggestions,
    approveSuggestion,
    rejectSuggestion,
    runCuratorFlow,
  };
});
