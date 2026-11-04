import { defineStore } from 'pinia';
import { ref } from 'vue';
import { listTaskSituations } from '../../features/session/api';
import type { TaskSituationItem, TaskSituationQuery } from '../../features/session/types';

export const useTaskSituationStore = defineStore('task-situations', () => {
  const items = ref<TaskSituationItem[]>([]);
  const total = ref(0);
  const limit = ref(20);
  const offset = ref(0);
  const loading = ref(false);
  const error = ref('');

  async function load(query: TaskSituationQuery = {}) {
    loading.value = true;
    error.value = '';
    try {
      const page = await listTaskSituations(query);
      items.value = page.items;
      total.value = page.total;
      limit.value = page.limit;
      offset.value = page.offset;
    } catch (cause: unknown) {
      error.value = cause instanceof Error ? cause.message : String(cause);
    } finally {
      loading.value = false;
    }
  }

  function reset() {
    items.value = [];
    total.value = 0;
    limit.value = 20;
    offset.value = 0;
    loading.value = false;
    error.value = '';
  }

  return { items, total, limit, offset, loading, error, load, reset };
});
