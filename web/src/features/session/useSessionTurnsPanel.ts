import { computed, ref, watch, type Ref } from 'vue';
import type { SessionTurn } from './types';
import { useSessionStore } from '../../stores/session/index';

const PAGE_SIZE = 20;

export function useSessionTurnsPanel(sessionId: Ref<string>) {
  const sessionStore = useSessionStore();
  const turns = ref<SessionTurn[]>([]);
  const total = ref(0);
  const loading = ref(false);
  const error = ref('');
  const offset = ref(0);

  const pageLabel = computed(
    () => `${offset.value + 1}-${Math.min(offset.value + PAGE_SIZE, total.value)} / ${total.value}`,
  );

  async function loadTurns() {
    const id = sessionId.value.trim();
    if (!id) {
      turns.value = [];
      total.value = 0;
      return;
    }
    loading.value = true;
    error.value = '';
    try {
      const result = await sessionStore.fetchTurns(id, PAGE_SIZE, offset.value);
      turns.value = result.items;
      total.value = result.total;
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载 Turn 失败';
    } finally {
      loading.value = false;
    }
  }

  function prevPage() {
    offset.value = Math.max(0, offset.value - PAGE_SIZE);
    void loadTurns();
  }

  function nextPage() {
    offset.value += PAGE_SIZE;
    void loadTurns();
  }

  watch(
    sessionId,
    () => {
      offset.value = 0;
      void loadTurns();
    },
    { immediate: true },
  );

  return {
    turns,
    total,
    loading,
    error,
    offset,
    pageSize: PAGE_SIZE,
    pageLabel,
    loadTurns,
    prevPage,
    nextPage,
  };
}
