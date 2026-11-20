// 会话中心（工作台 › 对话）的数据口径：把各来源的会话拼成「任务 → 执行」。
// 设计：docs/aranea-agents/design/2026-11-17-三大模块重设计/07-会话中心.md §10
//   任务 = 一件要办成的事；执行 = 一个执行者（本平台智能体 / 编码工具）的一段会话。
//   P1 只用现成接口：本平台会话、工作台交办登记、编码工具桥任务、运行台编排与终端；在前端拼装。
import type { CodingTask } from '../../services/kratos/agentbridge/v1';
import type { AgentPreset, RunDetail, Terminal, Workspace } from '../fleet/api';
import type { OfficeTaskRow } from '../office/types';
import type { Session } from '../session/types';
import { parseMeta, type CcLink, type Deliverable } from './handoff';

export type ExecKind = 'session' | 'bridge' | 'terminal';
export type ExecStatus = 'waiting' | 'running' | 'failed' | 'done' | 'idle';
export type Bucket = 'active' | 'archived' | 'deleted';
export type TaskOrigin = 'office' | 'run' | 'auto';
/** waiting/failure 的原因代码，由页面翻成文案（不在这里写中文） */
export type ExecReason =
  | ''
  | 'tool_confirmation'
  | 'clarification'
  | 'agent_awaiting_reply'
  | 'approval'
  | 'input'
  | 'cancelled';

export interface Execution {
  /** `${kind}:${refId}`，页面里唯一 */
  id: string;
  kind: ExecKind;
  refId: string;
  /** 执行者：编码工具用工具键（cc / cx / wb / ds / gk / 其他工具 id），本平台用 `agent:<id>` / `team:<id>` */
  executor: string;
  executorName: string;
  what: string;
  model: string;
  status: ExecStatus;
  reason: ExecReason;
  /** 失败原因、最近一句等原文（来自数据，不翻译） */
  detail: string;
  start: string;
  last: string;
  turns: number;
  tokens: number;
  costUsd: number;
  bucket: Bucket;
  pinned: boolean;
  agentId: string;
  workspaceId: string;
  /** 编码任务：发起它的会话 */
  parentSessionId: string;
  project: string;
  /** 这次执行收到的交接单原文（会话中心派出的才有） */
  hand: string;
}

export interface ConvTask {
  id: string;
  title: string;
  origin: TaskOrigin;
  project: string;
  /** 执行的先后；内层数组是并行的一组（编排里的多个 worker） */
  chain: (Execution | Execution[])[];
  /** 交给下一位时挂到哪段会话：资料袋会话，或任务里的承载会话；没有会话的任务为空 */
  anchorId?: string;
  /** 资料袋会话（会话中心新建的任务才有）：任务的「你给的」资料都在这里 */
  recordId?: string;
  /** 工作台交办登记 */
  officeId?: string;
  /** 交办登记已完成，或已验收 */
  officeDone?: boolean;
  /** 做完交回什么；选了才需要验收 */
  deliver?: Deliverable[];
}

export interface Sources {
  sessions: Session[];
  agentNames: Record<string, string>;
  teamNames: Record<string, string>;
  officeTasks: OfficeTaskRow[];
  codingTasks: CodingTask[];
  runs: RunDetail[];
  terminals: Terminal[];
  presets: AgentPreset[];
  workspaces: Workspace[];
}

// ——— 执行者 ———
// 编码工具在不同来源里叫法不一样，统一成一个键
const TOOL_KEYS: Record<string, string> = {
  claude: 'cc',
  claude_code: 'cc',
  codex: 'cx',
  codebuddy: 'wb',
  workbuddy: 'wb',
  deepseek: 'ds',
  grok: 'gk',
};
const TOOL_NAMES: Record<string, string> = {
  cc: 'Claude Code',
  cx: 'Codex',
  wb: 'WorkBuddy',
  ds: 'DeepSeek Harness',
  gk: 'Grok',
};
export const KNOWN_TOOLS = ['cc', 'cx', 'wb', 'ds', 'gk'] as const;
export const toolKey = (raw: string) => TOOL_KEYS[raw] ?? raw;
export const toolName = (key: string, presets: AgentPreset[] = []) =>
  TOOL_NAMES[key] ?? presets.find((p) => p.id === key)?.label ?? key;
/** 执行者筛选用的分组：本平台智能体 / 团队都归「ag」 */
export const executorGroup = (executor: string) =>
  executor.startsWith('agent:') || executor.startsWith('team:') ? 'ag' : executor;

// ——— 各来源 → 执行 ———
const n = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) ? v : 0);

export function sessionStatus(s: Pick<Session, 'status' | 'status_reason'>): {
  status: ExecStatus;
  reason: ExecReason;
} {
  const reason = String(s.status_reason ?? '');
  switch (s.status) {
    case 'running':
      return { status: 'running', reason: '' };
    case 'awaiting_confirmation':
      return {
        status: 'waiting',
        reason: reason === 'clarification' || reason === 'agent_awaiting_reply' ? reason : 'tool_confirmation',
      };
    case 'interrupted':
      return reason === 'user_cancelled' ? { status: 'done', reason: 'cancelled' } : { status: 'failed', reason: '' };
    case 'completed':
      return { status: 'done', reason: '' };
    default:
      return { status: 'idle', reason: '' };
  }
}

const bucketOf = (s: Pick<Session, 'archived_at' | 'deleted_at'>): Bucket =>
  s.deleted_at ? 'deleted' : s.archived_at ? 'archived' : 'active';

export function fromSession(s: Session, src: Pick<Sources, 'agentNames' | 'teamNames'>): Execution {
  const { status, reason } = sessionStatus(s);
  const team = s.owner_type === 'team' && s.team_id;
  return {
    id: `session:${s.id}`,
    kind: 'session',
    refId: s.id,
    executor: team ? `team:${s.team_id}` : `agent:${s.agent_id}`,
    executorName: (team ? src.teamNames[s.team_id] : src.agentNames[s.agent_id]) || s.agent_id || s.team_id,
    what: s.title,
    model: s.model || s.provider || '',
    status,
    reason,
    detail: status === 'failed' ? String(s.status_reason ?? '') : s.summary,
    start: s.created_at,
    last: s.last_message_at || s.updated_at || s.created_at,
    turns: n(s.run_count),
    tokens: n(s.total_tokens),
    costUsd: n(s.total_cost_micro_usd) / 1e6,
    bucket: bucketOf(s),
    pinned: Boolean(s.pinned || s.pinned_at),
    agentId: s.agent_id,
    workspaceId: '',
    parentSessionId: '',
    project: '',
    hand: parseMeta(s.metadata_json).cc_hand ?? '',
  };
}

export function fromCodingTask(t: CodingTask, presets: AgentPreset[] = []): Execution {
  const raw = t.status ?? '';
  const status: ExecStatus =
    raw === 'awaiting_approval'
      ? 'waiting'
      : raw === 'failed'
        ? 'failed'
        : raw === 'done' || raw === 'cancelled'
          ? 'done'
          : 'running';
  const key = toolKey(t.agentKey ?? '');
  const prompt = (t.prompt ?? '').trim();
  return {
    id: `bridge:${t.id}`,
    kind: 'bridge',
    refId: t.id ?? '',
    executor: key,
    executorName: toolName(key, presets),
    what: prompt.split('\n')[0]?.slice(0, 80) ?? '',
    model: t.agentKey ?? '',
    status,
    reason: status === 'waiting' ? 'approval' : raw === 'cancelled' ? 'cancelled' : '',
    detail: status === 'failed' ? (t.error ?? '') : t.lastExcerpt || t.summary || '',
    start: t.createdAt ?? '',
    last: t.lastActiveAt || t.completedAt || t.createdAt || '',
    turns: n(t.progressCount),
    tokens: 0,
    costUsd: 0,
    bucket: t.reviewStatus === 'archived' ? 'archived' : 'active',
    pinned: false,
    agentId: '',
    workspaceId: '',
    parentSessionId: t.sessionId ?? '',
    project: t.projectName ?? '',
    // 会话里 LLM 派的（chat）提示是模型写的，不算交接单；工作台 / 会话中心派的才是
    hand: t.origin === 'chat' ? '' : prompt,
  };
}

export function fromTerminal(t: Terminal, src: Pick<Sources, 'presets' | 'workspaces'>): Execution {
  const status: ExecStatus =
    t.status === 'needs_input'
      ? 'waiting'
      : t.status === 'starting' || t.status === 'working'
        ? 'running'
        : t.status === 'failed'
          ? 'failed'
          : t.status === 'idle'
            ? 'idle'
            : 'done';
  const key = toolKey(t.agent);
  const name = toolName(key, src.presets);
  const ws = src.workspaces.find((w) => w.id === t.workspaceId);
  return {
    id: `terminal:${t.id}`,
    kind: 'terminal',
    refId: t.id,
    executor: key,
    executorName: name,
    what: (t.title && t.title !== name ? t.title : '') || ws?.name || name,
    model: '',
    status,
    reason: status === 'waiting' ? 'input' : '',
    // 终端的 preview 是屏幕原样（框线、状态栏），当「最近一句」看是噪音，不用
    detail: '',
    start: t.startedAt,
    last: t.lastActivityAt || t.statusChangedAt || t.startedAt,
    turns: 0,
    tokens: 0,
    costUsd: 0,
    bucket: 'active',
    pinned: false,
    agentId: '',
    workspaceId: t.workspaceId,
    parentSessionId: '',
    project: ws?.name ?? '',
    hand: '',
  };
}

// ——— 拼成任务 ———
const byStart = (a: Execution, b: Execution) => a.start.localeCompare(b.start);
const stepStart = (x: Execution | Execution[]) => (Array.isArray(x) ? (x[0]?.start ?? '') : x.start);

export function buildTasks(src: Sources): ConvTask[] {
  const meta = new Map(src.sessions.map((s) => [s.id, parseMeta(s.metadata_json)]));
  // 资料袋会话不算一次执行：只放资料、挂从它派出的编码任务
  const records = src.sessions.filter((s) => meta.get(s.id)?.cc_role === 'task_record');
  const sessions = new Map(
    src.sessions.filter((s) => meta.get(s.id)?.cc_role !== 'task_record').map((s) => [s.id, fromSession(s, src)]),
  );
  const bridge = src.codingTasks.map((t) => fromCodingTask(t, src.presets));
  const terminals = new Map(
    src.terminals.filter((t) => t.agent && t.agent !== 'shell').map((t) => [t.id, fromTerminal(t, src)]),
  );
  const used = new Set<string>();
  const take = (e: Execution | undefined) => {
    if (!e || used.has(e.id)) return null;
    used.add(e.id);
    return e;
  };
  const kidsOf = (sessionId: string) =>
    bridge
      .filter((b) => b.parentSessionId === sessionId)
      .map(take)
      .filter((e): e is Execution => Boolean(e));
  // 一个会话 + 从它派出去的编码任务
  const withChildren = (session: Execution | null) =>
    session ? [session, ...kidsOf(session.refId)].sort(byStart) : [];
  // 运行台编排：协调者终端 → 各 worker 终端（多个时是并行的一组）
  const runChain = (rd: RunDetail, hand = '') => {
    const coordinator = take(terminals.get(rd.run.coordinatorTerminalId));
    const workers = rd.tasks
      .map((t) => take(terminals.get(t.workerTerminalId)))
      .filter((e): e is Execution => Boolean(e))
      .sort(byStart);
    const chain: ConvTask['chain'] = [];
    if (coordinator) chain.push(hand ? { ...coordinator, hand } : coordinator);
    if (workers.length === 1) chain.push(workers[0]!);
    else if (workers.length > 1) chain.push(workers);
    return chain;
  };

  // 0. 先认领「跟进」：交给下一位时新开的会话（cc_task），以及记在锚点会话上的终端 / 编排（cc_links）
  const followers = [...sessions.values()]
    .filter((e) => meta.get(e.refId)?.cc_task)
    .map(take)
    .filter((e): e is Execution => Boolean(e));
  const linked: { anchor: string; steps: ConvTask['chain'] }[] = [];
  for (const s of src.sessions) {
    for (const link of (meta.get(s.id)?.cc_links ?? []) as CcLink[]) {
      if (link.kind === 'terminal') {
        const e = take(terminals.get(link.id));
        if (e) linked.push({ anchor: s.id, steps: [{ ...e, hand: link.hand }] });
      } else {
        const rd = src.runs.find((r) => r.run.id === link.id);
        const steps = rd ? runChain(rd, link.hand) : [];
        if (steps.length) linked.push({ anchor: s.id, steps });
      }
    }
  }
  const tasks: ConvTask[] = [];
  const officeOf = (sessionId: string) =>
    src.officeTasks.find((o) => o.source === 'delegate' && o.source_ref === sessionId);

  // 1. 会话中心新建的任务：任务 = 资料袋（+ 它的交办登记）；执行 = 从它派出的编码任务 + 后面挂上的跟进
  for (const r of records) {
    const m = meta.get(r.id)!;
    const ot = officeOf(r.id);
    tasks.push({
      id: ot ? `office:${ot.id}` : `record:${r.id}`,
      title: ot?.title || r.title,
      origin: 'office',
      project: '',
      chain: kidsOf(r.id),
      anchorId: r.id,
      recordId: r.id,
      officeId: ot?.id,
      officeDone: ot?.status === 'done' || Boolean(m.cc_reviewed),
      deliver: m.cc_deliver ?? [],
    });
  }

  // 2. 工作台交办登记：任务 = 这条登记；执行 = 承载会话 + 从它派出的编码任务
  for (const ot of src.officeTasks) {
    if (ot.source !== 'delegate' || !ot.source_ref || meta.get(ot.source_ref)?.cc_role === 'task_record') continue;
    const chain = withChildren(take(sessions.get(ot.source_ref)));
    if (chain.length)
      tasks.push({
        id: `office:${ot.id}`,
        title: ot.title,
        origin: 'office',
        project: '',
        chain,
        anchorId: ot.source_ref,
        officeId: ot.id,
        officeDone: ot.status === 'done',
      });
  }

  // 3. 运行台编排：任务 = Run
  for (const rd of src.runs) {
    const chain = runChain(rd);
    if (chain.length) {
      const ws = src.workspaces.find((w) => w.id === rd.run.workspaceId);
      tasks.push({ id: `run:${rd.run.id}`, title: rd.run.objective, origin: 'run', project: ws?.name ?? '', chain });
    }
  }

  // 4. 其余的会话各自成一个任务（带上从它派出的编码任务）
  for (const s of sessions.values()) {
    const chain = withChildren(take(s));
    if (chain.length)
      tasks.push({
        id: `auto:${s.id}`,
        title: s.what || s.executorName,
        origin: 'auto',
        project: '',
        chain,
        anchorId: s.refId,
      });
  }

  // 5. 跟进挂回所在的任务；找不到（锚点不在已加载范围里）就自成一个任务
  const holder = (anchor: string) =>
    anchor
      ? tasks.find((t) => t.anchorId === anchor || flat(t).some((e) => e.kind === 'session' && e.refId === anchor))
      : undefined;
  for (const f of followers) {
    const steps = [f, ...kidsOf(f.refId)];
    const t = holder(meta.get(f.refId)?.cc_task ?? '');
    if (t) t.chain.push(...steps);
    else
      tasks.push({
        id: `auto:${f.id}`,
        title: f.what || f.executorName,
        origin: 'auto',
        project: '',
        chain: steps,
        anchorId: f.refId,
      });
  }
  for (const l of linked) {
    const t = holder(l.anchor);
    if (t) t.chain.push(...l.steps);
    else {
      const first = l.steps.flat()[0]!;
      tasks.push({
        id: `auto:${first.id}`,
        title: first.what || first.executorName,
        origin: 'auto',
        project: first.project,
        chain: l.steps,
      });
    }
  }

  // 6. 其余的编码任务、终端各自成一个任务
  for (const e of [...bridge, ...terminals.values()]) {
    if (!take(e)) continue;
    tasks.push({ id: `auto:${e.id}`, title: e.what || e.executorName, origin: 'auto', project: e.project, chain: [e] });
  }

  // 资料袋建好、还没派出去（或派失败了）的任务没有执行，不显示
  const out = tasks.filter((t) => t.chain.length);
  for (const t of out) {
    t.chain.sort((a, b) => stepStart(a).localeCompare(stepStart(b)));
    if (!t.project) t.project = flat(t).find((e) => e.project)?.project ?? '';
  }
  return out;
}

// ——— 派生 ———
export const flat = (t: ConvTask) => t.chain.flat();

const RANK: Record<ExecStatus, number> = { waiting: 0, running: 1, failed: 2, idle: 3, done: 4 };

/** 选了「交回什么」、交办没完成、最后一次执行做完了 → 等你验收 */
export function needsReview(t: ConvTask): boolean {
  if (!t.deliver?.length || t.officeDone) return false;
  const xs = flat(t);
  if (xs.some((e) => e.status === 'waiting' || e.status === 'running')) return false;
  return [...xs].sort((a, b) => a.last.localeCompare(b.last)).pop()?.status === 'done';
}

/** 有执行在等你 → 等你；有执行在跑 → 进行中；做完等验收 → 等你；否则取最后一次执行的结果 */
export function taskStatus(t: ConvTask): ExecStatus {
  const xs = flat(t);
  if (xs.some((e) => e.status === 'waiting')) return 'waiting';
  if (xs.some((e) => e.status === 'running')) return 'running';
  if (needsReview(t)) return 'waiting';
  return [...xs].sort((a, b) => a.last.localeCompare(b.last)).pop()?.status ?? 'idle';
}
export const taskLast = (t: ConvTask) => flat(t).reduce((max, e) => (e.last > max ? e.last : max), '');
export const taskStart = (t: ConvTask) =>
  flat(t).reduce((min, e) => (!min || (e.start && e.start < min) ? e.start : min), '');
export const isLive = (t: ConvTask) => RANK[taskStatus(t)] < 2;
/** 有一次执行还在用 → 任务算在用；否则全归档了算已归档；否则在回收站 */
export function taskBucket(t: ConvTask): Bucket {
  const xs = flat(t);
  if (xs.some((e) => e.bucket === 'active')) return 'active';
  if (xs.some((e) => e.bucket === 'archived')) return 'archived';
  return 'deleted';
}
export const taskPinned = (t: ConvTask) => flat(t).some((e) => e.pinned);
export const sumOf = (t: ConvTask, k: 'turns' | 'tokens' | 'costUsd') => flat(t).reduce((s, e) => s + e[k], 0);

/** 默认看哪一次执行：在等你或在跑的那次，否则最后一次 */
export function defaultExecution(t: ConvTask): Execution | undefined {
  const xs = flat(t);
  return xs.find((e) => e.status === 'waiting') ?? xs.find((e) => e.status === 'running') ?? xs[xs.length - 1];
}

/** 列表排序：等你 → 进行中 → 其余按最近动静 */
export function sortTasks(tasks: ConvTask[], by: 'last' | 'cost' | 'turns' = 'last'): ConvTask[] {
  const key = (t: ConvTask) => (by === 'last' ? 0 : sumOf(t, by === 'cost' ? 'costUsd' : 'turns'));
  return [...tasks].sort((a, b) => {
    const ra = Math.min(RANK[taskStatus(a)], 2);
    const rb = Math.min(RANK[taskStatus(b)], 2);
    if (ra !== rb && (ra < 2 || rb < 2)) return ra - rb;
    return by === 'last' ? taskLast(b).localeCompare(taskLast(a)) : key(b) - key(a);
  });
}

export type View = 'now' | 'waiting' | 'today' | 'all' | 'pinned' | 'archived' | 'trash';

export function inView(t: ConvTask, view: View, now: Date): boolean {
  const bucket = taskBucket(t);
  if (view === 'archived') return bucket === 'archived';
  if (view === 'trash') return bucket === 'deleted';
  if (bucket !== 'active') return false;
  const status = taskStatus(t);
  const today = sameDay(taskLast(t), now);
  if (view === 'now') return isLive(t) || (status === 'failed' && today);
  if (view === 'waiting') return status === 'waiting';
  if (view === 'today') return today;
  if (view === 'pinned') return taskPinned(t);
  return true;
}

export function sameDay(iso: string, now: Date): boolean {
  if (!iso) return false;
  const d = new Date(iso);
  return d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth() && d.getDate() === now.getDate();
}

/** 搜索：标题、执行者、做了什么、模型、项目、原文摘录（P1 只搜已加载的；全文搜索在 P3） */
export function matches(t: ConvTask, q: string): boolean {
  const needle = q.trim().toLowerCase();
  if (!needle) return true;
  return [t.title, t.project, ...flat(t).flatMap((e) => [e.executorName, e.what, e.model, e.detail])]
    .join('\n')
    .toLowerCase()
    .includes(needle);
}

// ——— 编码工具桥的过程事件 → 对话块 ———
export type BridgeBlock =
  | { kind: 'ai'; text: string }
  | { kind: 'tools'; items: { title: string; text: string }[] }
  | { kind: 'plan' | 'ask' | 'err'; title: string; text: string };

/** 连续的消息块合成一段话，连续的工具调用合成一组；心跳不显示 */
export function groupBridgeEvents(events: { kind?: string; title?: string; text?: string }[]): BridgeBlock[] {
  const out: BridgeBlock[] = [];
  for (const ev of events) {
    const last = out[out.length - 1];
    const title = ev.title ?? '';
    const text = ev.text ?? '';
    switch (ev.kind) {
      case 'agent_message_chunk':
        if (last?.kind === 'ai') last.text += text;
        else out.push({ kind: 'ai', text });
        break;
      case 'tool_call':
        if (last?.kind === 'tools') last.items.push({ title, text });
        else out.push({ kind: 'tools', items: [{ title, text }] });
        break;
      case 'plan':
        out.push({ kind: 'plan', title, text });
        break;
      case 'permission':
        out.push({ kind: 'ask', title, text });
        break;
      case 'error':
        out.push({ kind: 'err', title, text });
        break;
      default:
        break;
    }
  }
  return out;
}
