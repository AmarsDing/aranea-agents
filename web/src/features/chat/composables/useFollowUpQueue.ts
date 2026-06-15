import { onUnmounted, ref, type Ref } from 'vue';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import type { PendingMessage } from '../types';
import { messageQueuedFromEnvelope, type RunStatusFromWs } from '../envelopeRunStatus';
import type { Envelope } from '../envelope';

export function useFollowUpQueue(
  sessionId: Ref<string | undefined>,
  sending: Ref<boolean>,
  notifyError?: (message: string) => void,
) {
  const pendingMessages = ref<PendingMessage[]>([]);
  /** One-shot poll timer: fires once after run starts, then WS events take over. */
  let pendingPollTimer: ReturnType<typeof setTimeout> | null = null;

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
      notifyError?.(err instanceof Error ? err.message : '加载排队消息失败');
      pendingMessages.value = [];
    }
  }

  function stopPendingPoll() {
    if (pendingPollTimer != null) {
      clearTimeout(pendingPollTimer);
      pendingPollTimer = null;
    }
  }

  /** Initial one-shot fetch after a short delay; subsequent updates come via WS events. */
  function scheduleInitialFetch() {
    stopPendingPoll();
    pendingPollTimer = setTimeout(() => {
      pendingPollTimer = null;
      void refreshPendingMessages();
    }, 500);
  }

  /** WS-driven refresh: called when a relevant envelope arrives.
   *  Replaces the old 3-second polling loop with real-time push. */
  function onRunStatusEnvelope(env: Envelope) {
    if (messageQueuedFromEnvelope(env)) {
      void refreshPendingMessages();
    }
    // Also refresh on run completion to clear stale pending items
    const meta = env.metadata ?? {};
    const rs = String(meta.status ?? '');
    if (rs === 'completed' || rs === 'cancelled' || rs === 'failed') {
      void refreshPendingMessages();
    }
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
        notifyError?.('取消排队消息失败');
      }
    } catch (err) {
      notifyError?.(err instanceof Error ? err.message : '取消排队消息失败');
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
          pm.id === pendingId ? { ...pm, content: content.trim() } : pm,
        );
      } else {
        notifyError?.('更新排队消息失败');
      }
    } catch (err) {
      notifyError?.(err instanceof Error ? err.message : '更新排队消息失败');
    }
  }

  async function onInterruptPending(pendingId: string) {
    const sid = sessionId.value;
    if (!sid || !pendingId) return;
    const runtime = useChatRuntimeStore();
    try {
      const ok = await runtime.interruptAndSend(sid, pendingId);
      if (ok) {
        pendingMessages.value = pendingMessages.value.filter((pm) => pm.id !== pendingId);
      } else {
        notifyError?.('立即发送失败');
      }
    } catch (err) {
      notifyError?.(err instanceof Error ? err.message : '立即发送失败');
    }
  }

  function watchSending(active: boolean) {
    if (active) {
      // One-shot fetch after 500ms; WS events handle subsequent updates
      scheduleInitialFetch();
    } else {
      // Final refresh after run completes, then stop
      setTimeout(() => {
        void refreshPendingMessages();
      }, 1000);
    }
  }

  onUnmounted(stopPendingPoll);

  return {
    pendingMessages,
    refreshPendingMessages,
    onRunStatusEnvelope,
    onCancelPending,
    onInterruptPending,
    onUpdatePending,
    watchSending,
  };
}
