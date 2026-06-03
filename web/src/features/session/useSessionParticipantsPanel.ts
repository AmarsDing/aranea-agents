import { ref, watch, type Ref } from 'vue';
import type { SessionParticipant } from './types';
import { useSessionStore } from '../../stores/session';

export function useSessionParticipantsPanel(sessionId: Ref<string>) {
  const sessionStore = useSessionStore();
  const participants = ref<SessionParticipant[]>([]);
  const loading = ref(false);
  const error = ref('');

  async function loadParticipants() {
    const id = sessionId.value.trim();
    if (!id) {
      participants.value = [];
      return;
    }
    loading.value = true;
    error.value = '';
    try {
      participants.value = await sessionStore.fetchParticipants(id);
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载参与者失败';
    } finally {
      loading.value = false;
    }
  }

  watch(sessionId, () => void loadParticipants(), { immediate: true });

  return { participants, loading, error, loadParticipants };
}
