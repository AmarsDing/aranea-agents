import { ref, watch, type Ref } from 'vue';
import type { MemoryLayerOverview } from '../../types';
import { useMemoryStore } from '../../../../stores/memory';

/** 层级全景数据加载：agent/session 变化时自动重取，单次请求返回五层统计 + 行动项 + 动态。 */
export function useLayerOverview(agentId: Ref<string | null>, sessionId: Ref<string | null>) {
  const memoryStore = useMemoryStore();
  const overview = ref<MemoryLayerOverview | null>(null);
  const loading = ref(false);
  const error = ref('');

  async function load() {
    const id = agentId.value;
    if (!id) {
      overview.value = null;
      return;
    }
    loading.value = true;
    error.value = '';
    try {
      overview.value = await memoryStore.loadLayerOverview(id, sessionId.value ?? '');
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      overview.value = null;
    } finally {
      loading.value = false;
    }
  }

  watch([agentId, sessionId], () => void load(), { immediate: true });

  return { overview, loading, error, reload: load };
}
