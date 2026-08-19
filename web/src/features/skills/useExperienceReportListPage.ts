import { computed, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useSkillIntelligenceStore } from '../../stores/skillIntelligence';

export function useExperienceReportListPage(skillIdFromQuery?: string) {
  const store = useSkillIntelligenceStore();
  const route = useRoute();

  const skillId = ref(skillIdFromQuery ?? '');
  const from = ref('');
  const to = ref('');
  const page = ref(1);
  const pageSize = ref(20);

  // 防双加载守卫：resetFilters 同时改筛选值与页码时两个 watch 都会触发，跳过 page watch，由 filter watch 统一加载
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
    const nextSkillId = skillIdFromQuery ?? '';
    const filtersWillChange = skillId.value !== nextSkillId || from.value !== '' || to.value !== '';
    skillId.value = nextSkillId;
    from.value = '';
    to.value = '';
    if (page.value !== 1) {
      // 筛选值与页码同时变化时 filter watch 会统一加载，跳过 page watch 防双加载
      if (filtersWillChange) {
        skipNextPageWatch = true;
      }
      page.value = 1;
    }
  }

  watch([skillId, from, to], () => {
    if (page.value === 1) {
      void loadRows();
    } else {
      // 不可设 skipNextPageWatch：page watch 是唯一会触发的加载入口，跳过将导致筛选后数据不刷新
      page.value = 1;
    }
  });

  // 同路由 query 变化时组件被复用、setup 不重跑：同步外部 skill_id 导航（行级入口/侧栏菜单），
  // 避免筛选残留。skillId 变更由上方 watch 统一触发 reload。
  watch(
    () => route.query.skill_id,
    (val) => {
      const next = typeof val === 'string' ? val : '';
      if (next !== skillId.value) {
        skillId.value = next;
      }
    },
  );

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
