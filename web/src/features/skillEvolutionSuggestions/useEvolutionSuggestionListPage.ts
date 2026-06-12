import { computed, onMounted, ref, watch } from 'vue';
import { useSkillEvolutionStore } from '../../stores/skillEvolution';
import { useAuthStore } from '../../stores/auth';
import type { SkillEvolutionView, EvolutionTargetType } from '../skills/types';

export const statusOptions = [
  { label: '全部', value: '' },
  { label: '待审批', value: 'pending' },
  { label: '已批准', value: 'approved' },
  { label: '已拒绝', value: 'rejected' },
  { label: '已应用', value: 'applied' },
];

export const targetTypeOptions = [
  { label: '全部', value: '' },
  { label: 'Skill', value: 'skill' },
  { label: 'Agent', value: 'agent' },
];

export function useEvolutionSuggestionListPage() {
  const store = useSkillEvolutionStore();
  const auth = useAuthStore();

  const targetType = ref<EvolutionTargetType | ''>('');
  const status = ref('');
  const targetId = ref('');
  const page = ref(1);
  const pageSize = ref(20);

  const rows = computed(() => store.suggestions);
  const total = computed(() => store.total);
  const loading = computed(() => store.loading);
  const error = computed(() => store.error ?? '');

  const rejectDialogOpen = ref(false);
  const rejectTarget = ref<SkillEvolutionView | null>(null);
  const rejectionReason = ref('');
  const rejecting = ref(false);

  const approvingId = ref('');

  const curatorSkillId = ref('');
  const curatorDialogOpen = ref(false);
  const triggeringCurator = ref(false);

  const detailDialogOpen = ref(false);
  const detailTarget = ref<SkillEvolutionView | null>(null);

  function openDetailDialog(row: SkillEvolutionView) {
    detailTarget.value = row;
    detailDialogOpen.value = true;
  }

  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

  async function loadRows() {
    try {
      await store.loadSuggestions({
        targetType: targetType.value || undefined,
        targetId: targetId.value?.trim() || undefined,
        status: status.value?.trim() || undefined,
        page: page.value,
        pageSize: pageSize.value,
      });
    } catch {
      // error is already captured in store.error
    }
  }

  async function approveSuggestion(item: SkillEvolutionView) {
    if (!item.id) return;
    approvingId.value = item.id;
    try {
      await store.approveSuggestion(item.id, auth.displayLabel || 'unknown');
      setTimeout(() => void loadRows(), 500);
    } catch {
      // error is already captured in store.error
    } finally {
      approvingId.value = '';
    }
  }

  function openRejectDialog(item: SkillEvolutionView) {
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
        auth.displayLabel || 'unknown',
        rejectionReason.value?.trim() || '',
      );
      rejectDialogOpen.value = false;
      rejectTarget.value = null;
      rejectionReason.value = '';
      void loadRows();
    } catch {
      // error is already captured in store.error
    } finally {
      rejecting.value = false;
    }
  }

  function handleTriggerCurator(currentSkillId: string) {
    curatorSkillId.value = currentSkillId || '';
    curatorDialogOpen.value = true;
  }

  async function confirmTriggerCurator() {
    const id = curatorSkillId.value.trim();
    if (!id) return;
    triggeringCurator.value = true;
    try {
      await store.runCuratorFlow(id);
      curatorDialogOpen.value = false;
      curatorSkillId.value = '';
      void loadRows();
    } catch {
      // error handled in store
    } finally {
      triggeringCurator.value = false;
    }
  }

  function resetFilters() {
    targetType.value = '';
    status.value = '';
    targetId.value = '';
    page.value = 1;
    void loadRows();
  }

  watch([targetType, status, targetId], () => {
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
    targetType,
    status,
    targetId,
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
    curatorSkillId,
    curatorDialogOpen,
    triggeringCurator,
    detailDialogOpen,
    detailTarget,
    openDetailDialog,
    loadRows,
    approveSuggestion,
    openRejectDialog,
    confirmReject,
    handleTriggerCurator,
    confirmTriggerCurator,
    resetFilters,
  };
}
