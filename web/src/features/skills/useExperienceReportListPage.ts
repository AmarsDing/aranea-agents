import { computed, onMounted, ref, watch } from 'vue';
import { useSkillIntelligenceStore } from '../../stores/skillIntelligence';
import type { ExperienceReport } from '../../services/kratos/skill_intelligence/v1/index';

export function useExperienceReportListPage(skillIdFromQuery?: string) {
  const store = useSkillIntelligenceStore();

  const skillId = ref(skillIdFromQuery ?? '');
  const from = ref('');
  const to = ref('');
  const page = ref(1);
  const pageSize = ref(20);
  const rows = ref<ExperienceReport[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const error = ref('');

  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

  async function loadRows() {
    loading.value = true;
    error.value = '';
    try {
      const data = await store.loadExperienceReports({
        skillId: skillId.value || undefined,
        startTime: from.value || undefined,
        endTime: to.value || undefined,
        page: page.value,
        pageSize: pageSize.value,
      });
      rows.value = data.items;
      total.value = data.total;
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载经验报告失败';
    } finally {
      loading.value = false;
    }
  }

  function resetFilters() {
    skillId.value = skillIdFromQuery ?? '';
    from.value = '';
    to.value = '';
    page.value = 1;
    void loadRows();
  }

  watch([skillId, from, to], () => {
    if (page.value === 1) {
      void loadRows();
    } else {
      page.value = 1;
    }
  });
  watch([page, pageSize], () => {
    void loadRows();
  });

  onMounted(loadRows);

  return {
    skillId,
    from,
    to,
    page,
    pageSize,
    rows,
    total,
    loading,
    error,
    pageMax,
    loadRows,
    resetFilters,
  };
}
