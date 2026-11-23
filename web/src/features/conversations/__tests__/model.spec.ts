import { describe, expect, it } from 'vitest';
import type { CodingTask } from '../../../services/kratos/agentbridge/v1';
import type { RunDetail, Terminal } from '../../fleet/api';
import type { OfficeTaskRow } from '../../office/types';
import type { Session } from '../../session/types';
import {
  buildTasks,
  defaultExecution,
  groupBridgeEvents,
  flat,
  inView,
  matches,
  sessionStatus,
  sortTasks,
  taskBucket,
  taskLight,
  execLight,
  execDuration,
  taskStatus,
  type ConvTask,
  type Execution,
  type Sources,
} from '../model';

const session = (over: Partial<Session>): Session =>
  ({
    id: 's',
    owner_type: 'agent',
    agent_id: 'a1',
    team_id: '',
    title: '会话',
    summary: '',
    status: 'idle',
    status_reason: '',
    created_at: '2026-11-19T01:00:00Z',
    last_message_at: '2026-11-19T02:00:00Z',
    archived_at: '',
    deleted_at: '',
    ...over,
  }) as Session;
const coding = (over: Partial<CodingTask>): CodingTask =>
  ({
    id: 'c',
    agentKey: 'codex',
    status: 'done',
    prompt: '修复',
    createdAt: '2026-11-19T03:00:00Z',
    ...over,
  }) as CodingTask;
const terminal = (over: Partial<Terminal>): Terminal =>
  ({
    id: 't',
    agent: 'claude',
    status: 'idle',
    title: '',
    workspaceId: 'w1',
    startedAt: '2026-11-19T04:00:00Z',
    lastActivityAt: '2026-11-19T04:30:00Z',
    preview: '',
    statusChangedAt: '',
    ...over,
  }) as Terminal;
const office = (over: Partial<OfficeTaskRow>): OfficeTaskRow =>
  ({
    id: 'o',
    title: '交办',
    body: '',
    source: 'delegate',
    source_ref: '',
    status: 'open',
    assignee_kind: 'agent',
    assignee_id: 'a1',
    assignee_name: '',
    created_at: '',
    updated_at: '',
    ...over,
  }) as OfficeTaskRow;
const src = (over: Partial<Sources>): Sources => ({
  sessions: [],
  agentNames: { a1: '精灵助手' },
  teamNames: {},
  officeTasks: [],
  codingTasks: [],
  runs: [],
  terminals: [],
  presets: [],
  workspaces: [{ id: 'w1', name: 'teamwork' } as Sources['workspaces'][number]],
  ...over,
});

describe('sessionStatus', () => {
  it('maps platform session states to the five display states', () => {
    expect(sessionStatus(session({ status: 'awaiting_confirmation', status_reason: 'agent_awaiting_reply' }))).toEqual({
      status: 'waiting',
      reason: 'agent_awaiting_reply',
    });
    expect(sessionStatus(session({ status: 'interrupted', status_reason: 'error' })).status).toBe('failed');
    expect(sessionStatus(session({ status: 'interrupted', status_reason: 'user_cancelled' })).status).toBe('done');
    expect(sessionStatus(session({ status: 'running', status_reason: '' })).status).toBe('running');
    expect(sessionStatus(session({ status: 'idle', status_reason: '' })).status).toBe('idle');
  });
});

describe('flow runs', () => {
  const flow = (over: Record<string, unknown> = {}) =>
    ({
      executionId: 'x1',
      graphId: 'g1',
      sessionId: 'rec',
      status: 'running',
      currentState: {},
      steps: [],
      interruptNode: '',
      errorMessage: '',
      currentNode: '审核',
      lineageId: '',
      startedAt: '2026-11-21T01:00:00Z',
      finishedAt: '',
      ...over,
    }) as NonNullable<Sources['flows']>[number];
  const record = session({
    id: 'rec',
    metadata_json: JSON.stringify({
      cc_role: 'task_record',
      cc_links: [{ kind: 'flow', id: 'x1', graphId: 'g1', name: '发版流程', hand: '【任务】发版' }],
    }),
  });

  it('hangs a dispatched flow run under its task with the name and handoff from the link', () => {
    const tasks = buildTasks(
      src({
        sessions: [record],
        officeTasks: [office({ id: 'o1', title: '发版', source_ref: 'rec' })],
        flows: [flow()],
      }),
    );
    expect(tasks).toHaveLength(1);
    expect(tasks[0]).toMatchObject({ id: 'office:o1', title: '发版' });
    expect(tasks[0]!.chain).toHaveLength(1);
    expect(tasks[0]!.chain[0]).toMatchObject({
      kind: 'flow',
      refId: 'x1',
      executor: 'flow:g1',
      executorName: '发版流程',
      status: 'running',
      what: '审核',
      hand: '【任务】发版',
    });
  });

  it('builds a task from the execution table alone when the office task has no carrying session', () => {
    // 流程页直接运行后登记的任务：没有会话、没有资料袋，只有关联表里的一行
    const tasks = buildTasks(
      src({
        officeTasks: [
          office({ id: 'o9', title: '月度复盘', source_ref: '', assignee_kind: 'flow', assignee_id: 'g1' }),
        ],
        flows: [flow({ status: 'completed' })],
        executions: [
          {
            id: 'e1',
            task_id: 'o9',
            kind: 'graph_execution',
            ref_id: 'x1',
            executor: 'flow:g1',
            executor_name: '发版流程',
            hand: '',
            created_at: '2026-11-21T01:00:00Z',
          },
        ],
      }),
    );
    expect(tasks).toHaveLength(1);
    expect(tasks[0]).toMatchObject({ id: 'office:o9', title: '月度复盘', officeId: 'o9' });
    expect(tasks[0]!.chain[0]).toMatchObject({ kind: 'flow', executorName: '发版流程', status: 'done' });
  });

  it('maps flow states and hides the task when the run could not be read', () => {
    const one = (status: string) => buildTasks(src({ sessions: [record], flows: [flow({ status })] }))[0]!.chain[0];
    expect(one('interrupted')).toMatchObject({ status: 'waiting', reason: 'input' });
    expect(one('failed')).toMatchObject({ status: 'failed' });
    expect(one('completed')).toMatchObject({ status: 'done' });
    expect(buildTasks(src({ sessions: [record], flows: [] }))).toHaveLength(0);
  });
});

describe('buildTasks', () => {
  it('groups a delegated task with its carrying session and the coding tasks it dispatched', () => {
    const tasks = buildTasks(
      src({
        sessions: [session({ id: 's1' }), session({ id: 's2', title: '单独聊' })],
        officeTasks: [office({ id: 'o1', title: '修审批中心', source_ref: 's1' })],
        codingTasks: [coding({ id: 'c1', sessionId: 's1', agentKey: 'deepseek', status: 'running' })],
      }),
    );
    expect(tasks.map((t) => [t.id, t.origin, flat(t).map((e) => e.id)])).toEqual([
      ['office:o1', 'office', ['session:s1', 'bridge:c1']],
      ['auto:session:s2', 'auto', ['session:s2']],
    ]);
    expect(flat(tasks[0]!)[1]).toMatchObject({ executor: 'ds', executorName: 'DeepSeek Harness' });
    expect(taskStatus(tasks[0]!)).toBe('running');
  });

  it('turns a fleet run into coordinator → parallel workers, and leaves shell terminals out', () => {
    const run = {
      run: { id: 'r1', objective: '拆分重构', coordinatorTerminalId: 't0', workspaceId: 'w1' },
      tasks: [{ workerTerminalId: 't2' }, { workerTerminalId: 't1' }],
      messages: [],
    } as unknown as RunDetail;
    const tasks = buildTasks(
      src({
        runs: [run],
        terminals: [
          terminal({ id: 't0', agent: 'claude' }),
          terminal({ id: 't1', agent: 'codex', startedAt: '2026-11-19T05:00:00Z' }),
          terminal({ id: 't2', agent: 'codebuddy', startedAt: '2026-11-19T06:00:00Z', status: 'needs_input' }),
          terminal({ id: 't3', agent: 'shell' }),
          terminal({ id: 't4', agent: 'gemini' }),
        ],
      }),
    );
    expect(tasks[0]!.chain.map((c) => (Array.isArray(c) ? c.map((e) => e.executor) : c.executor))).toEqual([
      'cc',
      ['cx', 'wb'],
    ]);
    expect(tasks[0]).toMatchObject({ title: '拆分重构', project: 'teamwork' });
    expect(taskStatus(tasks[0]!)).toBe('waiting');
    expect(defaultExecution(tasks[0]!)?.refId).toBe('t2');
    // 编排外的终端自成任务；普通终端不算对话
    expect(tasks.slice(1).map((t) => flat(t)[0]!.refId)).toEqual(['t4']);
  });

  it('puts a task in the archive only when every execution is archived', () => {
    const [task] = buildTasks(
      src({
        sessions: [session({ id: 's1', archived_at: '2026-11-18T00:00:00Z' })],
        codingTasks: [coding({ id: 'c1', sessionId: 's1', reviewStatus: 'archived' })],
      }),
    );
    expect(taskBucket(task!)).toBe('archived');
    expect(inView(task!, 'archived', new Date('2026-11-19T08:00:00Z'))).toBe(true);
    expect(inView(task!, 'all', new Date('2026-11-19T08:00:00Z'))).toBe(false);
  });
});

describe('sortTasks / matches', () => {
  it('puts waiting first, then running, then the rest by last activity', () => {
    const tasks = buildTasks(
      src({
        sessions: [
          session({ id: 'old', last_message_at: '2026-11-19T01:00:00Z' }),
          session({ id: 'new', last_message_at: '2026-11-19T09:00:00Z' }),
          session({ id: 'run', status: 'running', last_message_at: '2026-11-18T09:00:00Z' }),
          session({ id: 'ask', status: 'awaiting_confirmation', last_message_at: '2026-11-17T09:00:00Z' }),
        ],
      }),
    );
    expect(sortTasks(tasks).map((t) => flat(t)[0]!.refId)).toEqual(['ask', 'run', 'new', 'old']);
    expect(matches(tasks[0]!, '精灵')).toBe(true);
    expect(matches(tasks[0]!, '不存在的词')).toBe(false);
  });
});

describe('groupBridgeEvents', () => {
  it('merges message chunks, groups consecutive tool calls and drops heartbeats', () => {
    expect(
      groupBridgeEvents([
        { kind: 'agent_message_chunk', text: '先读' },
        { kind: 'agent_message_chunk', text: '代码。' },
        { kind: 'tool_call', title: 'read', text: 'a.ts' },
        { kind: 'heartbeat' },
        { kind: 'tool_call', title: 'edit', text: 'a.ts' },
        { kind: 'permission', title: '写文件', text: 'a.ts' },
        { kind: 'agent_message_chunk', text: '好了' },
      ]),
    ).toEqual([
      { kind: 'ai', text: '先读代码。' },
      {
        kind: 'tools',
        items: [
          { title: 'read', text: 'a.ts' },
          { title: 'edit', text: 'a.ts' },
        ],
      },
      { kind: 'ask', title: '写文件', text: 'a.ts' },
      { kind: 'ai', text: '好了' },
    ]);
  });
});

describe('status lights', () => {
  const ex = (over: Partial<Execution> = {}): Execution => ({
    id: 'session:s',
    kind: 'session',
    refId: 's',
    executor: 'agent:a',
    executorName: '写手',
    what: '',
    model: '',
    status: 'done',
    reason: '',
    detail: '',
    start: '2026-11-21T01:00:00Z',
    last: '2026-11-21T01:01:30Z',
    turns: 0,
    tokens: 0,
    costUsd: 0,
    bucket: 'active',
    pinned: false,
    agentId: 'a',
    workspaceId: '',
    parentSessionId: '',
    project: '',
    hand: '',
    ...over,
  });
  const task = (e: Execution, over: Partial<ConvTask> = {}): ConvTask => ({
    id: 't',
    title: 't',
    origin: 'auto',
    project: '',
    chain: [e],
    ...over,
  });

  it('green while working, yellow when it needs you, red when interrupted', () => {
    expect(taskLight(task(ex({ status: 'running' })))).toBe('running');
    expect(taskLight(task(ex({ status: 'waiting' })))).toBe('waiting');
    expect(taskLight(task(ex({ status: 'failed' })))).toBe('failed');
  });

  it('blue when done until you confirm it, then grey; idle chats count as done', () => {
    expect(taskLight(task(ex()))).toBe('done');
    expect(taskLight(task(ex({ status: 'idle' })))).toBe('done');
    expect(taskLight(task(ex(), { officeDone: true }))).toBe('confirmed');
    expect(taskLight(task(ex({ bucket: 'archived' })))).toBe('confirmed');
  });

  it('a reviewed coding-tool run is grey on its own card', () => {
    expect(execLight(ex({ kind: 'bridge', reviewed: true }))).toBe('confirmed');
    expect(execLight(ex({ kind: 'bridge' }))).toBe('done');
    expect(execDuration(ex())).toBe(90_000);
    expect(execDuration(ex({ last: '' }))).toBe(0);
  });

  it('platform notes from the coding-tool bridge become note blocks', () => {
    expect(
      groupBridgeEvents([
        { kind: '_aranea/note', text: '在运行台里启动' },
        { kind: '_aranea/screen', text: '│ ♦ │' },
      ]),
    ).toEqual([{ kind: 'note', text: '在运行台里启动' }]);
  });
});
