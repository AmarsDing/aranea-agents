import { describe, it, expect } from 'vitest';
import type { TeamRunEvent } from '../../teams/types';
import type { PlatformResource, MonitorTrace } from '../types';
import {
  categoryForEventType,
  severityForPersistedStatus,
  severityForWsEvent,
  wsEventToView,
  persistedEventToView,
} from '../eventView';

/** 测试用 t()：回显 key 与插值参数，断言与语言解耦。 */
const t = (key: string, ...args: unknown[]) =>
  args.length ? `${key}${JSON.stringify(args)}` : key;

function makeWsEvent(partial: Partial<TeamRunEvent>): TeamRunEvent {
  return { type: 'runtime.event', team_id: '', run_id: '', ...partial };
}

function makeRow(partial: Partial<PlatformResource>): PlatformResource {
  return {
    id: 'row-1',
    resource: 'monitor-events',
    key: '',
    name: '',
    description: '',
    status: '',
    enabled: true,
    sort_order: 0,
    parent_id: '',
    level: '',
    agent_id: '',
    agent_key: '',
    display_name: '',
    config_json: '{}',
    metadata_json: '{}',
    created_at: '2026-07-29T10:00:00Z',
    updated_at: '2026-07-29T10:00:00Z',
    deleted_at: '',
    ...partial,
  } as PlatformResource;
}

describe('categoryForEventType', () => {
  it('runner.completion 归入 system（需求 §3.2）', () => {
    expect(categoryForEventType('runner.completion')).toBe('system');
    expect(categoryForEventType('runner_completion')).toBe('system');
  });
  it('team_run / run.* 归入 task', () => {
    expect(categoryForEventType('team_run_started')).toBe('task');
    expect(categoryForEventType('team_run_failed')).toBe('task');
  });
  it('chat/message 归入 message', () => {
    expect(categoryForEventType('chat.user_feedback')).toBe('message');
    expect(categoryForEventType('message.received')).toBe('message');
  });
  it('alert/skill/system 归入 system', () => {
    expect(categoryForEventType('alert.fired')).toBe('system');
    expect(categoryForEventType('skill.filesystem.rejected')).toBe('system');
    expect(categoryForEventType('usage.budget_alert')).toBe('system');
  });
  it('tool/agent/intent_pass 映射', () => {
    expect(categoryForEventType('tool.invoke')).toBe('tool');
    expect(categoryForEventType('agent.started')).toBe('agent');
    expect(categoryForEventType('intent_pass')).toBe('agent');
    expect(categoryForEventType('team_step_started')).toBe('tool');
  });
});

describe('severityForPersistedStatus', () => {
  it('status 映射 severity', () => {
    expect(severityForPersistedStatus('warn')).toBe('warn');
    expect(severityForPersistedStatus('warning')).toBe('warn');
    expect(severityForPersistedStatus('error')).toBe('critical');
    expect(severityForPersistedStatus('critical')).toBe('critical');
    expect(severityForPersistedStatus('ok')).toBe('success');
    expect(severityForPersistedStatus('info')).toBe('info');
    expect(severityForPersistedStatus('')).toBe('info');
  });
});

describe('severityForWsEvent', () => {
  it('failed/error 为 warn', () => {
    expect(severityForWsEvent('team_run_failed')).toBe('warn');
    expect(severityForWsEvent('team_step_finished', { failed: true })).toBe('warn');
  });
  it('finished/completed 为 success', () => {
    expect(severityForWsEvent('team_run_finished')).toBe('success');
    expect(severityForWsEvent('team_step_finished')).toBe('success');
  });
  it('其余为 info', () => {
    expect(severityForWsEvent('team_run_started')).toBe('info');
    expect(severityForWsEvent('intent_pass')).toBe('info');
  });
});

describe('wsEventToView', () => {
  it('team_run_started 人话化标题，不再显示原始 type', () => {
    const view = wsEventToView(
      t,
      makeWsEvent({
        type: 'team_run_started',
        session_id: 'sess-1234567890',
        run: { id: 'run-1', status: 'running', created_at: '2026-07-29T10:00:00Z', updated_at: '' } as never,
      }),
      0,
    );
    expect(view.title).toBe('monitorPage.events.title.teamRunStarted');
    expect(view.title).not.toContain('team_run_started');
    expect(view.severity).toBe('info');
    expect(view.category).toBe('task');
    expect(view.sessionId).toBe('sess-1234567890');
  });

  it('team_run_failed 摘要含错误信息', () => {
    const view = wsEventToView(
      t,
      makeWsEvent({
        type: 'team_run_failed',
        run: { id: 'run-2', status: 'failed', error_message: 'LLM timeout', created_at: '', updated_at: '' } as never,
      }),
      1,
    );
    expect(view.title).toBe('monitorPage.events.title.teamRunFailed');
    expect(view.subtitle).toBe('LLM timeout');
    expect(view.severity).toBe('warn');
  });

  it('team_step_started 标题含 Agent 名', () => {
    const view = wsEventToView(
      t,
      makeWsEvent({
        type: 'team_step_started',
        step: {
          id: 'step-1',
          agent_name: '小红',
          status: 'running',
          created_at: '2026-07-29T10:00:01Z',
        } as never,
      }),
      2,
    );
    expect(view.title).toContain('小红');
    expect(view.actor).toBe('小红');
  });

  it('team_step_finished 失败时 severity=warn', () => {
    const view = wsEventToView(
      t,
      makeWsEvent({
        type: 'team_step_finished',
        step: { id: 'step-2', agent_name: '小明', status: 'failed', error_message: 'boom', created_at: '' } as never,
      }),
      3,
    );
    expect(view.severity).toBe('warn');
    expect(view.subtitle).toBe('boom');
  });

  it('id 使用 seq 而非数组长度（稳定）', () => {
    const a = wsEventToView(t, makeWsEvent({ type: 'team_run_started', run: { id: 'r' } as never }), 7);
    expect(a.id).toContain('-7');
  });

  it('未知类型回退：标题为 type，无占位废文案', () => {
    const view = wsEventToView(t, makeWsEvent({ type: 'custom.event' }), 0);
    expect(view.title).toBe('custom.event');
    expect(view.subtitle).toBe('');
  });
});

describe('persistedEventToView', () => {
  const traces: MonitorTrace[] = [];

  it('skill.filesystem.rejected：severity 来自 status，副标题用 description 而非 JSON', () => {
    const view = persistedEventToView(
      t,
      makeRow({
        id: 'r1',
        key: 'skill.filesystem.rejected',
        name: 'e2e-tag-test',
        description: 'directory name mismatch',
        status: 'warn',
      }),
      traces,
    );
    expect(view.severity).toBe('warn');
    expect(view.title).toBe('e2e-tag-test');
    expect(view.subtitle).toBe('directory name mismatch');
    expect(view.category).toBe('system');
  });

  it('alert.fired：规则名为标题', () => {
    const view = persistedEventToView(
      t,
      makeRow({ key: 'alert.fired', name: '错误率超阈', status: 'warn', description: 'error_rate 0.12 > 0.1' }),
      traces,
    );
    expect(view.title).toBe('错误率超阈');
    expect(view.severity).toBe('warn');
  });

  it('runner.completion error：人话标题 + warn', () => {
    const view = persistedEventToView(
      t,
      makeRow({
        key: 'runner.completion',
        status: 'error',
        metadata_json: JSON.stringify({ status: 'error', session_id: 'sess-1', duration_ms: 1200 }),
      }),
      traces,
    );
    expect(view.title).toBe('monitorPage.events.title.chatFailed');
    expect(view.severity).toBe('warn');
  });

  it('无 description 时不回退 JSON.stringify', () => {
    const view = persistedEventToView(
      t,
      makeRow({ key: 'skill.filesystem.imported', name: 'xlsx', status: 'info', config_json: '{"a":1}' }),
      traces,
    );
    expect(view.subtitle).not.toContain('{');
  });
});
