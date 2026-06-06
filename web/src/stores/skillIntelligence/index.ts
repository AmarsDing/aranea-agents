import { defineStore } from 'pinia';
import { ref } from 'vue';
import { listExperienceReports } from '../../features/skills/api';
import type { ExperienceReportView, FailureTagCountView } from '../../features/skills/types';

export const useSkillIntelligenceStore = defineStore('skillIntelligence', () => {
  const reports = ref<ExperienceReportView[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const error = ref('');
  const failureTagCounts = ref<FailureTagCountView[]>([]);
  const rootCauseReports = ref<ExperienceReportView[]>([]);

  async function loadExperienceReports(params: {
    skillId?: string;
    startTime?: string;
    endTime?: string;
    page?: number;
    pageSize?: number;
  }): Promise<void> {
    loading.value = true;
    error.value = '';
    try {
      const res = await listExperienceReports(params);
      reports.value = res.items;
      total.value = res.total;
      failureTagCounts.value = res.failureTagCounts;
      rootCauseReports.value = res.rootCauseReports;
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载经验报告失败';
    } finally {
      loading.value = false;
    }
  }

  return {
    reports,
    total,
    loading,
    error,
    failureTagCounts,
    rootCauseReports,
    loadExperienceReports,
  };
});
