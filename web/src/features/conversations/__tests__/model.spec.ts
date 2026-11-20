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
  taskStatus,
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
