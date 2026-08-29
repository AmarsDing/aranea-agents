import type { ComputedRef } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import type { SessionView } from '../../../components/chat/types';
import type { ConfirmStepPayload, SubmitClarificationPayload } from '../types';
import { confirmActivity, confirmActivityGrant, submitClarification } from '../api';
import { useChatSessionStore } from '../../../stores/chat/sessionStore';
import type { useChatStreamManager } from './useChatStreamManager';

interface ChatConfirmFlowsDeps {
  selectedSessionForUi: ComputedRef<SessionView | null>;
  streamManager: ReturnType<typeof useChatStreamManager>;
}

/**
 * Session action flows extracted from useChatWorkspace: manual context
 * compaction, HITL confirm/grant, clarification submit, and interrupted-task
 * resume. Each handler talks to the API/WS and surfaces the outcome via
 * $q.notify; the workspace only re-exports them in the session group.
 */
export function useChatConfirmFlows(deps: ChatConfirmFlowsDeps) {
  const { t } = useI18n();
  const $q = useQuasar();
  const sessionStore = useChatSessionStore();
  const { selectedSessionForUi, streamManager } = deps;

  async function onCompactSession(sessionId: string) {
    try {
      const result = await sessionStore.compactSessionAction(sessionId);
      if (result.compacted) {
        const before = Math.round((result.estimated_tokens_before / 1000) * 10) / 10;
        const after = Math.round((result.estimated_tokens_after / 1000) * 10) / 10;
        $q.notify({
          type: 'positive',
          message: t('chat.contextManuallyCompressed', { before, after }),
          timeout: 4000,
        });
      } else {
        $q.notify({
          type: 'info',
          message: t('chat.contextNoCompactionNeeded'),
          timeout: 3000,
        });
      }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      $q.notify({ type: 'negative', message: t('chat.contextCompactFailed') + `: ${msg}`, timeout: 5000 });
    }
  }

  // N-14: Handle confirm-activity event from ConfirmBlock → API call.
  // Encapsulated here (rather than in ChatPage.vue) to comply with FD2:
  // Page must not import API directly.
  async function onConfirmActivity(activityId: string, approved: boolean) {
    const sid = selectedSessionForUi.value?.id;
    if (!sid) return;
    try {
      const ok = await confirmActivity(sid, activityId, approved);
      if (!ok) {
        $q.notify({
          type: 'warning',
          message: approved ? t('chat.confirmActivity.approveRejected') : t('chat.confirmActivity.denyRejected'),
        });
      }
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('chat.confirmActivity.failed') });
    }
  }

  async function onConfirmActivityGrant(payload: ConfirmStepPayload) {
    try {
      const ok = await confirmActivityGrant(payload);
      payload.onSettled?.(ok);
      if (!ok) {
        $q.notify({
          type: 'warning',
          message: t('chat.confirmActivity.approveRejected'),
        });
      }
    } catch (err) {
      payload.onSettled?.(false);
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('chat.confirmActivity.failed') });
    }
  }

  // Clarification Gate (B.10.18): Handle submit-clarification event from
  // ClarifyBlock → API call. The backend flips the step to completed and
  // resumes the turn; the WS step.updated event drives the card's summary view.
  async function onSubmitClarification(payload: SubmitClarificationPayload) {
    try {
      const ok = await submitClarification(payload);
      if (!ok) {
        $q.notify({
          type: 'warning',
          message: t('chat.clarify.submitRejected'),
        });
      }
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('chat.clarify.submitFailed') });
    }
  }

  /**
   * L3: Resume an interrupted task (server-restart recovery).
   *
   * Sends a `resume_task` WS upstream on the task's chat stream. The backend
   * CAS-claims the task (interrupted → running) and reruns it with the
   * persisted execution trace; the resulting `task.updated` event drives the
   * UI back to the running state — no optimistic local update needed.
   * Failures surface as a ws_error notice from the backend.
   */
  function resumeTask(task: { ID: string; SessionID: string }) {
    const sid = task.SessionID || selectedSessionForUi.value?.id;
    if (!sid || !task.ID) return;
    try {
      const stream = streamManager.ensureChatStream(sid);
      streamManager.sendChatViaWs(stream, {
        direction: 'client_to_server',
        channel: 'chat',
        type: 'resume_task',
        payload: { task_id: task.ID },
      });
      $q.notify({ type: 'info', message: t('chat.v2.resumeTaskSent'), timeout: 1500 });
    } catch (err) {
      console.warn('[chat] resume_task send failed', err);
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('chat.sendFailed') });
    }
  }

  return {
    onCompactSession,
    onConfirmActivity,
    onConfirmActivityGrant,
    onSubmitClarification,
    resumeTask,
  };
}
