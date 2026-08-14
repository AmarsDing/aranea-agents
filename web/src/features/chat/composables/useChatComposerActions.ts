import type { Ref } from 'vue';
import { useQuasar } from 'quasar';
import type { useChatSessionStore } from '../../../stores/chat/sessionStore';
import type { useChatMessageStore } from '../../../stores/chat/messageStore';
import type { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import type { useChatStreamManager } from './useChatStreamManager';
import type { useChatSender } from './useChatSender';
import type { Task } from '../v2Types';

export interface ComposerActionDeps {
  sessionStore: ReturnType<typeof useChatSessionStore>;
  messageStore: ReturnType<typeof useChatMessageStore>;
  runtimeStore: ReturnType<typeof useChatRuntimeStore>;
  streamManager: ReturnType<typeof useChatStreamManager>;
  sender: ReturnType<typeof useChatSender>;
  runStatus: Ref<string>;
  selectedSessionId: Ref<string | undefined>;
  notify: (opts: { type: string; message: string }) => void;
  t: (key: string, fallbackOrNamed?: string | Record<string, unknown>) => string;
  sessionDrafts: Map<string, string>;
  inputText: Ref<string>;
  selectedSkillSlugs: Ref<string[]>;
}

export function useChatComposerActions(deps: ComposerActionDeps) {
  const $q = useQuasar();
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
    inputText,
    selectedSkillSlugs,
  } = deps;

  async function onSend() {
    const sid = selectedSessionId.value;
    // Skill picker：发送前把选中技能的加载提示拼到输入框尾部（透明注入，
    // 气泡可见）。发送被接受（sender 清空输入框）后清空选中；失败保留。
    const slugs = selectedSkillSlugs.value;
    if (slugs.length > 0) {
      const prompt = t('chat.loadSkillPrompt', { slug: slugs.join(', ') });
      const raw = inputText.value.trimEnd();
      if (!raw.includes(prompt)) {
        inputText.value = raw ? `${raw}\n${prompt}` : prompt;
      }
    }
    await sender.onSend();
    if (sid) sessionDrafts.delete(sid);
    if (slugs.length > 0 && !inputText.value.trim()) {
      selectedSkillSlugs.value = [];
    }
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

  /** v2 Task 重新生成：确认后原封不动重新发送 UserMessage。 */
  function regenerateV2Task(task: Task) {
    const sid = selectedSessionId.value;
    if (!sid || !task.UserMessage) return;
    $q.dialog({
      title: t('chat.v2.regenerateConfirmTitle', '重新生成'),
      message: t('chat.v2.regenerateConfirmMessage', '是否重新发送该问题？'),
      cancel: { label: t('common.cancel', '取消'), flat: true },
      ok: { label: t('common.confirm', '确认'), color: 'primary' },
    }).onOk(() => {
      if (runStatus.value === 'running' || runStatus.value === 'pending') {
        streamManager.cancelActiveStream();
        sender.stopStreaming(sid);
      }
      const entityKind = sessionStore.entityKind;
      if (entityKind === 'team') {
        sender.sendTeamMessage(task.UserMessage);
      } else {
        sender.sendAgentUserContent(task.UserMessage);
      }
    });
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
    regenerateV2Task,
    cancelBackgroundJob,
  };
}
