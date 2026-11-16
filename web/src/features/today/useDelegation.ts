/**
 * 指令栏的交办：可交办对象的读取，以及「新开会话 → 发出指令 → 登记到交办中」。
 * 三步里前两步任一失败都算没交办成；第三步失败时事情已经派出去了，只是
 * 工作台上看不到，如实告诉用户并给出会话入口。
 */
import { ref } from 'vue';
import { listAgents } from '../agents/api';
import { sendMessage } from '../chat/api';
import { createOfficeTask } from '../office/api';
import type { OfficeTaskRow } from '../office/types';
import { createSession } from '../session/api';
import { listTeams } from '../teams/api';
import { parseKratosApiError } from '../../utils/kratosError';
import { randomUUID } from '../../utils/uuid';
import { commandTitle, sessionLink, type Recipient } from './command';

export type RecipientsStatus = 'loading' | 'ok' | 'error';

const AGENT_FETCH_LIMIT = 200;

export type DelegateOutcome =
  | {
      ok: true;
      /** 承载这次交办的会话入口 */
      link: string;
      /** 登记到「交办中」的任务；登记失败时为 null */
      task: OfficeTaskRow | null;
    }
  | {
      ok: false;
      message: string;
      /** 会话已建好但指令没发出去时给出，便于用户进去重发 */
      link: string;
    };

function errorText(err: unknown): string {
  return parseKratosApiError(err).message || (err instanceof Error ? err.message : '') || 'error';
}

export function useRecipients() {
  const recipients = ref<Recipient[]>([]);
  const status = ref<RecipientsStatus>('loading');

  /** Agent 与团队各自读取：一边失败或无权限不影响另一边，都失败才算读取失败。 */
  async function load(): Promise<void> {
    const [agents, teams] = await Promise.allSettled([listAgents({ limit: AGENT_FETCH_LIMIT }), listTeams()]);
    const next: Recipient[] = [];
    if (agents.status === 'fulfilled') {
      for (const agent of agents.value) {
        if (!agent.id || agent.status === 'inactive') continue;
        next.push({ kind: 'agent', id: agent.id, key: agent.agent_key, name: agent.display_name || agent.agent_key });
      }
    } else {
      console.warn('[today] agents load failed', agents.reason);
    }
    if (teams.status === 'fulfilled') {
      for (const team of teams.value) {
        if (!team.id || team.status === 'inactive') continue;
        next.push({ kind: 'team', id: team.id, key: team.team_key, name: team.display_name || team.team_key });
      }
    } else {
      console.warn('[today] teams load failed', teams.reason);
    }
    if (agents.status === 'rejected' && teams.status === 'rejected') {
      status.value = 'error';
      return;
    }
    recipients.value = next;
    status.value = 'ok';
  }

  return { recipients, status, load };
}

export async function delegate(recipient: Recipient, text: string): Promise<DelegateOutcome> {
  const content = text.trim();
  const title = commandTitle(content);
  let sessionId: string;
  try {
    const session = await createSession({
      owner_type: recipient.kind,
      agent_id: recipient.kind === 'agent' ? recipient.id : undefined,
      team_id: recipient.kind === 'team' ? recipient.id : undefined,
      title,
    });
    sessionId = session.id;
  } catch (err) {
    return { ok: false, message: errorText(err), link: '' };
  }
  const link = sessionLink(recipient, sessionId);
  try {
    const ack = await sendMessage({
      session_id: sessionId,
      agent_key: recipient.kind === 'agent' ? recipient.key : undefined,
      team_id: recipient.kind === 'team' ? recipient.id : undefined,
      content,
      request_id: `pending-user-${randomUUID()}`,
    });
    if (ack.status !== 'accepted' && ack.status !== 'queued') return { ok: false, message: ack.status, link };
  } catch (err) {
    return { ok: false, message: errorText(err), link };
  }
  try {
    const task = await createOfficeTask(title, content, {
      sessionId,
      assigneeKind: recipient.kind,
      assigneeId: recipient.id,
      assigneeName: recipient.name,
    });
    return { ok: true, link, task };
  } catch (err) {
    console.warn('[today] delegation sent but not registered', err);
    return { ok: true, link, task: null };
  }
}
