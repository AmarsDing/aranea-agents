import { onUnmounted, ref, type Ref } from "vue";
import { useChatRuntimeStore } from "../../../stores/chat/runtimeStore";
import type { PendingMessage } from "../api";
import { messageQueuedFromEnvelope, type RunStatusFromWs } from "../envelopeRunStatus";
import type { Envelope } from "../envelope";

export function useFollowUpQueue(
  sessionId: Ref<string | undefined>,
  sending: Ref<boolean>,
  notifyError?: (message: string) => void
) {
  const pendingMessages = ref<PendingMessage[]>([]);
  let pendingPollTimer: ReturnType<typeof setInterval> | null = null;

  async function refreshPendingMessages() {
    const sid = sessionId.value;
    if (!sid) {
      pendingMessages.value = [];
      return;
    }
    const runtime = useChatRuntimeStore();
    try {
      pendingMessages.value = await runtime.fetchPendingMessages(sid);
    } catch (err) {
      notifyError?.(err instanceof Error ? err.message : "加载排队消息失败");
      pendingMessages.value = [];
    }
  }

  function stopPendingPoll() {
    if (pendingPollTimer != null) {
      clearInterval(pendingPollTimer);
      pendingPollTimer = null;
    }
  }

  function startPendingPoll() {
    stopPendingPoll();
    void refreshPendingMessages();
    pendingPollTimer = setInterval(refreshPendingMessages, 3000);
  }

  function onRunStatusEnvelope(env: Envelope) {
    if (messageQueuedFromEnvelope(env)) {
      void refreshPendingMessages();
    }
  }

  function onRunStatusHint(_rs: RunStatusFromWs) {
    // reserved for non-envelope hints
  }

  async function onCancelPending(pendingId: string) {
    const sid = sessionId.value;
    if (!sid || !pendingId) return;
    const runtime = useChatRuntimeStore();
    try {
      const ok = await runtime.cancelPending(sid, pendingId);
      if (ok) {
        pendingMessages.value = pendingMessages.value.filter((pm) => pm.id !== pendingId);
      } else {
        notifyError?.("取消排队消息失败");
      }
    } catch (err) {
      notifyError?.(err instanceof Error ? err.message : "取消排队消息失败");
    }
  }

  async function onUpdatePending(pendingId: string, content: string) {
    const sid = sessionId.value;
    if (!sid || !pendingId || !content.trim()) return;
    const runtime = useChatRuntimeStore();
    try {
      const ok = await runtime.updatePending(sid, pendingId, content.trim());
      if (ok) {
        pendingMessages.value = pendingMessages.value.map((pm) =>
          pm.id === pendingId ? { ...pm, content: content.trim() } : pm
        );
      } else {
        notifyError?.("更新排队消息失败");
      }
    } catch (err) {
      notifyError?.(err instanceof Error ? err.message : "更新排队消息失败");
    }
  }

  function watchSending(active: boolean) {
    if (active) {
      startPendingPoll();
    } else {
      setTimeout(() => {
        void refreshPendingMessages().then(() => {
          if (pendingMessages.value.length === 0) {
            stopPendingPoll();
          }
        });
      }, 1000);
    }
  }

  onUnmounted(stopPendingPoll);

  return {
    pendingMessages,
    refreshPendingMessages,
    startPendingPoll,
    stopPendingPoll,
    onRunStatusHint,
    onRunStatusEnvelope,
    onCancelPending,
    onUpdatePending,
    watchSending
  };
}
