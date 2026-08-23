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
  /** 筛选条件下的聚合统计（随 loadExperienceReports 一并返回） */
  const successCount = ref(0);
  const failureCount = ref(0);
  const avgScore = ref(0);

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
      successCount.value = res.successCount;
      failureCount.value = res.failureCount;
      avgScore.value = res.avgScore;
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
    successCount,
    failureCount,
    avgScore,
    loadExperienceReports,
  };
});
