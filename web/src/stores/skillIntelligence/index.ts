import { defineStore } from 'pinia';
import { ref } from 'vue';
import { listExperienceReports } from '../../features/skills/api';
import { isForbiddenApiError } from '../../utils/kratosError';
import type { ExperienceReportView, FailureTagCountView } from '../../features/skills/types';

export const useSkillIntelligenceStore = defineStore('skillIntelligence', () => {
  const reports = ref<ExperienceReportView[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const error = ref('');
  /** 列表 403：列表页据此渲染 forbidden 态（无重试）。 */
  const forbidden = ref(false);
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
    forbidden.value = false;
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
      forbidden.value = isForbiddenApiError(err);
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
    forbidden,
    failureTagCounts,
    rootCauseReports,
    successCount,
    failureCount,
    avgScore,
    loadExperienceReports,
  };
});
