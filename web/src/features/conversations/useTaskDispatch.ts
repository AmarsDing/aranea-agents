// 会话中心派活：新任务 / 交给下一位（P1，只用现成接口）。设计：07-会话中心.md §12.4
//   新任务：建资料袋会话（属主是精灵助手，metadata 标 task_record）→ 资料存进去 → 按执行者派出 → 登记到工作台交办。
//   交给下一位：挂在任务原来的锚点会话下（资料袋，或旧任务的承载会话）。
//   智能体 / 团队：新开一段会话（metadata 带 cc_task + 交接单），资料复制过去作为附件随第一句发出。
//   编码工具 / 运行台：资料给本机绝对路径（宿主机模式下产物就在本机磁盘），交接单作为提示。
import { getArtifact, uploadArtifact } from '../artifact/api';
import { readFileAsBase64 } from '../artifact/fileBase64';
import { ARTIFACT_MAX_BYTES } from '../artifact/limits';
import type { ArtifactMeta } from '../artifact/types';
import { sendMessage } from '../chat/api';
import { dispatchCodingTask } from '../coding/api';
import { createRun, createTerminal } from '../fleet/api';
import { createOfficeTask } from '../office/api';
import { createSession, deleteSession, getSession, updateSessionMetadata } from '../session/api';
import type { ProjectCandidate } from '../../services/kratos/agentbridge/v1';
import { parseKratosApiError } from '../../utils/kratosError';
import { randomUUID } from '../../utils/uuid';
import { isAbsPath, mergeMeta, parseMeta, type CcLink, type Deliverable } from './handoff';

export type Target =
  | { kind: 'agent' | 'team'; id: string; key: string; name: string }
  | { kind: 'tool'; agentKey: string; name: string; project: string }
  | { kind: 'terminal'; preset: string; name: string; workspaceId: string }
  | { kind: 'run'; preset: string; name: string; projectId: string; workspaceId: string };

/** 一份资料：刚选的本机文件，或任务里已有的文件（资料袋 / 前面交出来的） */
export type Material = { file: File } | { meta: ArtifactMeta };

export interface DispatchInput {
  mode: 'new' | 'handoff';
  /** 新任务的标题；交接时是任务标题（新会话也用它） */
  title: string;
  body: string;
  target: Target;
  materials: Material[];
  deliver: Deliverable[];
  /** 交接单：资料路径要等存好才知道，所以由调用方按资料清单现拼 */
  buildHand: (mats: { name: string; path?: string }[]) => string;
  /** 资料袋会话的属主（精灵助手） */
  spiritId: string;
  recordTitle: string;
  /** 交接：任务的锚点会话；为空（任务里没有会话）就新建一个资料袋，成为新任务 */
  anchorId?: string;
}

export type DispatchErrorCode = 'no_local_path' | 'too_large' | 'not_sent' | 'failed';

export type DispatchOutcome =
  | { ok: true; anchorId: string; sessionId: string; registered: boolean }
  | { ok: false; code: DispatchErrorCode; message: string; candidates?: ProjectCandidate[] };

class DispatchError extends Error {
  constructor(
    readonly code: DispatchErrorCode,
    message = '',
  ) {
    super(message);
  }
}

async function upload(sessionId: string, file: File): Promise<ArtifactMeta> {
  if (file.size > ARTIFACT_MAX_BYTES) throw new DispatchError('too_large', file.name);
  return uploadArtifact({
    session_id: sessionId,
    name: file.name,
    mime_type: file.type || undefined,
    data_base64: await readFileAsBase64(file),
  });
}

// shortcut: 附件只认本会话的产物，跨会话要下载再上传；P2 允许引用同一任务资料袋里的文件后去掉
async function copy(meta: ArtifactMeta, sessionId: string): Promise<ArtifactMeta> {
  if (meta.session_id === sessionId) return meta;
  const data = await getArtifact(meta.id);
  return uploadArtifact({
    session_id: sessionId,
    name: meta.name,
    mime_type: meta.mime_type || undefined,
    data_base64: data.data_base64,
  });
}

async function link(anchorId: string, item: CcLink) {
  const s = await getSession(anchorId);
  const links = parseMeta(s.metadata_json).cc_links ?? [];
  await updateSessionMetadata(anchorId, mergeMeta(s.metadata_json, { cc_links: [...links, item] }));
}

export async function dispatchTask(input: DispatchInput): Promise<DispatchOutcome> {
  const { target } = input;
  let anchor = input.anchorId ?? '';
  let createdRecord = false;
  let sessionId = '';
  try {
    if (!anchor) {
      const rec = await createSession({
        owner_type: 'agent',
        agent_id: input.spiritId,
        title: input.recordTitle,
        metadata_json: JSON.stringify({ cc_role: 'task_record', cc_deliver: input.deliver }),
      });
      anchor = rec.id;
      createdRecord = true;
    }
    // 新选的文件先存进锚点会话：交给编码工具时要它的本机路径，以后交接也从这里取
    const stored: ArtifactMeta[] = [];
    for (const m of input.materials) stored.push('meta' in m ? m.meta : await upload(anchor, m.file));

    if (target.kind === 'agent' || target.kind === 'team') {
      const hand = input.buildHand(stored.map((a) => ({ name: a.name })));
      const s = await createSession({
        owner_type: target.kind,
        agent_id: target.kind === 'agent' ? target.id : undefined,
        team_id: target.kind === 'team' ? target.id : undefined,
        title: input.title,
        metadata_json: JSON.stringify({ cc_task: anchor, cc_hand: hand }),
      });
      sessionId = s.id;
      const attachments: { id: string }[] = [];
      for (const a of stored) attachments.push({ id: (await copy(a, s.id)).id });
      const ack = await sendMessage({
        session_id: s.id,
        agent_key: target.kind === 'agent' ? target.key : undefined,
        team_id: target.kind === 'team' ? target.id : undefined,
        content: hand,
        request_id: `pending-user-${randomUUID()}`,
        options: attachments.length ? { attachments } : undefined,
      });
      if (ack.status !== 'accepted' && ack.status !== 'queued') throw new DispatchError('not_sent', ack.status);
    } else {
      const mats = stored.map((a) => ({ name: a.name, path: a.storage_uri }));
      if (mats.some((m) => !isAbsPath(m.path))) throw new DispatchError('no_local_path');
      const hand = input.buildHand(mats);
      if (target.kind === 'tool') {
        const res = await dispatchCodingTask({
          sessionId: anchor,
          agentKey: target.agentKey,
          projectQuery: target.project,
          prompt: hand,
        });
        if (!res.task?.id) {
          if (createdRecord) await deleteSession(anchor).catch(() => undefined);
          return { ok: false, code: 'failed', message: '', candidates: res.candidates ?? [] };
        }
      } else if (target.kind === 'terminal') {
        const term = await createTerminal({
          workspaceId: target.workspaceId,
          agent: target.preset,
          prompt: hand,
          title: input.title,
        });
        await link(anchor, { kind: 'terminal', id: term.id, hand });
      } else if (target.kind === 'run') {
        const run = await createRun({
          projectId: target.projectId,
          workspaceId: target.workspaceId,
          objective: hand,
          agent: target.preset,
          fullAccess: false,
        });
        await link(anchor, { kind: 'run', id: run.id, hand });
      }
    }
  } catch (err) {
    // 一位都没派出去：撤掉刚建的资料袋（进回收站），免得留下空任务
    if (createdRecord && !sessionId) await deleteSession(anchor).catch(() => undefined);
    if (err instanceof DispatchError) return { ok: false, code: err.code, message: err.message };
    return { ok: false, code: 'failed', message: parseKratosApiError(err).message || String(err) };
  }

  // 新任务登记到工作台交办（「任务」页签的交办中能看到）；登记失败不影响已经派出去的事
  let registered = !createdRecord;
  if (createdRecord) {
    try {
      const person = target.kind === 'agent' || target.kind === 'team';
      await createOfficeTask(input.title, input.body, {
        sessionId: anchor,
        assigneeKind: target.kind === 'team' ? 'team' : 'agent',
        assigneeId: person ? target.id : input.spiritId,
        assigneeName: target.name,
      });
      registered = true;
    } catch (err) {
      console.warn('[cc] dispatched but not registered', err);
    }
  }
  return { ok: true, anchorId: anchor, sessionId, registered };
}
