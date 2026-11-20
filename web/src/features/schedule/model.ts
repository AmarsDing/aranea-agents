// 日程 P1 页内纯逻辑（设计 docs/aranea-agents/design/2026-11-17-三大模块重设计/08-日程.md §3）。
// 页面只管拉数据和交互，月格怎么排、哪天算逾期、一格放几行都在这里，方便单测。

import type { InboxItemRow } from '../approval/api';
import type { OfficeEventRow, OfficeEventStatus } from '../office/types';

const DAY_MS = 86_400_000;
/** 结束后再过 30 分钟还没完成才算逾期（08 §6 S3）。 */
const OVERDUE_GRACE_MS = 30 * 60_000;
/** 一格最多 3 行，超出时留 2 行 + 「+N 项」。 */
const CELL_LINES = 3;

export type DisplayStatus = OfficeEventStatus | 'overdue';

/** AI 排期草案里的一条（还没进日程）。一份草案只能整份采纳或不要。 */
export type DraftEntry = {
  key: string;
  inboxId: string;
  index: number;
  total: number;
  title: string;
  note: string;
  start_at: string;
  end_at: string;
  reminder_minutes: number;
  reject_requires_note: boolean;
};

export type CellLine =
  | { type: 'plan'; event: OfficeEventRow; status: DisplayStatus }
  | { type: 'log'; count: number; title: string }
  | { type: 'ai'; count: number };

export type DayCell = {
  key: string;
  day: number;
  inMonth: boolean;
  isToday: boolean;
  weekend: boolean;
  lines: CellLine[];
  more: number;
};

export type Visible = { plan: boolean; log: boolean; ai: boolean };

export function dayKey(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** 'YYYY-MM-DD' → 本地 0 点。 */
export function parseDayKey(key: string): Date {
  const [y, m, d] = key.split('-').map(Number);
  return new Date(y ?? 1970, (m ?? 1) - 1, d ?? 1);
}

/** 月格起点：周一开始，6 行 × 7 列共 42 格。 */
export function gridStart(month: Date): Date {
  const first = new Date(month.getFullYear(), month.getMonth(), 1);
  return new Date(first.getFullYear(), first.getMonth(), 1 - ((first.getDay() + 6) % 7));
}

function addDays(d: Date, n: number): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
}

/** 按「和这一天有交集」分桶（跨天的多天都可见；结束时间不含）。 */
export function bucketByDay<T>(items: T[], range: (x: T) => [string, string]): Map<string, T[]> {
  const map = new Map<string, T[]>();
  for (const item of items) {
    const [s, e] = range(item);
    const start = new Date(s);
    const end = new Date(e);
    if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) continue;
    let cur = new Date(start.getFullYear(), start.getMonth(), start.getDate());
    // 起止相同（零时长）也落在开始那天。
    do {
      const k = dayKey(cur);
      const arr = map.get(k);
      if (arr) arr.push(item);
      else map.set(k, [item]);
      cur = addDays(cur, 1);
    } while (cur.getTime() < end.getTime());
  }
  return map;
}

export function isOverdue(ev: OfficeEventRow, now: Date): boolean {
  if (ev.kind !== 'plan' || ev.status === 'done' || ev.status === 'cancelled') return false;
  const end = new Date(ev.end_at);
  if (Number.isNaN(end.getTime())) return false;
  if (ev.all_day) {
    // 全天的：过了最后一天才算（结束时间不含，取它前一刻所在那天的 24 点）。
    const lastDay = new Date(end.getTime() - 1);
    return (
      now.getTime() >= addDays(new Date(lastDay.getFullYear(), lastDay.getMonth(), lastDay.getDate()), 1).getTime()
    );
  }
  return now.getTime() > end.getTime() + OVERDUE_GRACE_MS;
}

export function displayStatus(ev: OfficeEventRow, now: Date): DisplayStatus {
  return isOverdue(ev, now) ? 'overdue' : ev.status;
}

/** 状态按钮点一下循环：未开始 → 进行中 → 已完成 → 未开始；已取消点一下回到未开始。 */
export function nextStatus(s: OfficeEventStatus): OfficeEventStatus {
  return s === 'todo' ? 'doing' : s === 'doing' ? 'done' : 'todo';
}

/** 一天里的排序：全天的在前，再按开始时间。 */
export function sortDay(list: OfficeEventRow[]): OfficeEventRow[] {
  return list.slice().sort((a, b) => Number(b.all_day) - Number(a.all_day) || a.start_at.localeCompare(b.start_at));
}

export function cellLines(
  plans: OfficeEventRow[],
  logs: OfficeEventRow[],
  drafts: DraftEntry[],
  show: Visible,
  now: Date,
): { lines: CellLine[]; more: number } {
  const all: CellLine[] = [];
  if (show.plan) for (const ev of sortDay(plans)) all.push({ type: 'plan', event: ev, status: displayStatus(ev, now) });
  if (show.log && logs.length)
    all.push({ type: 'log', count: logs.length, title: logs.length === 1 ? (logs[0]?.title ?? '') : '' });
  if (show.ai && drafts.length) all.push({ type: 'ai', count: drafts.length });
  if (all.length <= CELL_LINES) return { lines: all, more: 0 };
  return { lines: all.slice(0, CELL_LINES - 1), more: all.length - (CELL_LINES - 1) };
}

export function monthCells(
  month: Date,
  events: OfficeEventRow[],
  drafts: DraftEntry[],
  show: Visible,
  now: Date,
): DayCell[] {
  const byDay = bucketByDay(events, (e) => [e.start_at, e.end_at]);
  const draftsByDay = bucketByDay(drafts, (d) => [d.start_at, d.end_at]);
  const start = gridStart(month);
  const todayKey = dayKey(now);
  const cells: DayCell[] = [];
  for (let i = 0; i < 42; i++) {
    const d = addDays(start, i);
    const key = dayKey(d);
    const dayEvents = byDay.get(key) ?? [];
    const { lines, more } = cellLines(
      dayEvents.filter((e) => e.kind === 'plan'),
      dayEvents.filter((e) => e.kind === 'log'),
      draftsByDay.get(key) ?? [],
      show,
      now,
    );
    cells.push({
      key,
      day: d.getDate(),
      inMonth: d.getMonth() === month.getMonth(),
      isToday: key === todayKey,
      weekend: d.getDay() === 0 || d.getDay() === 6,
      lines,
      more,
    });
  }
  return cells;
}

function inMonth(iso: string, month: Date): boolean {
  const d = new Date(iso);
  return d.getFullYear() === month.getFullYear() && d.getMonth() === month.getMonth();
}

/** 顶上一行：本月计划数 / 已完成 / 逾期 / 工作记录数（按开始时间归月；工作内容不计入完成率，08 §6 S4）。 */
export function monthStats(events: OfficeEventRow[], month: Date, now: Date) {
  const mine = events.filter((e) => inMonth(e.start_at, month));
  const plans = mine.filter((e) => e.kind === 'plan' && e.status !== 'cancelled');
  return {
    plans: plans.length,
    done: plans.filter((e) => e.status === 'done').length,
    overdue: plans.filter((e) => isOverdue(e, now)).length,
    logs: mine.filter((e) => e.kind === 'log').length,
  };
}

/** 选月面板：某年 12 个月各有多少计划（不含已取消）。 */
export function monthPlanCounts(events: OfficeEventRow[], year: number): number[] {
  const out = Array.from({ length: 12 }, () => 0);
  for (const e of events) {
    if (e.kind !== 'plan' || e.status === 'cancelled') continue;
    const d = new Date(e.start_at);
    if (d.getFullYear() === year) out[d.getMonth()] = (out[d.getMonth()] ?? 0) + 1;
  }
  return out;
}

/** 审批中心的排期草案 → 一条条可铺到月历上的建议（context_json.items，见 source_schedule_draft.go）。 */
export function parseDrafts(rows: InboxItemRow[]): DraftEntry[] {
  const out: DraftEntry[] = [];
  for (const row of rows) {
    if (row.source !== 'schedule_draft') continue;
    let items: unknown[] = [];
    try {
      const ctx = JSON.parse(row.context_json || '{}') as { items?: unknown };
      items = Array.isArray(ctx.items) ? ctx.items : [];
    } catch {
      continue;
    }
    items.forEach((raw, index) => {
      const it = (raw ?? {}) as Record<string, unknown>;
      const start = String(it.start_at ?? '');
      if (!start || Number.isNaN(new Date(start).getTime())) return;
      out.push({
        key: `${row.id}#${index}`,
        inboxId: row.id,
        index,
        total: items.length,
        title: String(it.title ?? ''),
        note: String(it.note ?? ''),
        start_at: start,
        end_at: String(it.end_at ?? start),
        reminder_minutes: Number(it.reminder_minutes ?? 0),
        reject_requires_note: row.reject_requires_note,
      });
    });
  }
  return out;
}

/** 结束那天比开始那天晚几天（编辑时保住跨天的跨度；全天的结束不含，所以单日全天是 1）。 */
export function endOffsetDays(startISO: string, endISO: string): number {
  const s = new Date(startISO);
  const e = new Date(endISO);
  if (Number.isNaN(s.getTime()) || Number.isNaN(e.getTime())) return 0;
  const a = new Date(s.getFullYear(), s.getMonth(), s.getDate());
  const b = new Date(e.getFullYear(), e.getMonth(), e.getDate());
  return Math.max(0, Math.round((b.getTime() - a.getTime()) / DAY_MS));
}

/** 编辑面板里的日期 + 时间 → 起止 ISO。全天：开始那天 0 点到 endOffset 天后 0 点（结束不含）。 */
export function rangeFromForm(f: {
  date: string;
  start: string;
  end: string;
  allDay: boolean;
  endOffset?: number;
}): [string, string] | null {
  const day = parseDayKey(f.date);
  if (Number.isNaN(day.getTime()) || !/^\d{4}-\d{2}-\d{2}$/.test(f.date)) return null;
  const offset = f.endOffset ?? 0;
  if (f.allDay) return [day.toISOString(), addDays(day, Math.max(1, offset)).toISOString()];
  const at = (hm: string, plusDays: number) => {
    const [h, m] = hm.split(':').map(Number);
    if (h === undefined || m === undefined || Number.isNaN(h) || Number.isNaN(m)) return null;
    return new Date(day.getFullYear(), day.getMonth(), day.getDate() + plusDays, h, m);
  };
  const s = at(f.start, 0);
  let e = at(f.end, offset);
  if (!s || !e) return null;
  // 同一天里结束早于开始 = 跨到第二天（比如 22:00–01:00）。
  if (offset === 0 && e.getTime() < s.getTime()) e = at(f.end, 1);
  if (!e || e.getTime() <= s.getTime()) return null;
  return [s.toISOString(), e.toISOString()];
}

export function hm(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}
