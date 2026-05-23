import { ref, readonly, onUnmounted } from "vue";
import type { RunStatus, RunStatusValue } from "../features/chat/types";
import { useChatStore } from "../stores/chat";

const POLL_INTERVAL_MS = 2000;

/**
 * Polls GetRunStatus for a given session (legacy helper for non-workspace views).
 * Prefer useChatRunStatus inside Chat workspace.
 */
export function useRunStatus(sessionId: string) {
  const chatStore = useChatStore();
  const status = ref<RunStatusValue>("idle");
  const runId = ref("");
  const errorMessage = ref("");
  const updatedAt = ref("");
  const isAwaiting = ref(false);

  let timer: ReturnType<typeof setInterval> | null = null;

  async function poll() {
    if (!sessionId) return;
    try {
      const rs: RunStatus = await chatStore.fetchRunStatus(sessionId);
      status.value = rs.status;
      runId.value = rs.runId;
      errorMessage.value = rs.errorMessage;
      updatedAt.value = rs.updatedAt;
      isAwaiting.value = rs.status === "awaiting_user";
    } catch (err) {
      errorMessage.value = err instanceof Error ? err.message : "fetchRunStatus failed";
    }
  }

  function startPolling() {
    if (timer !== null) return;
    void poll();
    timer = setInterval(() => void poll(), POLL_INTERVAL_MS);
  }

  function stopPolling() {
    if (timer !== null) {
      clearInterval(timer);
      timer = null;
    }
  }

  async function submitReply(reply: string): Promise<boolean> {
    return chatStore.submitAwaitReply(sessionId, reply, runId.value || undefined);
  }

  onUnmounted(() => stopPolling());

  return {
    status: readonly(status),
    runId: readonly(runId),
    errorMessage: readonly(errorMessage),
    updatedAt: readonly(updatedAt),
    isAwaiting: readonly(isAwaiting),
    startPolling,
    stopPolling,
    submitReply,
    refresh: poll
  };
}
