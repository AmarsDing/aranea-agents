import { describe, expect, it } from 'vitest';
import type { InboxItemRow } from '../../approval/api';
import type { OfficeEventRow } from '../../office/types';
import {
  batchableLowRisk,
  buildTimeline,
  classifyDelegated,
  classifyOfficeDelegated,
  clock,
  DELEGATED_WINDOW_MS,
  inboxHeadline,
  primaryAction,
} from '../logic';

// 本地时间 2026-11-16 10:00；用本地构造避免测试依赖运行机时区。
const NOW = new Date(2026, 10, 16, 10, 0, 0);
const at = (h: number, m = 0, dayOffset = 0) => new Date(2026, 10, 16 + dayOffset, h, m, 0).toISOString();

function row(over: Partial<InboxItemRow>): InboxItemRow {
  return {
    id: 'x:1',
    source: 'schedule_draft',
    kind: 'decide',
    title: 't',
    summary: '',
    risk_level: 'medium',
    requester: '',
    agent_key: '',
    created_at: '',
    due_at: '',
    actions: ['approve', 'reject'],
    reject_requires_note: true,
    link: '/approvals',
    ...over,
  };
}

function event(over: Partial<OfficeEventRow>): OfficeEventRow {
  return {
    id: 'e1',
    task_ref: '',
    task_title: '',
    title: '周会',
    note: '',
    start_at: at(9, 30),
    end_at: at(10, 30),
    timezone: '',
    reminder_minutes: 0,
    source: 'manual',
    ...over,
  };
}

describe('clock', () => {
  it('formats local HH:mm and tolerates bad input', () => {
    expect(clock(at(9, 5))).toBe('09:05');
    expect(clock('')).toBe('');
    expect(clock('not a date')).toBe('');
  });
});

describe('inboxHeadline', () => {
  it('counts overdue and today as due today, and high risk separately', () => {
    const rows = [
      row({ id: 'a', due_at: at(18) }),
      row({ id: 'b', due_at: at(8, 0, -1), risk_level: 'high' }),
      row({ id: 'c', due_at: at(9, 0, 1), risk_level: 'high' }),
      row({ id: 'd' }),
    ];
    expect(inboxHeadline(rows, 4, NOW)).toEqual({ total: 4, dueToday: 2, highRisk: 2 });
  });

  it('never reports a total smaller than what is on screen', () => {
    expect(inboxHeadline([row({}), row({ id: 'b' })], 1, NOW).total).toBe(2);
    expect(inboxHeadline([row({})], 40, NOW).total).toBe(40);
  });
});

describe('primaryAction / batchableLowRisk', () => {
  it('picks the first non-reject action', () => {
    expect(primaryAction(row({ actions: ['reject', 'approve'] }))).toBe('approve');
    expect(primaryAction(row({ actions: ['ack'] }))).toBe('ack');
    expect(primaryAction(row({ actions: ['reject'] }))).toBe('');
    expect(primaryAction(row({ actions: [] }))).toBe('');
  });

  it('batches only low-risk approvable items of the same source', () => {
    const rows = [
      row({ id: 'a', source: 'skill_approval', risk_level: 'low' }),
      row({ id: 'b', source: 'skill_approval', risk_level: 'low' }),
      row({ id: 'c', source: 'skill_approval', risk_level: 'high' }),
      row({ id: 'd', source: 'skill_approval', risk_level: 'low', actions: [] }),
      row({ id: 'e', source: 'schedule_draft', risk_level: 'low' }),
    ];
    expect(batchableLowRisk(rows, 'skill_approval').map((r) => r.id)).toEqual(['a', 'b']);
  });
});

describe('buildTimeline', () => {
  it('merges events, deadlines and upcoming cron runs for today in time order', () => {
    const entries = buildTimeline({
      events: [event({}), event({ id: 'e2', title: '客户电话', start_at: at(14), end_at: at(15) })],
      inbox: [row({ id: 'hitl:1', title: '调仓方案', due_at: at(18) }), row({ id: 'nodue' })],
      cron: [
        { id: 'c1', name: '定时发文', nextRunAt: at(11) },
        { id: 'c2', name: '已错过', nextRunAt: at(8) },
      ],
      now: NOW,
    });
    expect(entries.map((e) => `${e.kind}:${e.title}`)).toEqual([
      'event:周会',
      'cron:定时发文',
      'event:客户电话',
      'deadline:调仓方案',
    ]);
    expect(entries[3]?.inboxId).toBe('hitl:1');
    expect(entries[0]?.to).toBe('/schedule');
  });

  it('marks an event past only after it ends, and drops other days', () => {
    const entries = buildTimeline({
      events: [
        event({ id: 'running', start_at: at(9, 30), end_at: at(10, 30) }),
        event({ id: 'over', start_at: at(8), end_at: at(9) }),
        event({ id: 'tomorrow', start_at: at(9, 0, 1), end_at: at(10, 0, 1) }),
        event({ id: 'bad', start_at: '' }),
      ],
      inbox: [row({ id: 'late', due_at: at(9) }), row({ id: 'yesterday', due_at: at(9, 0, -1) })],
      cron: [],
      now: NOW,
    });
    expect(entries.map((e) => [e.id, e.past])).toEqual([
      ['event:over', true],
      ['deadline:late', true],
      ['event:running', false],
    ]);
  });

  it('falls back to the linked task title when an event has no title', () => {
    const [entry] = buildTimeline({
      events: [event({ title: '', task_title: '写复盘' })],
      inbox: [],
      cron: [],
      now: NOW,
    });
    expect(entry?.title).toBe('写复盘');
  });
});

describe('classifyDelegated', () => {
  it('sorts stuck first, then to-review, then running, newest first within a state', () => {
    const rows = classifyDelegated(
      [
        { id: 'run-old', status: 'running', prompt: '询价跟进', lastActiveAt: at(8) },
        { id: 'run-new', status: 'dispatched', prompt: '新任务', createdAt: at(9, 30) },
        { id: 'review', status: 'done', summary: '竞品周报已完成', completedAt: at(9) },
        { id: 'stuck', status: 'failed', prompt: '直播脚本', completedAt: at(7) },
      ],
      NOW,
    );
    expect(rows.map((r) => `${r.id}:${r.state}`)).toEqual([
      'stuck:stuck',
      'review:review',
      'run-new:running',
      'run-old:running',
    ]);
    expect(rows[1]?.title).toBe('竞品周报已完成');
  });

  it('drops reviewed, cancelled and long-finished tasks', () => {
    const old = new Date(NOW.getTime() - DELEGATED_WINDOW_MS - 60_000).toISOString();
    const rows = classifyDelegated(
      [
        { id: 'approved', status: 'done', reviewStatus: 'approved', completedAt: at(9) },
        { id: 'rejected', status: 'failed', reviewStatus: 'rejected', completedAt: at(9) },
        { id: 'cancelled', status: 'cancelled', completedAt: at(9) },
        { id: 'ancient', status: 'failed', completedAt: old },
        { status: 'running' },
      ],
      NOW,
    );
    expect(rows).toEqual([]);
  });

  it('uses the first line of the prompt and clips long titles', () => {
    const [short] = classifyDelegated([{ id: 'a', status: 'running', prompt: '第一行\n第二行' }], NOW);
    expect(short?.title).toBe('第一行');
    const [long] = classifyDelegated([{ id: 'b', status: 'running', prompt: '长'.repeat(80) }], NOW);
    expect(long?.title).toBe(`${'长'.repeat(60)}…`);
    const [bare] = classifyDelegated([{ id: 'c', status: 'running', projectName: 'proj' }], NOW);
    expect(bare?.title).toBe('proj');
  });
});

describe('classifyOfficeDelegated', () => {
  const task = (id: string, session: string, kind = 'agent') => ({
    id,
    title: `任务 ${id}`,
    source_ref: session,
    assignee_kind: kind,
    assignee_id: `${kind}-1`,
    assignee_name: '内容组',
    created_at: '2026-11-16T01:00:00Z',
  });

  it('maps the latest situation of the carrying session to a state', () => {
    const rows = classifyOfficeDelegated(
      [task('run', 's-run'), task('done', 's-done'), task('wait', 's-wait'), task('fresh', 's-none')],
      [
        {
          session_id: 's-run',
          status: 'running',
          updated_at: '2026-11-16T02:00:00Z',
          current_step_summary: '检索资料',
        },
        { session_id: 's-done', status: 'failed', updated_at: '2026-11-16T02:00:00Z' },
        { session_id: 's-done', status: 'completed', updated_at: '2026-11-16T03:00:00Z' },
        {
          session_id: 's-wait',
          status: 'waiting',
          updated_at: '2026-11-16T02:30:00Z',
          current_step_summary: '等待确认',
        },
      ],
    );
    expect(rows.map((r) => `${r.officeTaskId}:${r.state}`)).toEqual([
      'wait:stuck',
      'done:review',
      'run:running',
      'fresh:running',
    ]);
    expect(rows.find((r) => r.officeTaskId === 'run')?.detail).toBe('检索资料');
    expect(rows.find((r) => r.officeTaskId === 'done')?.detail).toBe('');
    expect(rows.find((r) => r.officeTaskId === 'fresh')?.at).toBe('2026-11-16T01:00:00Z');
  });

  it('links each row to its session and keeps ids distinct from coding tasks', () => {
    const [agent] = classifyOfficeDelegated([task('a', 's 1')], []);
    expect(agent).toMatchObject({ id: 'office:a', agentKey: '内容组', to: '/chat?session=s%201&agent=agent-1' });
    const [team] = classifyOfficeDelegated([task('b', 's2', 'team')], []);
    expect(team?.to).toBe('/chat?session=s2&team=team-1');
    const [coding] = classifyDelegated([{ id: 'a', status: 'running', prompt: 'x' }], NOW);
    expect(coding).toMatchObject({ id: 'a', officeTaskId: '', to: '/coding-tasks?focus=a' });
  });
});
