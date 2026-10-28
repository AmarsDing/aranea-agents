import { defineStore } from 'pinia';
import { onScopeDispose, reactive, ref } from 'vue';
import { CHAT_DRAFT_KEY, CHAT_DRAFT_RESET, type ChatDraftRecord } from '../../features/chat/chatDraftPersistence';

export const useChatDraftStore = defineStore('chat-drafts', () => {
  const records = reactive(new Map<string, ChatDraftRecord>());
  const saveFailed = ref(false);
  try {
    const saved: unknown = JSON.parse(sessionStorage.getItem(CHAT_DRAFT_KEY) || '[]');
    if (Array.isArray(saved))
      for (const row of saved) {
        if (!Array.isArray(row) || typeof row[0] !== 'string' || !row[1]) continue;
        const value = row[1] as ChatDraftRecord;
        if (
          typeof value.text === 'string' &&
          Number.isSafeInteger(value.revision) &&
          value.revision >= 0 &&
          Number.isFinite(value.updatedAt) &&
          Date.now() - value.updatedAt < 86_400_000
        )
          records.set(row[0], value);
      }
  } catch {
    saveFailed.value = true;
  }

  function persist() {
    try {
      sessionStorage.setItem(CHAT_DRAFT_KEY, JSON.stringify([...records].filter(([, value]) => value.text.length > 0)));
      saveFailed.value = false;
    } catch {
      saveFailed.value = true;
    }
  }
  function reset() {
    records.clear();
    saveFailed.value = false;
  }
  if (typeof window !== 'undefined') {
    window.addEventListener(CHAT_DRAFT_RESET, reset);
    onScopeDispose(() => window.removeEventListener(CHAT_DRAFT_RESET, reset));
  }
  return { records, saveFailed, persist };
});
