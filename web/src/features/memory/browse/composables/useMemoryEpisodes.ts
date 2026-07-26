import { computed, ref, watch, type Ref } from 'vue';
import type { MemoryEpisode } from '../../types';
import { getMemoryEpisodes } from '../../api';

/** L2 情景时间线数据加载：agent 变化时重置重取，支持「加载更多」分页追加。 */
export function useMemoryEpisodes(agentId: Ref<string | null>, sessionId: Ref<string | null>, pageSize = 20) {
  const items = ref<MemoryEpisode[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const loadingMore = ref(false);
  const error = ref('');

  const hasMore = computed(() => items.value.length < total.value);

  async function load() {
    const id = agentId.value;
    if (!id) {
      items.value = [];
      total.value = 0;
      return;
    }
    loading.value = true;
    error.value = '';
    try {
      const result = await getMemoryEpisodes(id, sessionId.value ?? '', pageSize, 0);
      items.value = result.items;
      total.value = result.total;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      items.value = [];
      total.value = 0;
    } finally {
      loading.value = false;
    }
  }

  async function loadMore() {
    const id = agentId.value;
    if (!id || loadingMore.value || !hasMore.value) {
      return;
    }
    loadingMore.value = true;
    error.value = '';
    try {
      const result = await getMemoryEpisodes(id, sessionId.value ?? '', pageSize, items.value.length);
      items.value = [...items.value, ...result.items];
      total.value = result.total;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      loadingMore.value = false;
    }
  }

  watch([agentId, sessionId], () => void load(), { immediate: true });

  return { items, total, loading, loadingMore, error, hasMore, reload: load, loadMore };
}
