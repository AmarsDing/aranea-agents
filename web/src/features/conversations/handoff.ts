// 会话中心的派活与交接（P1，纯前端）：资料袋会话、执行之间的交接单、做完交回什么。
// 设计：docs/aranea-agents/design/2026-11-17-三大模块重设计/07-会话中心.md §12
// shortcut: 任务和执行的关系记在会话的 metadata_json 里（资料袋会话 + 跟进会话的 cc_task），P2 有任务 ↔ 执行关联表后改为后端字段。

export const DELIVERABLES = ['say', 'doc', 'sheet', 'code'] as const;
export type Deliverable = (typeof DELIVERABLES)[number];

/** 运行台的终端 / 编排、流程的一次运行没有地方记「属于哪个任务」，挂在任务锚点会话的 metadata 里 */
export interface CcLink {
  kind: 'terminal' | 'run' | 'flow';
  id: string;
  hand: string;
  /** 流程：哪个流程、叫什么（运行记录里没有名字） */
  graphId?: string;
  name?: string;
}

/** 会话 metadata_json 里会话中心用到的键；其余键原样保留 */
export interface CcMeta {
  /** task_record = 资料袋会话：只放任务的资料、挂从它派出的编码任务，不算一次执行 */
  cc_role?: 'task_record';
  /** 跟进的执行：挂到这段会话（资料袋或任务的承载会话）所在的任务下 */
  cc_task?: string;
  /** 这次执行收到的交接单原文 */
  cc_hand?: string;
  cc_deliver?: Deliverable[];
  /** 交办登记没建成时，验收结果记在资料袋上 */
  cc_reviewed?: boolean;
  cc_links?: CcLink[];
}

export function parseMeta(json: string | undefined): CcMeta & Record<string, unknown> {
  try {
    const v: unknown = JSON.parse(json || '{}');
    return v && typeof v === 'object' && !Array.isArray(v) ? (v as CcMeta & Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

/** 在原有 metadata 上改几个键，不丢别人写的键 */
export const mergeMeta = (json: string | undefined, patch: CcMeta) => JSON.stringify({ ...parseMeta(json), ...patch });

/** 编码工具和后端在同一台机器上时，产物的 storage_uri 是本机绝对路径（宿主机模式），工具能按路径读 */
export const isAbsPath = (p: string) => /^([A-Za-z]:[\\/]|\\\\|\/)/.test(p);

export interface HandLabels {
  task: string;
  require: string;
  before: string;
  mats: string;
  ask: string;
  deliver: string;
  /** 「说明做了什么、还剩什么。」 */
  tail: string;
  /** 交给智能体时资料随消息作为附件发出，路径一栏写这个（含括号） */
  attached: string;
  /** 列表分隔符（中文「、」/ 英文 ", "） */
  sep: string;
  /** 段落内分句符（中文「；」/ 英文 "; "） */
  semi: string;
}

export interface HandInput {
  title: string;
  body?: string;
  /** 交接时：前面每一位做到哪，一位一句 */
  before?: string[];
  /** 交接时：要下一位做什么 */
  ask?: string;
  /** path 为空 = 作为附件发出 */
  materials: { name: string; path?: string }[];
  /** 已翻译的「交回什么」 */
  deliver: string[];
}

/** 交接单：固定几段、大白话，原样作为下一位收到的第一句话 */
export function buildHandText(input: HandInput, L: HandLabels): string {
  const out = [`${L.task}${input.title.trim()}`];
  if (input.body?.trim()) out.push(`${L.require}${input.body.trim()}`);
  if (input.before?.length) out.push(`${L.before}${input.before.join(L.semi)}`);
  if (input.materials.length)
    out.push(
      `${L.mats}\n${input.materials.map((m, i) => `${i + 1}. ${m.name}${m.path ? `   ${m.path}` : L.attached}`).join('\n')}`,
    );
  if (input.ask?.trim()) out.push(`${L.ask}${input.ask.trim()}`);
  out.push(`${L.deliver}${input.deliver.length ? input.deliver.join(L.sep) + L.semi : ''}${L.tail}`);
  return out.join('\n');
}

/** 输入框：第一行是标题，其余是补充要求 */
export function splitTitle(text: string): { title: string; body: string } {
  const [first = '', ...rest] = text.trim().split('\n');
  return { title: first.trim(), body: rest.join('\n').trim() };
}

/** 资料清单上的类型小标和大小 */
export const fileExt = (name: string) => (/\.([a-z0-9]{1,4})$/i.exec(name)?.[1] ?? 'FILE').toUpperCase();
export const fmtSize = (b: number) =>
  b >= 1048576 ? `${(b / 1048576).toFixed(1)} MB` : `${Math.max(1, Math.round(b / 1024))} KB`;
