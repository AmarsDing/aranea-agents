// F-08 fix: route through Store instead of direct API calls.
// S-09 fix: add loading state management for observability.
import { reactive } from 'vue';
import { useMemoryStore } from '../../../stores/memory';

export function useMemoryApi() {
  const store = useMemoryStore();
  const loading = reactive<Record<string, boolean>>({});

  function setLoading(key: string, value: boolean) {
    loading[key] = value;
  }

  async function compositeSearchMemories(params: Parameters<typeof store.searchMemoriesComposite>[0]) {
    setLoading('compositeSearch', true);
    try {
      return await store.searchMemoriesComposite(params);
    } finally {
      setLoading('compositeSearch', false);
    }
  }

  async function debugMemoryRecall(params: Parameters<typeof store.recallDebug>[0]) {
    setLoading('debugRecall', true);
    try {
      return await store.recallDebug(params);
    } finally {
      setLoading('debugRecall', false);
    }
  }

  async function getMemoryNeighborhood(centerID: string, params?: Parameters<typeof store.fetchNeighborhood>[1]) {
    setLoading('neighborhood', true);
    try {
      return await store.fetchNeighborhood(centerID, params);
    } finally {
      setLoading('neighborhood', false);
    }
  }

  async function getMemoryPlatformSettings() {
    setLoading('platformSettings', true);
    try {
      return await store.fetchPlatformSettings();
    } finally {
      setLoading('platformSettings', false);
    }
  }

  async function listMemoryDeadLetters(state?: string, limit?: number) {
    setLoading('deadLetters', true);
    try {
      return await store.fetchDeadLetters(state, limit);
    } finally {
      setLoading('deadLetters', false);
    }
  }

  async function updateMemoryPlatformSettings(input: Parameters<typeof store.savePlatformSettings>[0]) {
    setLoading('updatePlatformSettings', true);
    try {
      return await store.savePlatformSettings(input);
    } finally {
      setLoading('updatePlatformSettings', false);
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
