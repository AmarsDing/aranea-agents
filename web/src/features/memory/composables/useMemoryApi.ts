// F-08 fix: route through Store instead of direct API calls.
// S-09 fix: add loading state management for observability.
import { ref } from 'vue';
import { useMemoryStore } from '../../../stores/memory';

export function useMemoryApi() {
  const store = useMemoryStore();
  const loading = ref(false);

  async function compositeSearchMemories(params: Parameters<typeof store.searchMemoriesComposite>[0]) {
    loading.value = true;
    try {
      return await store.searchMemoriesComposite(params);
    } finally {
      loading.value = false;
    }
  }

  async function debugMemoryRecall(params: Parameters<typeof store.recallDebug>[0]) {
    loading.value = true;
    try {
      return await store.recallDebug(params);
    } finally {
      loading.value = false;
    }
  }

  async function getMemoryNeighborhood(centerID: string, params?: Parameters<typeof store.fetchNeighborhood>[1]) {
    loading.value = true;
    try {
      return await store.fetchNeighborhood(centerID, params);
    } finally {
      loading.value = false;
    }
  }

  async function getMemoryPlatformSettings() {
    loading.value = true;
    try {
      return await store.fetchPlatformSettings();
    } finally {
      loading.value = false;
    }
  }

  async function listMemoryDeadLetters(state?: string, limit?: number) {
    loading.value = true;
    try {
      return await store.fetchDeadLetters(state, limit);
    } finally {
      loading.value = false;
    }
  }

  async function updateMemoryPlatformSettings(input: Parameters<typeof store.savePlatformSettings>[0]) {
    loading.value = true;
    try {
      return await store.savePlatformSettings(input);
    } finally {
      loading.value = false;
    }
  }

  return {
    loading,
    compositeSearchMemories,
    debugMemoryRecall,
    getMemoryNeighborhood,
    getMemoryPlatformSettings,
    listMemoryDeadLetters,
    updateMemoryPlatformSettings,
  };
}
