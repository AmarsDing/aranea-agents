/**
 * Chat streaming snapshot cache.
 *
 * P2 #1: 原实现使用模块级 `ref<Record<string, ChatStreamingSnapshot>>`，
 * 但经验证 `snapshots.value` 从未在任何响应式上下文（template/computed）中被读取，
 * `get()` 方法也从未被调用。该 ref 实质上是一个"恰好用了 ref 包装的非响应式缓存"，
 * 反而误导维护者以为存在响应式订阅。
 *
 * 现改为 `Map` 实现：消除伪响应式开销和 API 误导，保留 put/get/clear 语义。
 * 若未来需要 DevTools 可见性，应迁入 Pinia defineStore。
 */
export type ChatStreamingSnapshot = {
  reasoning: string;
  partialText: string;
  updatedAt: number;
};

const snapshots = new Map<string, ChatStreamingSnapshot>();

export function useChatStreamingSnapshots() {
  function put(
    sessionId: string,
    patch: Partial<Pick<ChatStreamingSnapshot, 'reasoning' | 'partialText'>> & { replace?: boolean },
  ) {
    const sid = sessionId.trim();
    if (!sid) return;
    if (patch.replace) {
      snapshots.set(sid, {
        reasoning: patch.reasoning ?? '',
        partialText: patch.partialText ?? '',
        updatedAt: Date.now(),
      });
      return;
    }
    const cur = snapshots.get(sid) ?? { reasoning: '', partialText: '', updatedAt: 0 };
    snapshots.set(sid, {
      reasoning: patch.reasoning !== undefined ? `${cur.reasoning}${patch.reasoning}` : cur.reasoning,
      partialText: patch.partialText !== undefined ? `${cur.partialText}${patch.partialText}` : cur.partialText,
      updatedAt: Date.now(),
    });
  }

  function get(sessionId: string): ChatStreamingSnapshot | undefined {
    return snapshots.get(sessionId.trim());
  }

  function clear(sessionId: string) {
    const sid = sessionId.trim();
    if (!sid) return;
    snapshots.delete(sid);
  }

  return { put, get, clear };
}
