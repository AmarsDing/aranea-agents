/**
 * 底部指令窗里直接记一条任务（2026-11-20 起取代「我的任务」里的输入框）：
 * 建任务 → 传附件 → 写了「@某 Agent」/「让某某…」就立刻派给它；「记：…」且没带附件的记成随手记录。
 * 做完在 window 上广播 COMMAND_DONE_EVENT，开着的今日页 / 任务页就地更新。
 */
import { createOfficeTask, TASK_FILE_MAX_BYTES, TASK_FILES_MAX, uploadTaskFile } from '../office/api';
import type { OfficeTaskRow } from '../office/types';
import { parseCommand, recipientRef, type Recipient } from '../today/command';
import { COMMAND_DONE_EVENT, type CommandDoneDetail } from '../today/useCommandRunner';
import type { DelegateOutcome } from '../today/useDelegation';
import { parseKratosApiError } from '../../utils/kratosError';

/** 页面请指令窗展开并聚焦（今日页「我的任务」、任务页「待办」上的 + 按钮） */
export const OPEN_DOCK_EVENT = 'aranea:open-dock';

export type QuickAddOutcome =
  | {
      ok: true;
      kind: 'task' | 'note' | 'delegate';
      task: OfficeTaskRow | null;
      recipient: Recipient | null;
      link: string;
      failedFiles: string[];
    }
  | { ok: false; message: string; link: string };

/** 一句话落成什么：有附件时只能是任务（随手记录不带附件）。 */
export function quickAddPlan(raw: string, recipients: readonly Recipient[], hasFiles: boolean) {
  const parsed = parseCommand(raw, recipients);
  const recipient = recipients.find((r) => recipientRef(r) === parsed.recipientRef) ?? null;
  if (parsed.intent === 'note' && !hasFiles)
    return { kind: 'note' as const, title: parsed.text || raw.trim(), recipient: null };
  if (recipient) return { kind: 'delegate' as const, title: parsed.text || raw.trim(), recipient };
  return { kind: 'task' as const, title: raw.trim(), recipient: null };
}

/** 附件先筛一遍：超过 50MB 的、超过 20 个的不收，返回能收的和拒收的原因 */
export function acceptFiles(
  current: number,
  incoming: readonly File[],
): { accepted: File[]; rejected: { name: string; reason: 'tooLarge' | 'tooMany' }[] } {
  const accepted: File[] = [];
  const rejected: { name: string; reason: 'tooLarge' | 'tooMany' }[] = [];
  for (const file of incoming) {
    if (file.size > TASK_FILE_MAX_BYTES) rejected.push({ name: file.name, reason: 'tooLarge' });
    else if (current + accepted.length >= TASK_FILES_MAX) rejected.push({ name: file.name, reason: 'tooMany' });
    else accepted.push(file);
  }
  return { accepted, rejected };
}

function announce(detail: CommandDoneDetail) {
  window.dispatchEvent(new CustomEvent<CommandDoneDetail>(COMMAND_DONE_EVENT, { detail }));
}

function errText(err: unknown): string {
  return parseKratosApiError(err).message || (err instanceof Error ? err.message : '') || 'error';
}

export async function quickAdd(
  raw: string,
  files: readonly File[],
  deps: {
    recipients: readonly Recipient[];
    dispatch: (task: OfficeTaskRow, recipient: Recipient) => Promise<DelegateOutcome>;
  },
): Promise<QuickAddOutcome> {
  const plan = quickAddPlan(raw, deps.recipients, files.length > 0);
  if (!plan.title) return { ok: false, message: 'empty', link: '' };
  let task: OfficeTaskRow;
  try {
    task = await createOfficeTask(plan.title, '', plan.kind === 'note' ? 'note' : 'manual');
  } catch (err) {
    return { ok: false, message: errText(err), link: '' };
  }
  if (plan.kind === 'note') {
    announce({ intent: 'note', task });
    return { ok: true, kind: 'note', task, recipient: null, link: '', failedFiles: [] };
  }
  const failedFiles: string[] = [];
  for (const file of files) {
    try {
      await uploadTaskFile(task.id, file);
    } catch (err) {
      failedFiles.push(`${file.name}（${errText(err)}）`);
    }
  }
  task = { ...task, file_count: files.length - failedFiles.length };
  if (!plan.recipient) {
    announce({ intent: 'task', task });
    return { ok: true, kind: 'task', task, recipient: null, link: '', failedFiles };
  }
  const outcome = await deps.dispatch(task, plan.recipient);
  if (!outcome.ok) {
    // 没派出去：任务已经建好，留在我的待办里
    announce({ intent: 'task', task });
    return { ok: false, message: outcome.message, link: outcome.link };
  }
  announce({ intent: 'delegate', task: outcome.task });
  return { ok: true, kind: 'delegate', task: outcome.task, recipient: plan.recipient, link: outcome.link, failedFiles };
}
