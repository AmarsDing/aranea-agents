import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { i18n } from '../../i18n';
import { useSkillEvolutionStore } from '../../stores/skillEvolution';
import { useAuthStore } from '../../stores/auth';
import type { SkillEvolutionView, EvolutionTargetType } from '../skills/types';

export const statusOptions = [
  { label: '全部', value: '' },
  { label: '待审批', value: 'pending' },
  { label: '已批准', value: 'approved' },
  { label: '已拒绝', value: 'rejected' },
  { label: '已应用', value: 'applied' },
  { label: i18n.global.t('evolutionSuggestionsPage.statusRegistered'), value: 'registered' },
  { label: i18n.global.t('evolutionSuggestionsPage.statusExpired'), value: 'expired' },
];

export const targetTypeOptions = [
  { label: 'Skill', value: 'skill' },
  { label: 'Agent', value: 'agent' },
];

export function useEvolutionSuggestionListPage() {
  const store = useSkillEvolutionStore();
  const auth = useAuthStore();

  // Default to a single target type so server pagination totals stay trustworthy.
  // Dual-source "all" merge cannot paginate correctly across two backends.
  const targetType = ref<EvolutionTargetType>('skill');
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
        targetType: targetType.value,
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
      schedulePostApprovalRefresh();
    } catch {
      // error is already captured in store.error
    } finally {
      approvingId.value = '';
    }
  }

  // The applier transitions approved → applied asynchronously; poll a few
  // times so the row settles on its final state without a manual refresh.
  const APPROVE_POLL_DELAYS_MS = [500, 3000, 8000] as const;
  const pollTimers: ReturnType<typeof setTimeout>[] = [];

  function schedulePostApprovalRefresh() {
    for (const delay of APPROVE_POLL_DELAYS_MS) {
      pollTimers.push(setTimeout(() => void loadRows(), delay));
    }
  }

  onBeforeUnmount(() => {
    for (const timer of pollTimers) clearTimeout(timer);
    pollTimers.length = 0;
  });

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
    // 仅在 Skill tab 下预填搜索框内容；Agent tab 的搜索值是 Agent ID，
    // 不能当作 Curator 流水线的 Skill ID。
    curatorSkillId.value = targetType.value === 'skill' ? currentSkillId || '' : '';
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
    targetType.value = 'skill';
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
