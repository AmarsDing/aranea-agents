import { ref, watch, type Ref } from "vue";
import type { Message } from "../../domain/types";
import { useSessionStore } from "../../stores/session/index";

export function useSessionMessagesPanel(sessionId: Ref<string>) {
  const sessionStore = useSessionStore();
  const messages = ref<Message[]>([]);
  const loading = ref(false);
  const error = ref("");

  async function loadMessages() {
    const id = sessionId.value.trim();
    if (!id) {
      messages.value = [];
      return;
    }
    loading.value = true;
    error.value = "";
    try {
      const result = await sessionStore.fetchMessages(id);
      messages.value = result.items;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "加载消息失败";
    } finally {
      loading.value = false;
    }
  }

  watch(sessionId, () => void loadMessages(), { immediate: true });

  return { messages, loading, error, loadMessages };
}
