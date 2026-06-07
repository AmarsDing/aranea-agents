// FD4+FB3 fix: extract dead-letter data fetching + error handling from
// MemoryDeadLetterPanel.vue into composable so the .vue file only handles template.
import { onMounted, ref } from 'vue';
import type { MemoryDeadLetterEntry } from '../types';
import { useMemoryApi } from './useMemoryApi';

export function useMemoryDeadLetterPanel() {
  const { listMemoryDeadLetters } = useMemoryApi();
  const rows = ref<MemoryDeadLetterEntry[]>([]);
  const loading = ref(false);

  async function load() {
    loading.value = true;
    try {
      rows.value = await listMemoryDeadLetters('pending', 50);
    } catch {
      rows.value = [];
    } finally {
      loading.value = false;
    }
  }

  onMounted(load);

  return { rows, loading, load };
}
