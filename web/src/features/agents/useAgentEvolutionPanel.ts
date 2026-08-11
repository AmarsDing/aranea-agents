import { computed, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';

import type { EvolutionMetrics, EvolutionSuggestion } from './types';
import { useAgentDetailStore } from '../../stores/agents/detail';

export function useAgentEvolutionPanel(agentId: () => string, range: () => string) {
  const $q = useQuasar();
  const { t } = useI18n();
  const agentDetailStore = useAgentDetailStore();
  const metricsLoading = ref(false);
  const metrics = ref<EvolutionMetrics | null>(null);
  const suggestions = ref<EvolutionSuggestion[]>([]);
  const applyingId = ref<string | null>(null);
  const rejectingId = ref<string | null>(null);
  const rejectLoading = ref(false);
  const rollbackingId = ref<string | null>(null);

  const rejectDialogOpen = ref(false);
  const rejectReason = ref('');
  const detailDialogOpen = ref(false);
  const detailSuggestion = ref<EvolutionSuggestion | null>(null);

  const pendingSuggestionsCount = computed(() => suggestions.value.filter((s) => s.status === 'pending').length);

  async function fetchMetrics() {
    const id = agentId();
    if (!id) return;
    metricsLoading.value = true;
    try {
      metrics.value = await agentDetailStore.fetchEvolutionMetrics(id, range());
    } catch {
      metrics.value = null;
    } finally {
      metricsLoading.value = false;
    }
  }

  async function fetchSuggestions() {
    const id = agentId();
    if (!id) return;
    try {
      suggestions.value = await agentDetailStore.fetchEvolutionSuggestions(id);
    } catch {
      suggestions.value = [];
    }
  }

  async function onApply(id: string) {
    const aid = agentId();
    if (!aid) return;
    $q.dialog({
      title: '应用进化建议',
      message: '确定应用此进化建议？将修改 Agent 的相关配置。',
      cancel: true,
      persistent: true,
    }).onOk(async () => {
      applyingId.value = id;
      try {
        await agentDetailStore.applyEvolution(aid, id);
        await fetchSuggestions();
        await fetchMetrics();
      } catch (e: unknown) {
        // 后端会拒绝通知类建议（无可应用内容）——把原因告知用户。
        const msg = e instanceof Error ? e.message : String(e);
        $q.notify({ type: 'warning', message: msg || '应用失败' });
      } finally {
        applyingId.value = null;
      }
    });
  }

  function onReject(id: string) {
    if (!agentId()) return;
    rejectingId.value = id;
    rejectReason.value = '';
    rejectDialogOpen.value = true;
  }

  async function confirmReject() {
    const aid = agentId();
    const id = rejectingId.value;
    if (!aid || !id) return;
    rejectLoading.value = true;
    try {
      await agentDetailStore.rejectEvolution(aid, id, rejectReason.value || undefined);
      rejectDialogOpen.value = false;
      await fetchSuggestions();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      $q.notify({ type: 'warning', message: msg || t('agentSettings.evolution.operationFailed') });
    } finally {
      rejectLoading.value = false;
    }
  }

  function onShowDetail(id: string) {
    detailSuggestion.value = suggestions.value.find((s) => s.id === id) ?? null;
    detailDialogOpen.value = true;
  }

  async function onRollback(id: string) {
    const aid = agentId();
    if (!aid) return;
    $q.dialog({
      title: t('agentSettings.evolution.rollbackConfirmTitle'),
      message: t('agentSettings.evolution.rollbackConfirmMessage'),
      cancel: true,
      persistent: true,
    }).onOk(async () => {
      rollbackingId.value = id;
      try {
        await agentDetailStore.rollbackEvolution(aid, id);
        await fetchSuggestions();
        await fetchMetrics();
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : String(e);
        $q.notify({ type: 'warning', message: msg || t('agentSettings.evolution.operationFailed') });
      } finally {
        rollbackingId.value = null;
      }
    });
  }

  watch(
    () => [agentId(), range()],
    () => {
      void fetchMetrics();
      void fetchSuggestions();
    },
    { immediate: true },
  );

  return {
    metricsLoading,
    metrics,
    suggestions,
    applyingId,
    rejectingId,
    rejectLoading,
    rollbackingId,
    rejectDialogOpen,
    rejectReason,
    detailDialogOpen,
    detailSuggestion,
    pendingSuggestionsCount,
    onApply,
    onReject,
    confirmReject,
    onShowDetail,
    onRollback,
  };
}
