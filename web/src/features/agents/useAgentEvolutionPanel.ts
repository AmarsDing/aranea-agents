import { computed, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import type { EvolutionKey } from '../../components/agents/agentUi';
import type { EvolutionMetrics } from './types';
import { useAgentDetailStore } from '../../stores/agents/detail';
import { useSkillEvolutionStore } from '../../stores/skillEvolution';
import type { SkillEvolutionView } from '../skills/types';

export function useAgentEvolutionPanel(agentId: () => string, range: () => string) {
  const $q = useQuasar();
  const agentDetailStore = useAgentDetailStore();
  const evolutionStore = useSkillEvolutionStore();
  const metricsLoading = ref(false);
  const metrics = ref<EvolutionMetrics | null>(null);
  const applyingId = ref<string | null>(null);
  const rejectingId = ref<string | null>(null);

  const suggestions = computed<SkillEvolutionView[]>(() =>
    evolutionStore.suggestions.filter((s) => s.targetType === 'agent' && s.targetId === agentId()),
  );

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
      await evolutionStore.loadSuggestions({ targetType: 'agent', targetId: id, status: 'pending' });
    } catch {
      // error captured in store
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
        await evolutionStore.approveSuggestion(id, 'agent-panel');
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
      await evolutionStore.rejectSuggestion(id, 'agent-panel', '');
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
