import { computed, ref, watch, type Ref } from 'vue';
import type { SessionRunRecord } from './types';
import { useSessionStore } from '../../stores/session';

const PAGE_SIZE = 20;

export function useSessionRunsPanel(sessionId: Ref<string>) {
  const sessionStore = useSessionStore();
  const runs = ref<SessionRunRecord[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const error = ref('');
  const offset = ref(0);

  const pageLabel = computed(
    () => `${offset.value + 1}-${Math.min(offset.value + PAGE_SIZE, total.value)} / ${total.value}`,
  );

  async function loadRuns() {
    const id = sessionId.value.trim();
    if (!id) {
      runs.value = [];
      total.value = 0;
      return;
    }
    loading.value = true;
    error.value = '';
    try {
      const result = await sessionStore.fetchRuns(id, PAGE_SIZE, offset.value);
      runs.value = result.items;
      total.value = result.total;
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载 Runs 失败';
    } finally {
      loading.value = false;
    }
  }

  function prevPage() {
    offset.value = Math.max(0, offset.value - PAGE_SIZE);
    void loadRuns();
  }

  function nextPage() {
    offset.value += PAGE_SIZE;
    void loadRuns();
  }

  watch(
    sessionId,
    () => {
      offset.value = 0;
      void loadRuns();
    },
    { immediate: true },
  );

  return { runs, total, loading, error, offset, pageSize: PAGE_SIZE, pageLabel, loadRuns, prevPage, nextPage };
}
