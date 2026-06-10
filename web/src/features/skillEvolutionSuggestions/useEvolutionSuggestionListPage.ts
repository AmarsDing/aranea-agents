/**
 * @deprecated Use `useSkillEvolutionStore` from `stores/skillEvolution` directly instead.
 * This composable wraps the deprecated `useSkillEvolutionSuggestionStore` which only
 * handles skill-level suggestions. The unified store supports both skill and agent types.
 */
import { computed, onMounted, ref, watch } from 'vue';
import { useSkillEvolutionSuggestionStore } from '../../stores/skillEvolutionSuggestion';
import { useAuthStore } from '../../stores/auth';
import type { EvolutionSuggestionView } from '../skills/types';

export type EvolutionSuggestion = EvolutionSuggestionView;

export const statusOptions = [
  { label: '全部', value: '' },
  { label: '待审批', value: 'pending' },
  { label: '已批准', value: 'approved' },
  { label: '已拒绝', value: 'rejected' },
  { label: '已应用', value: 'applied' },
];

export function useEvolutionSuggestionListPage() {
  const store = useSkillEvolutionSuggestionStore();
  const auth = useAuthStore();

  const status = ref('');
  const skillId = ref('');
  const page = ref(1);
  const pageSize = ref(20);

  // Derive data from Store via computed to avoid dual-source refs.
  const rows = computed(() => store.suggestions);
  const total = computed(() => store.total);
  const loading = computed(() => store.loading);
  const error = computed(() => store.error ?? '');

  const rejectDialogOpen = ref(false);
  const rejectTarget = ref<EvolutionSuggestion | null>(null);
  const rejectionReason = ref('');
  const rejecting = ref(false);

  const approvingId = ref('');

  const curatorSkillId = ref('');
  const curatorDialogOpen = ref(false);
  const triggeringCurator = ref(false);

  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

  async function loadRows() {
    try {
      await store.loadSuggestions({
        skillId: skillId.value?.trim() || undefined,
        status: status.value?.trim() || undefined,
        page: page.value,
        pageSize: pageSize.value,
      });
    } catch {
      // error is already captured in store.error, exposed via computed
    }
  }

  async function approveSuggestion(item: EvolutionSuggestion) {
    if (!item.id) return;
    approvingId.value = item.id;
    try {
      await store.approveSuggestion(item.id, auth.displayLabel || 'unknown');
      // Delay refresh to allow backend GenerateDraft + ValidateSuggestion to complete
      setTimeout(() => void loadRows(), 500);
    } catch {
      // error is already captured in store.error
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
        auth.displayLabel || 'unknown',
        rejectionReason.value?.trim() || undefined,
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

  async function triggerCuratorFlow(targetSkillId: string): Promise<EvolutionSuggestionView | null> {
    try {
      return await store.runCuratorFlow(targetSkillId);
    } catch {
      // error is already captured in store.error
      throw store.error;
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
      await triggerCuratorFlow(id);
      curatorDialogOpen.value = false;
      curatorSkillId.value = '';
      void loadRows();
    } catch {
      // error handled in composable
    } finally {
      triggeringCurator.value = false;
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
    curatorSkillId,
    curatorDialogOpen,
    triggeringCurator,
    loadRows,
    approveSuggestion,
    openRejectDialog,
    confirmReject,
    handleTriggerCurator,
    confirmTriggerCurator,
    resetFilters,
    triggerCuratorFlow,
  };
}
