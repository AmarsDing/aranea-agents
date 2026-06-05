import { computed, onMounted, ref, watch } from 'vue';
import { useSkillEvolutionSuggestionStore } from '../../stores/skillEvolutionSuggestion';
import type { SkillEvolutionSuggestionMsg } from '../../services/kratos/skill_evolution_suggestion/v1/index';

export type EvolutionSuggestion = SkillEvolutionSuggestionMsg;

export const statusOptions = [
  { label: '全部', value: '' },
  { label: '待审批', value: 'pending' },
  { label: '已批准', value: 'approved' },
  { label: '已拒绝', value: 'rejected' },
  { label: '已应用', value: 'applied' },
];

export function useEvolutionSuggestionListPage() {
  const store = useSkillEvolutionSuggestionStore();

  const status = ref('');
  const skillId = ref('');
  const page = ref(1);
  const pageSize = ref(20);
  const rows = ref<EvolutionSuggestion[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const error = ref('');

  const rejectDialogOpen = ref(false);
  const rejectTarget = ref<EvolutionSuggestion | null>(null);
  const rejectionReason = ref('');
  const rejecting = ref(false);

  const approvingId = ref('');

  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

  async function loadRows() {
    loading.value = true;
    error.value = '';
    try {
      const data = await store.loadSuggestions({
        skillId: skillId.value?.trim() || undefined,
        status: status.value?.trim() || undefined,
        page: page.value,
        pageSize: pageSize.value,
      });
      rows.value = data.items;
      total.value = data.total;
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载进化建议列表失败';
    } finally {
      loading.value = false;
    }
  }

  async function approveSuggestion(item: EvolutionSuggestion) {
    if (!item.id) return;
    approvingId.value = item.id;
    try {
      await store.approveSuggestion(item.id, 'admin');
      void loadRows();
    } catch (err) {
      error.value = err instanceof Error ? err.message : '审批失败';
    } finally {
      approvingId.value = '';
    }
  }

  function openRejectDialog(item: EvolutionSuggestion) {
    rejectTarget.value = item;
    rejectionReason.value = '';
    rejectDialogOpen.value = true;
  }

  async function confirmReject() {
    if (!rejectTarget.value?.id) return;
    rejecting.value = true;
    try {
      await store.rejectSuggestion(
        rejectTarget.value.id,
        'admin',
        rejectionReason.value?.trim() || undefined,
      );
      rejectDialogOpen.value = false;
      rejectTarget.value = null;
      rejectionReason.value = '';
      void loadRows();
    } catch (err) {
      error.value = err instanceof Error ? err.message : '拒绝失败';
    } finally {
      rejecting.value = false;
    }
  }

  function resetFilters() {
    status.value = '';
    skillId.value = '';
    page.value = 1;
    void loadRows();
  }

  watch([status, skillId], () => {
    page.value = 1;
    void loadRows();
  });
  watch([page, pageSize], () => {
    void loadRows();
  });

  onMounted(() => {
    void loadRows();
  });

  return {
    status,
    skillId,
    page,
    pageSize,
    rows,
    total,
    loading,
    error,
    pageMax,
    approvingId,
    rejectDialogOpen,
    rejectTarget,
    rejectionReason,
    rejecting,
    loadRows,
    approveSuggestion,
    openRejectDialog,
    confirmReject,
    resetFilters,
  };
}
