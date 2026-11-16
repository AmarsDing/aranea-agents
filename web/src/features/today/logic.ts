/**
 * 工作台「今日」页的纯函数：时间线合并、交办状态归类、标题计数。
 * 待办的口径与排序在服务端（/v1/inbox），这里只做展示层的派生。
 */
import type { InboxItemRow } from '../approval/api';
import type { OfficeEventRow } from '../office/types';

// 展示组件不直接依赖 api 模块，从这里取收件箱类型。
export type { InboxAction, InboxItemRow } from '../approval/api';

// ——— 时间 ———

export function dayBounds(now: Date): { start: Date; end: Date } {
  const start = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const end = new Date(start.getTime() + 24 * 60 * 60 * 1000);
  return { start, end };
}

function parseMs(iso: string | undefined): number {
  if (!iso) return NaN;
  return new Date(iso).getTime();
}

function inDay(ms: number, now: Date): boolean {
  const { start, end } = dayBounds(now);
  return Number.isFinite(ms) && ms >= start.getTime() && ms < end.getTime();
}

/** HH:mm（本地时区）；无效时间返回空串。 */
export function clock(iso: string): string {
  const ms = parseMs(iso);
  if (!Number.isFinite(ms)) return '';
  const d = new Date(ms);
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}

// ——— 收件箱标题计数 ———

export type InboxHeadline = {
  /** 服务端总数（含未取回的条目） */
  total: number;
  /** 已取回条目中今天到期或已逾期的 */
  dueToday: number;
  /** 已取回条目中高风险的 */
  highRisk: number;
};

export function inboxHeadline(rows: readonly InboxItemRow[], total: number, now: Date): InboxHeadline {
  const { end } = dayBounds(now);
  let dueToday = 0;
  let highRisk = 0;
  for (const row of rows) {
    const due = parseMs(row.due_at);
    if (Number.isFinite(due) && due < end.getTime()) dueToday += 1;
    if (row.risk_level === 'high') highRisk += 1;
  }
  return { total: Math.max(total, rows.length), dueToday, highRisk };
}

/** 条目的主动作：首个非驳回动作；只能跳转处理的条目返回空串。 */
export function primaryAction(row: InboxItemRow): InboxItemRow['actions'][number] | '' {
  return row.actions.find((a) => a !== 'reject') ?? '';
}

/** 同一来源里可一键批量批准的低风险条目（设计 §5.2：同类低风险一键批量）。 */
export function batchableLowRisk(rows: readonly InboxItemRow[], source: string): InboxItemRow[] {
  return rows.filter((r) => r.source === source && r.risk_level === 'low' && r.actions.includes('approve'));
}

// ——— 今日时间线 ———

export type TimelineKind = 'event' | 'deadline' | 'cron';

export type TimelineEntry = {
  id: string;
  kind: TimelineKind;
  at: string;
  endAt: string;
  title: string;
  /** 已过去（日程以结束时间计） */
  past: boolean;
  to: string;
  /** deadline 条目对应的收件箱条目 id（点击后在列表中选中） */
  inboxId: string;
};

export type CronNextRun = { id: string; name: string; nextRunAt: string };

/** 日程、待办截止时间、Agent 定时任务合并到同一条当天时间轴，按时间升序。 */
export function buildTimeline(input: {
  events: readonly OfficeEventRow[];
  inbox: readonly InboxItemRow[];
  cron: readonly CronNextRun[];
  now: Date;
}): TimelineEntry[] {
  const { now } = input;
  const nowMs = now.getTime();
  const out: TimelineEntry[] = [];
  for (const ev of input.events) {
    const start = parseMs(ev.start_at);
    if (!inDay(start, now)) continue;
    const end = parseMs(ev.end_at);
    out.push({
      id: `event:${ev.id}`,
      kind: 'event',
      at: ev.start_at,
      endAt: ev.end_at,
      title: ev.title || ev.task_title,
      past: (Number.isFinite(end) ? end : start) < nowMs,
      to: '/schedule',
      inboxId: '',
    });
  }
  for (const row of input.inbox) {
    const due = parseMs(row.due_at);
    if (!inDay(due, now)) continue;
    out.push({
      id: `deadline:${row.id}`,
      kind: 'deadline',
      at: row.due_at,
      endAt: '',
      title: row.title || row.summary,
      past: due < nowMs,
      to: '',
      inboxId: row.id,
    });
  }
  for (const job of input.cron) {
    const at = parseMs(job.nextRunAt);
    if (!inDay(at, now) || at < nowMs) continue;
    out.push({
      id: `cron:${job.id}`,
      kind: 'cron',
      at: job.nextRunAt,
      endAt: '',
      title: job.name,
      past: false,
      to: '/cron',
      inboxId: '',
    });
  }
  return out.sort((a, b) => parseMs(a.at) - parseMs(b.at) || a.id.localeCompare(b.id));
}

// ——— 交办中 ———

export type DelegatedState = 'stuck' | 'review' | 'running';

export type DelegatedRow = {
  id: string;
  title: string;
  agentKey: string;
  state: DelegatedState;
  /** 状态对应的时间：卡住/待验收取完成时间，进行中取最近活跃时间 */
  at: string;
};

/** 交办任务的最小字段（与 agentbridge CodingTask 同名，便于直接传入）。 */
export type DelegatedTaskInput = {
  id?: string;
  agentKey?: string;
  projectName?: string;
  prompt?: string;
  status?: string;
  summary?: string;
  createdAt?: string;
  completedAt?: string;
  lastActiveAt?: string;
  reviewStatus?: string;
};

const ACTIVE = new Set(['dispatched', 'running', 'awaiting_approval', 'cancelling']);
/** 失败或待验收的任务只在这个窗口内算「还在我手上」，更早的不再占版面。 */
export const DELEGATED_WINDOW_MS = 7 * 24 * 60 * 60 * 1000;
const STATE_ORDER: Record<DelegatedState, number> = { stuck: 0, review: 1, running: 2 };

function firstLine(text: string | undefined, max = 60): string {
  const line = (text ?? '').trim().split(/\r?\n/)[0] ?? '';
  return line.length > max ? `${line.slice(0, max)}…` : line;
}

/**
 * 派出去的任务归成三态：卡住了（失败未验收）、待验收（完成未验收）、进行中。
 * 排序：卡住的最前，其次待验收，再进行中；同态按时间新到旧。
 */
export function classifyDelegated(tasks: readonly DelegatedTaskInput[], now: Date): DelegatedRow[] {
  const nowMs = now.getTime();
  const out: DelegatedRow[] = [];
  for (const task of tasks) {
    const id = task.id ?? '';
    if (!id) continue;
    const status = task.status ?? '';
    const reviewed = Boolean(task.reviewStatus);
    let state: DelegatedState | null = null;
    let at = '';
    if (ACTIVE.has(status)) {
      state = 'running';
      at = task.lastActiveAt || task.createdAt || '';
    } else if (!reviewed && (status === 'failed' || status === 'done')) {
      at = task.completedAt || task.createdAt || '';
      const ms = parseMs(at);
      if (Number.isFinite(ms) && nowMs - ms > DELEGATED_WINDOW_MS) continue;
      state = status === 'failed' ? 'stuck' : 'review';
    }
    if (!state) continue;
    out.push({
      id,
      title: firstLine(task.summary) || firstLine(task.prompt) || task.projectName || id,
      agentKey: task.agentKey ?? '',
      state,
      at,
    });
  }
  return out.sort((a, b) => STATE_ORDER[a.state] - STATE_ORDER[b.state] || (parseMs(b.at) || 0) - (parseMs(a.at) || 0));
}
