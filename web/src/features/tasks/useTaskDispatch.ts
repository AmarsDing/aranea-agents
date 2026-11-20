/**
 * 派给 AI 的共用入口：Agent / 团队名单（整页共用一份，第一次用到时才读）与派发动作。
 * 卡片上的「派给 AI」、详情里的派发、输入框里的「@某某」都走这里。
 */
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { OfficeTaskRow } from '../office/types';
import type { Recipient } from '../today/command';
import { dispatchTask, followUp, forwardFiles, useRecipients, type DelegateOutcome } from '../today/useDelegation';

const shared = useRecipients();
const loading = ref<Promise<void> | null>(null);

function dueText(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function useTaskDispatch() {
  const { t } = useI18n();

  /** 名单只读一次；读失败的允许下次再试。 */
  function ensureRecipients(): Promise<void> {
    if (shared.status.value === 'ok') return Promise.resolve();
    if (!loading.value) loading.value = shared.load().finally(() => (loading.value = null));
    return loading.value;
  }

  function dispatch(task: OfficeTaskRow, recipient: Recipient, note = ''): Promise<DelegateOutcome> {
    return dispatchTask(task, recipient, {
      note,
      formatDue: dueText,
      labels: {
        due: t('taskBoard.msg.due'),
        attached: t('taskBoard.msg.attached'),
        skipped: t('taskBoard.msg.skipped'),
        note: t('taskBoard.msg.note'),
      },
    });
  }

  function sendFollowUp(
    run: { sessionId?: string; assigneeKind?: string; assigneeId?: string },
    text: string,
  ): Promise<string> {
    return followUp(run, text, shared.recipients.value);
  }

  function sendFiles(
    run: { sessionId?: string; assigneeKind?: string; assigneeId?: string },
    files: readonly File[],
  ): Promise<string> {
    return forwardFiles(run, files, shared.recipients.value, t('taskBoard.msg.moreFiles'));
  }

  return {
    recipients: shared.recipients,
    recipientsStatus: shared.status,
    ensureRecipients,
    dispatch,
    sendFollowUp,
    sendFiles,
  };
}

export { dueText };
