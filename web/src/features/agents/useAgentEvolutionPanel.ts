import { computed, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import type { EvolutionKey } from '../../components/agents/agentUi';
import type { EvolutionMetrics, EvolutionSuggestion } from './types';
import { useAgentDetailStore } from '../../stores/agents/detail';

export function useAgentEvolutionPanel(agentId: () => string, range: () => string) {
  const $q = useQuasar();
  const agentDetailStore = useAgentDetailStore();
  const metricsLoading = ref(false);
  const metrics = ref<EvolutionMetrics | null>(null);
  const suggestions = ref<EvolutionSuggestion[]>([]);
  const applyingId = ref<string | null>(null);
  const rejectingId = ref<string | null>(null);
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
      suggestions.value = await agentDetailStore.fetchEvolutionSuggestions(id, 'pending');
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
      } finally {
        applyingId.value = null;
      }
    });
  }

  async function onReject(id: string) {
    const aid = agentId();
    if (!aid) return;
    rejectingId.value = id;
    try {
      await agentDetailStore.rejectEvolution(aid, id);
      await fetchSuggestions();
    } finally {
      rejectingId.value = null;
    }
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
    pendingSuggestionsCount,
    onApply,
    onReject,
  };
}
