import { describe, expect, it } from 'vitest';
import type { InboxItemRow } from '../../approval/api';
import { buildLanesFromInbox, groupByRequester, inboxRowToYouItem, inboxSourceLane } from '../logic';

function row(id: string, over: Partial<InboxItemRow> = {}): InboxItemRow {
  return {
    id,
    source: 'task_review',
    kind: 'review',
    title: `title ${id}`,
    summary: `summary ${id}`,
    risk_level: 'medium',
    requester: 'coder',
    agent_key: 'coder',
    created_at: '2026-11-15T01:00:00Z',
    due_at: '',
    actions: ['approve', 'reject'],
    reject_requires_note: true,
    link: '/approvals',
    ...over,
  };
}

describe('inboxSourceLane', () => {
  it('maps known sources and defaults everything else to approval', () => {
    expect(inboxSourceLane('hitl_confirm')).toBe('trade');
    expect(inboxSourceLane('publish_review')).toBe('publish');
    expect(inboxSourceLane('opportunity')).toBe('listing');
    expect(inboxSourceLane('dead_letter')).toBe('alerts');
    expect(inboxSourceLane('cron_failure')).toBe('alerts');
    expect(inboxSourceLane('schedule_draft')).toBe('approval');
    expect(inboxSourceLane('some_future_kind')).toBe('approval');
  });
});

describe('inboxRowToYouItem', () => {
  it('keeps the server id and picks the first non-reject action', () => {
    const item = inboxRowToYouItem(row('task_review:t1'));
    expect(item.id).toBe('task_review:t1');
    expect(item.lane).toBe('approval');
    expect(item.action).toBe('approve');
    expect(item.source).toBe('task_review');
    expect(item.agentKey).toBe('coder');
  });

  it('degrades to goto when the server offers no action', () => {
    const item = inboxRowToYouItem(row('offboard_export:o1', { source: 'offboard_export', actions: [], link: '' }));
    expect(item.action).toBe('goto');
    expect(item.to).toBe('/approvals');
  });

  it('uses display tags for alert sources and marks high risk as danger', () => {
    const dead = inboxRowToYouItem(
      row('dead_letter:d1', { source: 'dead_letter', actions: ['resolve'], risk_level: 'high' }),
    );
    expect(dead.lane).toBe('alerts');
    expect(dead.source).toBe('dead');
    expect(dead.action).toBe('resolve');
    expect(dead.danger).toBe(true);
    expect(inboxRowToYouItem(row('cron_failure:r1', { source: 'cron_failure', actions: ['resolve'] })).source).toBe(
      'cron',
    );
    expect(inboxRowToYouItem(row('opportunity:o1', { source: 'opportunity', actions: ['ack'] })).action).toBe('ack');
  });

  it('falls back to medium severity and placeholder requester', () => {
    const item = inboxRowToYouItem(row('x:1', { risk_level: '', requester: '', title: '' }));
    expect(item.severity).toBe('medium');
    expect(item.who).toBe('—');
    expect(item.summary).toBe('summary x:1');
  });
});

describe('buildLanesFromInbox', () => {
  const now = '2026-11-15T02:00:00Z';

  it('always returns five lanes in fixed order', () => {
    const lanes = buildLanesFromInbox([], {}, [], now);
    expect(lanes.map((l) => l.lane)).toEqual(['approval', 'trade', 'publish', 'listing', 'alerts']);
    expect(lanes.every((l) => l.count === 0 && !l.degraded && l.updatedAt === now)).toBe(true);
  });

  it('counts come from server totals, rows are capped at five, order is preserved', () => {
    const rows = Array.from({ length: 7 }, (_, i) => row(`task_review:t${i}`));
    const lanes = buildLanesFromInbox(
      rows,
      { task_review: 40, schedule_draft: 2, cron_failure: 3, dead_letter: 1 },
      [],
      now,
    );
    const approval = lanes[0];
    expect(approval.count).toBe(42);
    expect(approval.items.map((i) => i.id)).toEqual(rows.slice(0, 5).map((r) => r.id));
    expect(approval.agentCounts).toEqual({ coder: 7 });
    expect(lanes[4].count).toBe(4);
  });

  it('marks only the lane of a failed source as degraded and keeps its previous freshness', () => {
    const lanes = buildLanesFromInbox([], { hitl_confirm: 0 }, ['hitl_confirm'], now, {
      trade: '2026-11-15T01:30:00Z',
    });
    const trade = lanes.find((l) => l.lane === 'trade');
    expect(trade?.degraded).toBe(true);
    expect(trade?.updatedAt).toBe('2026-11-15T01:30:00Z');
    expect(lanes.filter((l) => l.degraded).map((l) => l.lane)).toEqual(['trade']);
    const neverLoaded = buildLanesFromInbox([], {}, ['cron_failure'], now).find((l) => l.lane === 'alerts');
    expect(neverLoaded?.updatedAt).toBe('');
  });
});

describe('groupByRequester', () => {
  it('组间按最高风险级排序、组内按风险级 → 时间；空请求人排最后', () => {
    const items = [
      row('task_review:low-u1', { requester: 'u1', risk_level: 'low', created_at: '2026-11-10T01:00:00Z' }),
      row('task_review:high-u2', { requester: 'u2', risk_level: 'high', created_at: '2026-11-10T00:00:00Z' }),
      row('task_review:mid-u1', { requester: 'u1', risk_level: 'medium', created_at: '2026-11-10T02:00:00Z' }),
      row('task_review:none', { requester: '', risk_level: 'high' }),
    ].map(inboxRowToYouItem);
    const groups = groupByRequester(items);
    expect(groups.map((g) => g.requester)).toEqual(['u2', 'u1', '—']);
    expect(groups[1].items.map((i) => i.id)).toEqual(['task_review:mid-u1', 'task_review:low-u1']);
  });
});
