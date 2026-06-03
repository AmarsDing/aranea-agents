import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listSessionEvents,
  type ListSessionEventsParams,
  type ListSessionEventsResult,
} from '../../features/event/api';
import type { Envelope } from '../../realtime/envelope';

export const useEventStore = defineStore('event', () => {
  const loading = ref(false);
  const error = ref('');

  async function fetchSessionEvents(params: ListSessionEventsParams): Promise<ListSessionEventsResult> {
    loading.value = true;
    error.value = '';
    try {
      return await listSessionEvents(params);
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load events';
      throw err;
    } finally {
      loading.value = false;
    }
  }

  return {
    loading,
    error,
    fetchSessionEvents,
  };
});

export type { ListSessionEventsParams, ListSessionEventsResult };
