/**
 * 指令的执行：建任务、记一笔、交办。Ctrl+K 指令面板与工作台指令栏共用这一份，
 * 两处行为完全一致。执行成功后在 window 上广播 COMMAND_DONE_EVENT，
 * 工作台据此立即刷新「我的任务 / 记录 / 交办中」，不必等下一轮轮询。
 */
import { createOfficeTask } from '../office/api';
import type { OfficeTaskRow } from '../office/types';
import { parseKratosApiError } from '../../utils/kratosError';
import type { CommandIntent, Recipient } from './command';
import { delegate } from './useDelegation';

export const COMMAND_DONE_EVENT = 'aranea:command-done';
/** 页面请壳层打开指令面板并带上一句话（detail 是那句话）；如会话中心的「交给精灵助手接着做」 */
export const OPEN_COMMAND_EVENT = 'aranea:open-command';

export type CommandRequest =
  | { intent: Exclude<CommandIntent, 'delegate'>; text: string }
  | { intent: 'delegate'; text: string; recipient: Recipient };

export type CommandResult =
  | {
      ok: true;
      intent: CommandIntent;
      /** 交办时承载会话的入口 */
      link: string;
      /** 交办已发出但没能登记到「交办中」 */
      untracked: boolean;
      task: OfficeTaskRow | null;
    }
  | { ok: false; message: string; link: string };

function errorText(err: unknown): string {
  return parseKratosApiError(err).message || (err instanceof Error ? err.message : '') || 'error';
}

export type CommandDoneDetail = { intent: CommandIntent; task: OfficeTaskRow | null };

function announce(detail: CommandDoneDetail) {
  window.dispatchEvent(new CustomEvent<CommandDoneDetail>(COMMAND_DONE_EVENT, { detail }));
}

export async function runCommand(request: CommandRequest): Promise<CommandResult> {
  const text = request.text.trim();
  if (!text) return { ok: false, message: 'empty', link: '' };
  if (request.intent === 'delegate') {
    const outcome = await delegate(request.recipient, text);
    if (!outcome.ok) return { ok: false, message: outcome.message, link: outcome.link };
    announce({ intent: 'delegate', task: outcome.task });
    return { ok: true, intent: 'delegate', link: outcome.link, untracked: !outcome.task, task: outcome.task };
  }
  try {
    const task = await createOfficeTask(text, '', request.intent === 'note' ? 'note' : 'manual');
    announce({ intent: request.intent, task });
    return { ok: true, intent: request.intent, link: '', untracked: false, task };
  } catch (err) {
    return { ok: false, message: errorText(err), link: '' };
  }
}
