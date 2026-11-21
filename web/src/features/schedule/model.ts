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

// ---------- 周视图（2026-11-20 补：对照 Google / Outlook / 飞书日历，月视图之外最常用的就是周视图） ----------

/** 这一天所在那周的周一 0 点。 */
export function weekStart(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() - ((d.getDay() + 6) % 7));
}

/** 一周 7 天的 key（周一起）。 */
export function weekDays(start: Date): string[] {
  return Array.from({ length: 7 }, (_, i) => dayKey(addDays(start, i)));
}

/**
 * 一天里时间段的并排布局：互相重叠的一簇平分宽度，每段落在第一个空着的「道」里。
 * 入参按开始时间排好；返回同序的 { lane, lanes }（第几道、这一簇共几道）。
 */
export function layoutLanes(items: Array<{ start: number; end: number }>): Array<{ lane: number; lanes: number }> {
  const out: Array<{ lane: number; lanes: number }> = [];
  let cluster: number[] = []; // 当前这簇里各段的下标
  let laneEnds: number[] = []; // 每道最后一段的结束时间
  let clusterEnd = -Infinity;
  const close = () => {
    for (const i of cluster) out[i]!.lanes = laneEnds.length;
    cluster = [];
    laneEnds = [];
  };
  items.forEach((it, i) => {
    if (it.start >= clusterEnd) close();
    let lane = laneEnds.findIndex((end) => end <= it.start);
    if (lane < 0) lane = laneEnds.push(it.end) - 1;
    else laneEnds[lane] = it.end;
    out[i] = { lane, lanes: 1 };
    cluster.push(i);
    clusterEnd = Math.max(clusterEnd, it.end);
  });
  close();
  return out;
}

/** 拖动改期：保持时长，开始挪到 start。返回 ISO 起止。 */
export function movedRange(ev: Pick<OfficeEventRow, 'start_at' | 'end_at'>, start: Date): [string, string] {
  const dur = Date.parse(ev.end_at) - Date.parse(ev.start_at);
  return [start.toISOString(), new Date(start.getTime() + Math.max(dur, 0)).toISOString()];
}

/** 改到另一天、时刻不变（月视图拖到别的格子、周视图拖到全天行）。 */
export function sameTimeOn(ev: Pick<OfficeEventRow, 'start_at'>, day: string): Date {
  const s = new Date(ev.start_at);
  const d = parseDayKey(day);
  return new Date(d.getFullYear(), d.getMonth(), d.getDate(), s.getHours(), s.getMinutes());
}

// ---------- 重复日程（RRULE 子集，和后端 internal/biz/office/recur.go 对应） ----------

export type RepeatFreq = '' | 'DAILY' | 'WEEKDAYS' | 'WEEKLY' | 'MONTHLY' | 'YEARLY';
export type RepeatEnd = { kind: 'never' } | { kind: 'until'; day: string } | { kind: 'count'; n: number };

const WD = ['SU', 'MO', 'TU', 'WE', 'TH', 'FR', 'SA'];

/** 表单 → RRULE。weekday 是开始那天的星期（0=周日），每周重复用它。 */
export function buildRRule(freq: RepeatFreq, end: RepeatEnd, weekday: number): string {
  if (!freq) return '';
  const base =
    freq === 'WEEKDAYS'
      ? 'FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR'
      : freq === 'WEEKLY'
        ? `FREQ=WEEKLY;BYDAY=${WD[weekday]}`
        : `FREQ=${freq}`;
  if (end.kind === 'until' && end.day) return `${base};UNTIL=${end.day.replace(/-/g, '')}`;
  if (end.kind === 'count' && end.n > 0) return `${base};COUNT=${end.n}`;
  return base;
}

/** RRULE → 表单（认不出的部件忽略，按最接近的给）。 */
export function parseRepeat(rrule: string): { freq: RepeatFreq; end: RepeatEnd } {
  const parts = new Map(
    rrule
      .replace(/^RRULE:/, '')
      .split(';')
      .filter(Boolean)
      .map((p) => p.split('=') as [string, string]),
  );
  const f = parts.get('FREQ') ?? '';
  const byday = parts.get('BYDAY') ?? '';
  const freq: RepeatFreq =
    f === 'WEEKLY' && byday === 'MO,TU,WE,TH,FR'
      ? 'WEEKDAYS'
      : ((['DAILY', 'WEEKLY', 'MONTHLY', 'YEARLY'] as const).find((x) => x === f) ?? '');
  const until = parts.get('UNTIL');
  const count = Number(parts.get('COUNT') ?? 0);
  const end: RepeatEnd = until
    ? { kind: 'until', day: `${until.slice(0, 4)}-${until.slice(4, 6)}-${until.slice(6, 8)}` }
    : count > 0
      ? { kind: 'count', n: count }
      : { kind: 'never' };
  return { freq, end };
}

/** 跳过的日期里加一天（去重）。 */
export function addExdate(exdates: string, day: string): string {
  const set = new Set(
    exdates
      .split(',')
      .map((d) => d.trim())
      .filter(Boolean),
  );
  set.add(day);
  return [...set].sort().join(',');
}

// ---------- 导入 .ics ----------

export type IcsEvent = {
  title: string;
  note: string;
  location: string;
  url: string;
  start: Date;
  end: Date;
  allDay: boolean;
  rrule: string;
};

function icsDate(v: string, params: string): { at: Date; allDay: boolean } | null {
  const m = /^(\d{4})(\d{2})(\d{2})(?:T(\d{2})(\d{2})(\d{2})(Z)?)?$/.exec(v.trim());
  if (!m) return null;
  const [, y, mo, d, h, mi, s, z] = m;
  if (!h || /VALUE=DATE(?!-)/.test(params)) return { at: new Date(+y!, +mo! - 1, +d!), allDay: true };
  // shortcut: 带 TZID 的按本机时间理解，不换算别的时区；导入跨时区日历时会差几个小时
  const at = z ? new Date(Date.UTC(+y!, +mo! - 1, +d!, +h!, +mi!, +s!)) : new Date(+y!, +mo! - 1, +d!, +h!, +mi!, +s!);
  return { at, allDay: false };
}

function icsText(v: string): string {
  return v.replace(/\\n/gi, '\n').replace(/\\([,;\\])/g, '$1');
}

/** 解析 .ics 里的 VEVENT（展开折行；没有结束时间的按 1 小时 / 1 天）。 */
export function parseIcs(text: string): IcsEvent[] {
  const lines = text.replace(/\r?\n[ \t]/g, '').split(/\r?\n/);
  const out: IcsEvent[] = [];
  let cur: Record<string, { v: string; p: string }> | null = null;
  for (const line of lines) {
    if (line === 'BEGIN:VEVENT') cur = {};
    else if (line === 'END:VEVENT' && cur) {
      const st = cur.DTSTART && icsDate(cur.DTSTART.v, cur.DTSTART.p);
      if (st && cur.SUMMARY) {
        const en = cur.DTEND && icsDate(cur.DTEND.v, cur.DTEND.p);
        const fallback = new Date(st.at.getTime() + (st.allDay ? 86_400_000 : 3_600_000));
        out.push({
          title: icsText(cur.SUMMARY.v),
          note: icsText(cur.DESCRIPTION?.v ?? ''),
          location: icsText(cur.LOCATION?.v ?? ''),
          url: cur.URL?.v ?? '',
          start: st.at,
          end: en && en.at > st.at ? en.at : fallback,
          allDay: st.allDay,
          rrule: cur.RRULE?.v ?? '',
        });
      }
      cur = null;
    } else if (cur) {
      const i = line.indexOf(':');
      if (i < 0) continue;
      const [name = '', ...params] = line.slice(0, i).split(';');
      cur[name.toUpperCase()] = { v: line.slice(i + 1), p: params.join(';').toUpperCase() };
    }
  }
  return out;
}

// ---------- 农历与节日 ----------

const LUNAR_DAY = [
  '初一',
  '初二',
  '初三',
  '初四',
  '初五',
  '初六',
  '初七',
  '初八',
  '初九',
  '初十',
  '十一',
  '十二',
  '十三',
  '十四',
  '十五',
  '十六',
  '十七',
  '十八',
  '十九',
  '二十',
  '廿一',
  '廿二',
  '廿三',
  '廿四',
  '廿五',
  '廿六',
  '廿七',
  '廿八',
  '廿九',
  '三十',
];
const LUNAR_FEST: Record<string, string> = {
  '正月-1': '春节',
  '正月-15': '元宵',
  '二月-2': '龙抬头',
  '五月-5': '端午',
  '七月-7': '七夕',
  '七月-15': '中元',
  '八月-15': '中秋',
  '九月-9': '重阳',
  '腊月-8': '腊八',
};
const SOLAR_FEST: Record<string, string> = {
  '1-1': '元旦',
  '3-8': '妇女节',
  '4-5': '清明',
  '5-1': '劳动节',
  '5-4': '青年节',
  '6-1': '儿童节',
  '7-1': '建党节',
  '8-1': '建军节',
  '9-10': '教师节',
  '10-1': '国庆节',
};

let lunarFmt: Intl.DateTimeFormat | null | undefined;
function lunarParts(d: Date): { month: string; day: number } | null {
  if (lunarFmt === undefined) {
    try {
      lunarFmt = new Intl.DateTimeFormat('zh-CN-u-ca-chinese', { month: 'long', day: 'numeric' });
    } catch {
      lunarFmt = null;
    }
  }
  if (!lunarFmt) return null;
  const parts = lunarFmt.formatToParts(d);
  const month = parts.find((p) => p.type === 'month')?.value ?? '';
  const day = Number(parts.find((p) => p.type === 'day')?.value);
  return month && day ? { month, day } : null;
}

/**
 * 月格里的小字：节日优先（含除夕），其次农历日（初一显示月份）。浏览器不支持农历时只有公历节日。
 * shortcut: 清明按 4 月 5 日近似（实际 4 月 4–6 日浮动）；法定调休没有数据源，不标。
 */
export function lunarLabel(d: Date): { text: string; festival: boolean } {
  const solar = SOLAR_FEST[`${d.getMonth() + 1}-${d.getDate()}`];
  const lp = lunarParts(d);
  if (lp) {
    const fest = LUNAR_FEST[`${lp.month}-${lp.day}`];
    if (fest) return { text: fest, festival: true };
    const next = lunarParts(new Date(d.getFullYear(), d.getMonth(), d.getDate() + 1));
    if (next && next.month === '正月' && next.day === 1) return { text: '除夕', festival: true };
  }
  if (solar) return { text: solar, festival: true };
  if (!lp) return { text: '', festival: false };
  return { text: lp.day === 1 ? lp.month : (LUNAR_DAY[lp.day - 1] ?? ''), festival: false };
}
