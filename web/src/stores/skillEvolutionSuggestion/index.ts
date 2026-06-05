import { defineStore } from 'pinia';
import { ref } from 'vue';
import { createSkillEvolutionSuggestionService } from '../../services';
import type { SkillEvolutionSuggestionMsg } from '../../services/kratos/skill_evolution_suggestion/v1/index';

export const useSkillEvolutionSuggestionStore = defineStore('skillEvolutionSuggestion', () => {
  const suggestions = ref<SkillEvolutionSuggestionMsg[]>([]);
  const total = ref(0);
  const loading = ref(false);

  async function loadSuggestions(params: {
    skillId?: string;
    status?: string;
    page?: number;
    pageSize?: number;
  }): Promise<{ items: SkillEvolutionSuggestionMsg[]; total: number }> {
    loading.value = true;
    try {
      const svc = createSkillEvolutionSuggestionService();
      const res = await svc.ListSkillEvolutionSuggestions({
        skillId: params.skillId || undefined,
        status: params.status || undefined,
        page: params.page,
        pageSize: params.pageSize,
      });
      suggestions.value = res.items ?? [];
      total.value = Number(res.total ?? 0);
      return { items: suggestions.value, total: total.value };
    } finally {
      loading.value = false;
    }
  }

  async function approveSuggestion(id: string, approvedBy: string): Promise<void> {
    const svc = createSkillEvolutionSuggestionService();
    await svc.ApproveSkillEvolutionSuggestion({ id, approvedBy });
  }

  async function rejectSuggestion(id: string, rejectedBy: string, rejectionReason?: string): Promise<void> {
    const svc = createSkillEvolutionSuggestionService();
    await svc.RejectSkillEvolutionSuggestion({
      id,
      rejectedBy,
      rejectionReason: rejectionReason || undefined,
    });
  }

  return {
    suggestions,
    total,
    loading,
    loadSuggestions,
    approveSuggestion,
    rejectSuggestion,
  };
});
