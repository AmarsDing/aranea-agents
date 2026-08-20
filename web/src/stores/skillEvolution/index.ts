import { defineStore } from 'pinia';
import { ref } from 'vue';
import type { SkillEvolutionView, EvolutionTargetType } from '../../features/skills/types';
import {
  listUnifiedEvolutionSuggestions,
  approveUnifiedEvolutionSuggestion,
  rejectUnifiedEvolutionSuggestion,
  registerUnifiedEvolutionSuggestion,
  triggerCuratorFlow,
} from '../../features/skills/api';

export const useSkillEvolutionStore = defineStore('skillEvolution', () => {
  const suggestions = ref<SkillEvolutionView[]>([]);
  const total = ref(0);
  const skillTotal = ref(0);
  const agentTotal = ref(0);
  const loading = ref(false);
  const error = ref<string | null>(null);

  async function loadSuggestions(params: {
    targetType?: EvolutionTargetType;
    targetId?: string;
    status?: string;
    page?: number;
    pageSize?: number;
  }) {
    loading.value = true;
    error.value = null;
    try {
      const res = await listUnifiedEvolutionSuggestions(params);
      suggestions.value = res.items;
      total.value = res.total;
      skillTotal.value = res.skillTotal;
      agentTotal.value = res.agentTotal;
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      // 失败时清空旧数据，避免错误横幅与过期列表/计数并存。
      suggestions.value = [];
      total.value = 0;
      skillTotal.value = 0;
      agentTotal.value = 0;
      error.value = msg;
    } finally {
      loading.value = false;
    }
  }

  async function approveSuggestion(id: string, approvedBy: string) {
    await approveUnifiedEvolutionSuggestion(id, approvedBy);
    const idx = suggestions.value.findIndex((s) => s.id === id);
    if (idx >= 0) {
      suggestions.value[idx] = { ...suggestions.value[idx], status: 'approved' };
    }
  }

  async function rejectSuggestion(id: string, rejectedBy: string, reason: string) {
    await rejectUnifiedEvolutionSuggestion(id, rejectedBy, reason);
    const idx = suggestions.value.findIndex((s) => s.id === id);
    if (idx >= 0) {
      suggestions.value[idx] = { ...suggestions.value[idx], status: 'rejected' };
    }
  }

  async function registerProposal(id: string) {
    await registerUnifiedEvolutionSuggestion(id);
    const idx = suggestions.value.findIndex((s) => s.id === id);
    if (idx >= 0) {
      suggestions.value[idx] = { ...suggestions.value[idx], status: 'applied' };
    }
  }

  async function runCuratorFlow(skillId: string) {
    return triggerCuratorFlow(skillId);
  }

  return {
    suggestions,
    total,
    skillTotal,
    agentTotal,
    loading,
    error,
    loadSuggestions,
    approveSuggestion,
    rejectSuggestion,
    registerProposal,
    runCuratorFlow,
  };
});
