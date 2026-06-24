import { ref } from 'vue';

export type ChatStreamingSnapshot = {
  reasoning: string;
  partialText: string;
  updatedAt: number;
};

// TECH-DEBT(S13): module-level singleton ref bypasses Pinia.
// Reason: streaming snapshot cache is hot-path (updated on every text_delta /
// reasoning_delta envelope, potentially hundreds of times per second). Pinia's
// proxy wrapping adds overhead that is undesirable here. The state is also
// session-scoped and ephemeral (cleared on session switch), not persisted.
// Migration plan: if DevTools visibility becomes necessary, wrap in a
// defineStore('chatStreamingSnapshots', ...) with direct state mutation
// (no actions needed). All consumers already go through useChatStreamingSnapshots().
// Issue: tracking — frontend architecture cleanup.
const snapshots = ref<Record<string, ChatStreamingSnapshot>>({});

export function useChatStreamingSnapshots() {
  function put(
    sessionId: string,
    patch: Partial<Pick<ChatStreamingSnapshot, 'reasoning' | 'partialText'>> & { replace?: boolean },
  ) {
    const sid = sessionId.trim();
    if (!sid) return;
    if (patch.replace) {
      snapshots.value[sid] = {
        reasoning: patch.reasoning ?? '',
        partialText: patch.partialText ?? '',
        updatedAt: Date.now(),
      };
      return;
    }
    const cur = snapshots.value[sid] ?? { reasoning: '', partialText: '', updatedAt: 0 };
    snapshots.value[sid] = {
      reasoning: patch.reasoning !== undefined ? `${cur.reasoning}${patch.reasoning}` : cur.reasoning,
      partialText: patch.partialText !== undefined ? `${cur.partialText}${patch.partialText}` : cur.partialText,
      updatedAt: Date.now(),
    };
  }

  function get(sessionId: string): ChatStreamingSnapshot | undefined {
    return snapshots.value[sessionId.trim()];
  }

  function clear(sessionId: string) {
    const sid = sessionId.trim();
    if (!sid) return;
    const next = { ...snapshots.value };
    delete next[sid];
    snapshots.value = next;
  }

  return { snapshots, put, get, clear };
}
