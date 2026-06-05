import { defineStore } from 'pinia';
import { ref } from 'vue';
import { createSkillIntelligenceService } from '../../services';
import type { ExperienceReport } from '../../services/kratos/skill_intelligence/v1/index';

export const useSkillIntelligenceStore = defineStore('skillIntelligence', () => {
  const reports = ref<ExperienceReport[]>([]);
  const total = ref(0);
  const loading = ref(false);

  async function loadExperienceReports(params: {
    skillId?: string;
    startTime?: string;
    endTime?: string;
    page?: number;
    pageSize?: number;
  }): Promise<{ items: ExperienceReport[]; total: number }> {
    loading.value = true;
    try {
      const svc = createSkillIntelligenceService();
      const data = await svc.ListExperienceReports({
        skillId: params.skillId || undefined,
        startTime: params.startTime || undefined,
        endTime: params.endTime || undefined,
        page: params.page,
        pageSize: params.pageSize,
      });
      reports.value = data.items ?? [];
      total.value = data.total ?? 0;
      return { items: reports.value, total: total.value };
    } finally {
      loading.value = false;
    }
  }

  return {
    reports,
    total,
    loading,
    loadExperienceReports,
  };
});
