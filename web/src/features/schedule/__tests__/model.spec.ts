import { describe, expect, it } from 'vitest';
import type { InboxItemRow } from '../../approval/api';
import type { OfficeEventRow } from '../../office/types';
import {
  bucketByDay,
  cellLines,
  dayKey,
  displayStatus,
  endOffsetDays,
  gridStart,
  addExdate,
  buildRRule,
  layoutLanes,
  lunarLabel,
  parseIcs,
  parseRepeat,
  monthCells,
  monthPlanCounts,
  monthStats,
  nextStatus,
  parseDrafts,
  movedRange,
  rangeFromForm,
  sameTimeOn,
  weekDays,
  weekStart,
} from '../model';

const local = (y: number, m: number, d: number, h = 0, min = 0) => new Date(y, m - 1, d, h, min).toISOString();

function ev(over: Partial<OfficeEventRow> = {}): OfficeEventRow {
  return {
    id: 'e1',
    task_ref: '',
    task_title: '',
    title: '走查',
    note: '',
    start_at: local(2026, 11, 19, 9, 30),
    end_at: local(2026, 11, 19, 10, 30),
    timezone: 'Asia/Shanghai',
    reminder_minutes: 0,
    source: 'manual',
    kind: 'plan',
    status: 'todo',
    all_day: false,
    outcome: '',
    location: '',
    url: '',
    color: '',
    rrule: '',
    exdates: '',
    recurrence_day: '',
    ...over,
  };
}

const ALL = { plan: true, log: true, ai: true };

describe('schedule model', () => {
  it('月格从周一开始，2026-11 第一格是 10-26', () => {
    expect(dayKey(gridStart(new Date(2026, 10, 1)))).toBe('2026-10-26');
    const cells = monthCells(new Date(2026, 10, 1), [], [], ALL, new Date(2026, 10, 19, 12));
    expect(cells).toHaveLength(42);
    expect(cells.find((c) => c.isToday)?.key).toBe('2026-11-19');
    expect(cells.find((c) => c.key === '2026-11-21')?.weekend).toBe(true);
    expect(cells[0]?.inMonth).toBe(false);
  });

  it('跨天按交集分桶，结束时间不含；全天两天的出差落两格', () => {
    const trip = ev({ all_day: true, start_at: local(2026, 11, 27), end_at: local(2026, 11, 29) });
    const map = bucketByDay([trip], (e) => [e.start_at, e.end_at]);
    expect([...map.keys()]).toEqual(['2026-11-27', '2026-11-28']);
    const zero = ev({ start_at: local(2026, 11, 3, 10), end_at: local(2026, 11, 3, 10) });
    expect([...bucketByDay([zero], (e) => [e.start_at, e.end_at]).keys()]).toEqual(['2026-11-03']);
  });

  it('逾期：结束过 30 分钟仍未完成；全天过了当天；完成/取消/工作内容不算', () => {
    const now = new Date(2026, 10, 19, 10, 59);
    expect(displayStatus(ev(), now)).toBe('todo');
    expect(displayStatus(ev(), new Date(2026, 10, 19, 11, 1))).toBe('overdue');
    expect(displayStatus(ev({ status: 'done' }), new Date(2026, 10, 20))).toBe('done');
    expect(displayStatus(ev({ kind: 'log' }), new Date(2026, 10, 20))).toBe('todo');
    const allDay = ev({ all_day: true, start_at: local(2026, 11, 19), end_at: local(2026, 11, 20) });
    expect(displayStatus(allDay, new Date(2026, 10, 19, 23, 59))).toBe('todo');
    expect(displayStatus(allDay, new Date(2026, 10, 20, 0, 0))).toBe('overdue');
  });

  it('一格最多 3 行：计划逐条，工作内容合成一行，AI 建议合成一行；多了留 2 行 + N', () => {
    const now = new Date(2026, 10, 19, 8);
    const plans = [ev({ id: 'a' }), ev({ id: 'b', all_day: true })];
    const one = cellLines(plans, [ev({ id: 'l', kind: 'log', title: '部署' })], [], ALL, now);
    expect(one.more).toBe(0);
    expect(one.lines.map((l) => l.type)).toEqual(['plan', 'plan', 'log']);
    expect(one.lines[0]?.type === 'plan' && one.lines[0].event.id).toBe('b'); // 全天在前
    expect(one.lines[2]).toEqual({ type: 'log', count: 1, title: '部署' });
    const many = cellLines([...plans, ev({ id: 'c' })], [ev({ kind: 'log' }), ev({ kind: 'log' })], [], ALL, now);
    expect(many.lines).toHaveLength(2);
    expect(many.more).toBe(2);
    const onlyLogs = cellLines(plans, [ev({ kind: 'log' })], [], { plan: false, log: true, ai: true }, now);
    expect(onlyLogs.lines.map((l) => l.type)).toEqual(['log']);
  });

  it('本月统计只按开始月份算，已取消不计计划，工作内容单独计数', () => {
    const now = new Date(2026, 10, 25);
    const rows = [
      ev({ id: 'a', status: 'done' }),
      ev({ id: 'b' }), // 11-19 结束，到 25 号已逾期
      ev({ id: 'c', status: 'cancelled' }),
      ev({ id: 'd', kind: 'log' }),
      ev({ id: 'e', start_at: local(2026, 12, 1, 9), end_at: local(2026, 12, 1, 10) }),
    ];
    expect(monthStats(rows, new Date(2026, 10, 1), now)).toEqual({ plans: 2, done: 1, overdue: 1, logs: 1 });
    const counts = monthPlanCounts(rows, 2026);
    expect(counts[10]).toBe(2);
    expect(counts[11]).toBe(1);
  });

  it('状态循环 未开始 → 进行中 → 已完成 → 未开始', () => {
    expect(nextStatus('todo')).toBe('doing');
    expect(nextStatus('doing')).toBe('done');
    expect(nextStatus('done')).toBe('todo');
    expect(nextStatus('cancelled')).toBe('todo');
  });

  it('解析审批中心排期草案的 items；坏 JSON 和没时间的条目跳过', () => {
    const base = {
      kind: 'schedule_draft',
      title: '',
      summary: '',
      risk_level: '' as const,
      requester: '',
      agent_key: '',
      created_at: '',
      due_at: '',
      actions: [],
      link: '',
    };
    const rows = [
      {
        ...base,
        id: 'schedule_draft:d1',
        source: 'schedule_draft',
        reject_requires_note: true,
        context_json: JSON.stringify({
          items: [
            { title: '补测试', start_at: local(2026, 11, 20, 9, 30), end_at: local(2026, 11, 20, 11) },
            { title: '没时间' },
          ],
        }),
      },
      { ...base, id: 'x', source: 'schedule_draft', reject_requires_note: false, context_json: '{bad' },
      { ...base, id: 'y', source: 'task_review', reject_requires_note: false, context_json: '{}' },
    ] satisfies InboxItemRow[];
    const drafts = parseDrafts(rows);
    expect(drafts).toHaveLength(1);
    expect(drafts[0]).toMatchObject({
      inboxId: 'schedule_draft:d1',
      index: 0,
      total: 2,
      title: '补测试',
      reject_requires_note: true,
    });
  });

  it('编辑表单 → 起止：全天到次日 0 点；结束早于开始跨天；起止相同无效', () => {
    expect(rangeFromForm({ date: '2026-11-19', start: '', end: '', allDay: true })).toEqual([
      local(2026, 11, 19),
      local(2026, 11, 20),
    ]);
    expect(rangeFromForm({ date: '2026-11-19', start: '22:00', end: '01:00', allDay: false })).toEqual([
      local(2026, 11, 19, 22),
      local(2026, 11, 20, 1),
    ]);
    expect(rangeFromForm({ date: '2026-11-19', start: '09:00', end: '09:00', allDay: false })).toBeNull();
    // 编辑跨天的不丢跨度：两天的出差、隔天结束的
    const trip = [local(2026, 11, 27), local(2026, 11, 29)] as const;
    expect(endOffsetDays(...trip)).toBe(2);
    expect(rangeFromForm({ date: '2026-11-27', start: '', end: '', allDay: true, endOffset: 2 })).toEqual(trip);
    expect(rangeFromForm({ date: '2026-11-19', start: '09:00', end: '18:00', allDay: false, endOffset: 1 })).toEqual([
      local(2026, 11, 19, 9),
      local(2026, 11, 20, 18),
    ]);
    expect(rangeFromForm({ date: 'bad', start: '09:00', end: '10:00', allDay: false })).toBeNull();
  });
});

describe('周视图', () => {
  it('周一起算；一周 7 天', () => {
    const sun = new Date(2026, 10, 22, 15); // 周日
    expect(dayKey(weekStart(sun))).toBe('2026-11-16');
    expect(dayKey(weekStart(new Date(2026, 10, 16)))).toBe('2026-11-16');
    expect(weekDays(weekStart(sun))).toEqual([
      '2026-11-16',
      '2026-11-17',
      '2026-11-18',
      '2026-11-19',
      '2026-11-20',
      '2026-11-21',
      '2026-11-22',
    ]);
  });

  it('重叠的并排、不重叠的各占整宽', () => {
    // 9-10、9:30-11、10-10:30 互相串成一簇 → 两道；12-13 自己一簇
    const r = layoutLanes([
      { start: 540, end: 600 },
      { start: 570, end: 660 },
      { start: 600, end: 630 },
      { start: 720, end: 780 },
    ]);
    expect(r).toEqual([
      { lane: 0, lanes: 2 },
      { lane: 1, lanes: 2 },
      { lane: 0, lanes: 2 },
      { lane: 0, lanes: 1 },
    ]);
    expect(layoutLanes([])).toEqual([]);
  });

  it('拖动改期保持时长；换天保持时刻', () => {
    const ev = { start_at: local(2026, 11, 20, 9), end_at: local(2026, 11, 20, 10, 30) };
    expect(movedRange(ev, new Date(2026, 10, 21, 14))).toEqual([local(2026, 11, 21, 14), local(2026, 11, 21, 15, 30)]);
    expect(sameTimeOn(ev, '2026-11-25').toISOString()).toBe(local(2026, 11, 25, 9));
  });
});

describe('重复、导入与农历', () => {
  it('重复规则双向', () => {
    expect(buildRRule('', { kind: 'never' }, 1)).toBe('');
    expect(buildRRule('WEEKDAYS', { kind: 'never' }, 1)).toBe('FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR');
    expect(buildRRule('WEEKLY', { kind: 'until', day: '2026-12-31' }, 5)).toBe('FREQ=WEEKLY;BYDAY=FR;UNTIL=20261231');
    expect(buildRRule('DAILY', { kind: 'count', n: 10 }, 0)).toBe('FREQ=DAILY;COUNT=10');
    expect(parseRepeat('FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR;UNTIL=20261231')).toEqual({
      freq: 'WEEKDAYS',
      end: { kind: 'until', day: '2026-12-31' },
    });
    expect(parseRepeat('FREQ=MONTHLY;COUNT=3')).toEqual({ freq: 'MONTHLY', end: { kind: 'count', n: 3 } });
    expect(addExdate('2026-11-20', '2026-11-18')).toBe('2026-11-18,2026-11-20');
    expect(addExdate('2026-11-20', '2026-11-20')).toBe('2026-11-20');
  });

  it('解析 .ics：折行、全天、UTC、转义、没有结束时间', () => {
    const ics = [
      'BEGIN:VCALENDAR',
      'BEGIN:VEVENT',
      'SUMMARY:周会\\, 第一次',
      'DTSTART:20261120T010000Z',
      'DTEND:20261120T020000Z',
      'DESCRIPTION:第一行\\n第',
      ' 二行',
      'RRULE:FREQ=WEEKLY;BYDAY=FR',
      'END:VEVENT',
      'BEGIN:VEVENT',
      'SUMMARY:出差',
      'DTSTART;VALUE=DATE:20261123',
      'END:VEVENT',
      'END:VCALENDAR',
    ].join('\r\n');
    const evs = parseIcs(ics);
    expect(evs).toHaveLength(2);
    expect(evs[0]).toMatchObject({
      title: '周会, 第一次',
      note: '第一行\n第二行',
      allDay: false,
      rrule: 'FREQ=WEEKLY;BYDAY=FR',
    });
    expect(evs[0]!.start.toISOString()).toBe('2026-11-20T01:00:00.000Z');
    expect(evs[1]!.allDay).toBe(true);
    expect(evs[1]!.end.getTime() - evs[1]!.start.getTime()).toBe(86_400_000);
  });

  it('农历与节日', () => {
    expect(lunarLabel(new Date(2026, 1, 17))).toEqual({ text: '春节', festival: true });
    expect(lunarLabel(new Date(2026, 1, 16))).toEqual({ text: '除夕', festival: true });
    expect(lunarLabel(new Date(2026, 9, 1))).toEqual({ text: '国庆节', festival: true });
    expect(lunarLabel(new Date(2026, 10, 20)).text).toBe('十二');
  });
});
