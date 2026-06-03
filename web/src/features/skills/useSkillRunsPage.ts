import { computed, onMounted, ref, watch } from 'vue';
import type { SkillInvocation } from './types';
import { useSkillsStore } from '../../stores/skills';

export function useSkillRunsPage() {
  const skillsStore = useSkillsStore();

  const skillId = ref('');
  const agentId = ref('');
  const status = ref('');
  const from = ref('');
  const to = ref('');
  const page = ref(1);
  const pageSize = ref(20);
  const rows = ref<SkillInvocation[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const error = ref('');

  const statusOptions = [
    { label: '成功', value: 'success' },
    { label: '失败', value: 'failure' },
    { label: '执行中', value: 'pending' },
  ];
  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

  async function loadRows() {
    loading.value = true;
    error.value = '';
    try {
      const data = await skillsStore.loadSkillRuns({
        skill_id: skillId.value,
        agent_id: agentId.value,
        status: status.value,
        from: from.value,
        to: to.value,
        page: page.value,
        page_size: pageSize.value,
      });
      rows.value = data.items;
      total.value = data.total;
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载运行记录失败';
    } finally {
      loading.value = false;
    }
  }

  function resetFilters() {
    skillId.value = '';
    agentId.value = '';
    status.value = '';
    from.value = '';
    to.value = '';
    page.value = 1;
    void loadRows();
  }

  watch([skillId, agentId, status, from, to], () => {
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
    agentId,
    status,
    from,
    to,
    page,
    pageSize,
    rows,
    total,
    loading,
    error,
    statusOptions,
    pageMax,
    loadRows,
    resetFilters,
  };
}
