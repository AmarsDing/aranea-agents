import { beforeEach, describe, expect, it, vi } from 'vitest';

const api = vi.hoisted(() => ({ createOfficeTask: vi.fn(), addOfficeTaskExecution: vi.fn() }));
vi.mock('../../office/api', () => api);

import { assigneeIcon, harnessOrigin, talksInSession } from '../board';
import { recordHarnessRun } from '../recordRun';

const run = {
  title: '把登录页改成扫码登录\n兼容旧账号',
  assigneeKind: 'run' as const,
  assigneeId: 'codex',
  assigneeName: 'Codex 协调',
  exec: { kind: 'fleet_run' as const, ref_id: 'r1', executor: 'cx', hand: '把登录页改成扫码登录' },
};

describe('recordHarnessRun', () => {
  beforeEach(() => {
    api.createOfficeTask.mockReset().mockResolvedValue({ id: 't1' });
    api.addOfficeTaskExecution.mockReset().mockResolvedValue({});
  });

  it('registers a session-less task titled by the first line and hangs the run under it', async () => {
    expect(await recordHarnessRun(run)).toBe('t1');
    expect(api.createOfficeTask).toHaveBeenCalledWith('把登录页改成扫码登录', '兼容旧账号', {
      sessionId: '',
      assigneeKind: 'run',
      assigneeId: 'codex',
      assigneeName: 'Codex 协调',
    });
    expect(api.addOfficeTaskExecution).toHaveBeenCalledWith('t1', {
      kind: 'fleet_run',
      ref_id: 'r1',
      executor: 'cx',
      executor_name: 'Codex 协调',
      hand: '把登录页改成扫码登录',
    });
  });

  it('never throws: the run has already started, a failed registration only means one row less', async () => {
    api.createOfficeTask.mockRejectedValue(new Error('old backend'));
    expect(await recordHarnessRun(run)).toBe('');
    expect(api.addOfficeTaskExecution).not.toHaveBeenCalled();
  });

  it('skips runs with nothing to title them by', async () => {
    expect(await recordHarnessRun({ ...run, title: '  ' })).toBe('');
    expect(api.createOfficeTask).not.toHaveBeenCalled();
  });
});

describe('executor kind helpers', () => {
  it('tells where a harness-started task came from and which icon it gets', () => {
    expect(harnessOrigin({ assignee_kind: 'run' })).toBe('fleet');
    expect(harnessOrigin({ assignee_kind: 'tool', session_id: '' })).toBe('fleet');
    expect(harnessOrigin({ assignee_kind: 'flow' })).toBe('flow');
    expect(harnessOrigin({ assignee_kind: 'agent' })).toBe('');
    // 交办面板派给流程的任务带着资料袋会话：是从工作台下达的，不标来源
    expect(harnessOrigin({ assignee_kind: 'flow', session_id: 'rec' })).toBe('');
    expect(harnessOrigin(null)).toBe('');
    expect(talksInSession('team')).toBe(true);
    expect(talksInSession('flow')).toBe(false);
    expect(assigneeIcon('team')).toBe('groups');
    expect(assigneeIcon('flow')).toBe('schema');
    expect(assigneeIcon('agent')).toBe('smart_toy');
    expect(assigneeIcon(undefined)).toBe('smart_toy');
  });
});
