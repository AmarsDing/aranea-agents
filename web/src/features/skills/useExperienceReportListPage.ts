import { computed, onMounted, ref, watch } from 'vue';
import { useSkillIntelligenceStore } from '../../stores/skillIntelligence';

export function useExperienceReportListPage(skillIdFromQuery?: string) {
  const store = useSkillIntelligenceStore();

  const skillId = ref(skillIdFromQuery ?? '');
  const from = ref('');
  const to = ref('');
  const page = ref(1);
  const pageSize = ref(20);

  // Filter guard: prevent double-load when filter change resets page to 1.
  let skipNextPageWatch = false;

  const pageMax = computed(() => Math.max(1, Math.ceil(store.total / pageSize.value)));

  /** 失败标签分布统计（来自后端聚合，非当前页计算） */
  const failureTagsDistribution = computed(() => {
    const map: Record<string, number> = {};
    for (const fc of store.failureTagCounts) {
      map[fc.tag] = fc.count;
    }
    return map;
  });

  async function loadRows() {
    // Convert YYYY-MM-DD date strings to RFC 3339 format for google.protobuf.Timestamp.
    const toRFC3339 = (d: string) => (d ? `${d}T00:00:00Z` : undefined);
    await store.loadExperienceReports({
      skillId: skillId.value || undefined,
      startTime: toRFC3339(from.value),
      endTime: to.value ? `${to.value}T23:59:59Z` : undefined,
      page: page.value,
      pageSize: pageSize.value,
    });
  }

  function resetFilters() {
    skillId.value = skillIdFromQuery ?? '';
    from.value = '';
    to.value = '';
    page.value = 1;
    skipNextPageWatch = true;
    void loadRows();
  }

  watch([skillId, from, to], () => {
    if (page.value === 1) {
      void loadRows();
    } else {
      skipNextPageWatch = true;
      page.value = 1; // triggers page watch, but skipNextPageWatch prevents double load
    }
  });

  watch([page, pageSize], () => {
    if (skipNextPageWatch) {
      skipNextPageWatch = false;
      return;
    }
    void loadRows();
  });

  onMounted(loadRows);

  return {
    skillId,
    from,
    to,
    page,
    pageSize,
    rows: computed(() => store.reports),
    total: computed(() => store.total),
    loading: computed(() => store.loading),
    error: computed(() => store.error),
    pageMax,
    failureTagsDistribution,
    rootCauseReports: computed(() => store.rootCauseReports),
    loadRows,
    resetFilters,
  };
}
