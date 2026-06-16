import { ref } from 'vue';

export type ChatStreamingSnapshot = {
  reasoning: string;
  partialText: string;
  updatedAt: number;
};

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
