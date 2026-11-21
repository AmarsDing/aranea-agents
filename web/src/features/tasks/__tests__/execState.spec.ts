import { describe, expect, it } from 'vitest';
import type { OfficeTaskExecutionRow } from '../../office/api';
import { classifyOfficeDelegated } from '../../today/logic';
import { executionOverrides, pendingRefs, type ExecFacts } from '../execState';

const row = (over: Partial<OfficeTaskExecutionRow>): OfficeTaskExecutionRow => ({
  id: 'e',
  task_id: 't1',
  kind: 'graph_execution',
  ref_id: 'x1',
  executor: 'flow:g1',
  executor_name: '发版流程',
  hand: '',
  created_at: '2026-11-21T01:00:00Z',
  ...over,
});
const flow = (status: string, over: Record<string, unknown> = {}) =>
  ({
    executionId: 'x1',
    graphId: 'g1',
    sessionId: 's',
    status,
    currentState: {},
    steps: [],
    interruptNode: '',
    errorMessage: '',
    currentNode: '审核',
    lineageId: '',
    startedAt: '2026-11-21T01:00:00Z',
    finishedAt: '',
    ...over,
  }) as ExecFacts['flows'][number];
const facts = (over: Partial<ExecFacts>): ExecFacts => ({
  codingTasks: [],
  flows: [],
  terminals: [],
  runs: [],
  ...over,
});

describe('executionOverrides', () => {
  it('folds the latest non-session execution of a task into the three board states', () => {
    const one = (status: string) => executionOverrides([row({})], facts({ flows: [flow(status)] })).get('t1');
    expect(one('running')).toMatchObject({ state: 'running', waiting: false, detail: '审核' });
    expect(one('interrupted')).toMatchObject({ state: 'stuck', waiting: true });
    expect(one('failed')).toMatchObject({ state: 'stuck', waiting: false });
    expect(one('completed')).toMatchObject({ state: 'review' });
  });

  it('lets a later execution of the same task win, and draws no conclusion for records it could not read', () => {
    const rows = [row({ id: 'a', ref_id: 'x1' }), row({ id: 'b', kind: 'fleet_run', ref_id: 'r1' })];
    const run = { id: 'r1', status: 'running', doneCount: 1, taskCount: 3, createdAt: '2026-11-21T02:00:00Z' };
    const got = executionOverrides(
      rows,
      facts({ flows: [flow('completed')], runs: [run as ExecFacts['runs'][number]] }),
    );
    expect(got.get('t1')).toMatchObject({ state: 'running', detail: '1/3' });
    expect(executionOverrides([row({})], facts({})).size).toBe(0);
  });
});

describe('pendingRefs', () => {
  it('only asks about non-session executions of tasks that are still open', () => {
    const got = pendingRefs(
      [
        row({ id: 'a' }),
        row({ id: 'b', kind: 'session', ref_id: 's1' }),
        row({ id: 'c', task_id: 'closed', ref_id: 'x9' }),
        row({ id: 'd', kind: 'fleet_terminal', ref_id: 'term' }),
      ],
      new Set(['t1']),
    );
    expect(got.rows.map((r) => r.id)).toEqual(['a', 'd']);
    expect(got.flowIds).toEqual(['x1']);
    expect(got.needsFleet).toBe(true);
  });
});

describe('classifyOfficeDelegated with execution overrides', () => {
  const task = {
    id: 't1',
    title: '发版',
    source_ref: 'rec',
    session_id: 'rec',
    assignee_kind: 'agent',
    assignee_id: 'spirit',
    assignee_name: '发版流程',
    created_at: '2026-11-21T01:00:00Z',
  };
  const done = new Map([['t1', { state: 'review' as const, waiting: false, detail: '', at: '2026-11-21T01:05:00Z' }]]);

  it('moves a finished flow task to review instead of leaving it running forever', () => {
    expect(classifyOfficeDelegated([task], [])[0]).toMatchObject({ state: 'running' });
    expect(classifyOfficeDelegated([task], [], done)[0]).toMatchObject({ state: 'review', at: '2026-11-21T01:05:00Z' });
  });

  it('still trusts the carrying session when it has a task situation', () => {
    const situations = [{ session_id: 'rec', status: 'running', updated_at: '2026-11-21T01:06:00Z' }];
    expect(classifyOfficeDelegated([task], situations, done)[0]).toMatchObject({ state: 'running' });
  });
});
