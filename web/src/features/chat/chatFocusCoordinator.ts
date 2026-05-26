export type FocusSessionOptions = {
  /** Skip full message reload when auto-focusing during a live channel turn (DECO-R-P2-01). */
  skipMessageReload?: boolean;
};

type InflightEntry = {
  promise: Promise<void>;
  /** OR-merge: any concurrent caller requesting live skip wins for the whole focus. */
  skipMessageReload: boolean;
};

/** Per-workspace coordinator for channel/route focus dedupe (not module-global). */
export function createChatFocusCoordinator() {
  let suppressRouteSessionWatch = 0;
  const focusSessionInflight = new Map<string, InflightEntry>();

  function focusKey(sessionId: string, agentId?: string): string {
    return `${sessionId.trim()}:${agentId?.trim() ?? ""}`;
  }

  function isRouteSessionWatchSuppressed(): boolean {
    return suppressRouteSessionWatch > 0;
  }

  async function withRouteWatchSuppressed<T>(fn: () => Promise<T>): Promise<T> {
    suppressRouteSessionWatch += 1;
    try {
      return await fn();
    } finally {
      suppressRouteSessionWatch = Math.max(0, suppressRouteSessionWatch - 1);
    }
  }

  /**
   * Run at most one focus task per session+agent. Concurrent callers share the same promise;
   * skipMessageReload is merged with OR so a live channel focus cannot be overwritten by a full reload.
   */
  function runFocusOnce(
    key: string,
    options: FocusSessionOptions | undefined,
    task: (resolveSkipReload: () => boolean) => Promise<void>
  ): Promise<void> {
    const wantsSkip = Boolean(options?.skipMessageReload);
    const existing = focusSessionInflight.get(key);
    if (existing) {
      if (wantsSkip) existing.skipMessageReload = true;
      return existing.promise;
    }

    const entry: InflightEntry = { skipMessageReload: wantsSkip, promise: undefined! };
    entry.promise = task(() => entry.skipMessageReload)
      .catch((err) => {
        console.warn("[chat] focus session failed:", key, err);
        throw err;
      })
      .finally(() => {
        if (focusSessionInflight.get(key) === entry) {
          focusSessionInflight.delete(key);
        }
      });
    focusSessionInflight.set(key, entry);
    return entry.promise;
  }

  return {
    focusKey,
    isRouteSessionWatchSuppressed,
    withRouteWatchSuppressed,
    runFocusOnce,
  };
}

export type ChatFocusCoordinator = ReturnType<typeof createChatFocusCoordinator>;
