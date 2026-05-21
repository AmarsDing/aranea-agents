import { onUnmounted, ref, type Ref } from "vue";
import {
  cancelPendingMessage,
  getPendingMessages,
  updatePendingMessage,
  type PendingMessage
} from "../api";
import { messageQueuedFromEnvelope, type RunStatusFromWs } from "../envelopeRunStatus";
import type { Envelope } from "../envelope";

export function useFollowUpQueue(sessionId: Ref<string | undefined>, sending: Ref<boolean>) {
  const pendingMessages = ref<PendingMessage[]>([]);
  let pendingPollTimer: ReturnType<typeof setInterval> | null = null;

  async function refreshPendingMessages() {
    const sid = sessionId.value;
    if (!sid) {
      pendingMessages.value = [];
      return;
    }
    pendingMessages.value = await getPendingMessages(sid);
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
    const ok = await cancelPendingMessage(sid, pendingId);
    if (ok) {
      pendingMessages.value = pendingMessages.value.filter((pm) => pm.id !== pendingId);
    }
  }

  async function onUpdatePending(pendingId: string, content: string) {
    const sid = sessionId.value;
    if (!sid || !pendingId || !content.trim()) return;
    const ok = await updatePendingMessage(sid, pendingId, content.trim());
    if (ok) {
      pendingMessages.value = pendingMessages.value.map((pm) =>
        pm.id === pendingId ? { ...pm, content: content.trim() } : pm
      );
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
