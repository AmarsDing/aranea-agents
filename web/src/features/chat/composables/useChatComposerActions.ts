import type { Ref } from 'vue';
import type { useChatSessionStore } from '../../../stores/chat/sessionStore';
import type { useChatMessageStore } from '../../../stores/chat/messageStore';
import type { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import type { useChatStreamManager } from './useChatStreamManager';
import type { useChatSender } from './useChatSender';

export interface ComposerActionDeps {
  sessionStore: ReturnType<typeof useChatSessionStore>;
  messageStore: ReturnType<typeof useChatMessageStore>;
  runtimeStore: ReturnType<typeof useChatRuntimeStore>;
  streamManager: ReturnType<typeof useChatStreamManager>;
  sender: ReturnType<typeof useChatSender>;
  runStatus: Ref<string>;
  selectedSessionId: Ref<string | undefined>;
  notify: (opts: { type: string; message: string }) => void;
  t: (key: string, fallback?: string) => string;
  sessionDrafts: Map<string, string>;
}

export function useChatComposerActions(deps: ComposerActionDeps) {
  const {
    sessionStore,
    messageStore,
    runtimeStore,
    streamManager,
    sender,
    runStatus,
    selectedSessionId,
    notify,
    t,
    sessionDrafts,
  } = deps;

  async function onSend() {
    const sid = selectedSessionId.value;
    await sender.onSend();
    if (sid) sessionDrafts.delete(sid);
  }

  function dismissFailedMessage(messageId: string) {
    const sid = selectedSessionId.value;
    if (!sid) return;
    messageStore.setMessages(
      sid,
      messageStore.getMessages(sid).filter((m) => m.id !== messageId),
    );
  }

  function regenerateMessage(message: { id: string; content_markdown: string; role: string }) {
    const sid = selectedSessionId.value;
    if (!sid) return;
    if (runStatus.value === 'running' || runStatus.value === 'pending') {
      streamManager.cancelActiveStream();
      sender.stopStreaming(sid);
    }

    let userMsg = '';
    if (message.role === 'user') {
      // Regenerate from the selected user message itself.
      userMsg = message.content_markdown;
    } else {
      // For assistant/tool/etc messages, find the user message in the same turn.
      const msgs = messageStore.getMessages(sid);
      const idx = msgs.findIndex((m) => m.id === message.id);
      if (idx < 0) return;
      for (let i = idx - 1; i >= 0; i--) {
        if (msgs[i].role === 'user') {
          userMsg = msgs[i].content_markdown;
          break;
        }
      }
    }
    if (!userMsg) return;
    const entityKind = sessionStore.entityKind;
    if (entityKind === 'team') {
      sender.sendTeamMessage(userMsg);
    } else {
      sender.sendAgentUserContent(userMsg);
    }
  }

  async function cancelBackgroundJob(job: { id: string; source: string }) {
    try {
      const ok = await runtimeStore.cancelBackgroundJob(job.id, job.source);
      if (ok) {
        notify({ type: 'positive', message: t('chat.job.cancelled', '任务已取消') });
      } else {
        notify({ type: 'warning', message: t('chat.job.cancelFailed', '取消失败') });
      }
    } catch {
      notify({ type: 'warning', message: t('chat.job.cancelFailed', '取消失败') });
    }
  }

  return {
    onSend,
    dismissFailedMessage,
    regenerateMessage,
    cancelBackgroundJob,
  };
}
