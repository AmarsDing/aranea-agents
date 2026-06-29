import { describe, expect, it } from 'vitest';
import type { ActivityEvent, Activity, ActivityEventType } from '../../../realtime/activityEvent';
import {
  activityMessageId,
  activityToToolEvent,
  cancelRunningToolMessages,
  mergeToolEvents,
  upsertToolMessageFromActivity,
} from '../activityToolCall';

function makeActivity(overrides: Partial<Activity> = {}): Activity {
  return {
    id: 'act-1',
    kind: 'action',
    status: 'running',
    session_id: 'sess-1',
    turn_id: 'turn-1',
    parent_activity_id: '',
    timestamp: '2026-05-20T10:00:00Z',
    duration_ms: 0,
    seq: 1,
    prompt_tokens: 0,
    completion_tokens: 0,
    content: '',
    reasoning: '',
    tool_name: '',
    tool_category: 'other',
    tool_call_id: '',
    tool_arguments: '',
    tool_result: '',
    tool_duration_ms: 0,
    tool_error_code: '',
    stage: '',
    child_board_id: '',
    spirit_session_id: '',
    team_id: '',
    dag_node_id: '',
    depends_on: [],
    agent_key: '',
    agent_name: '',
    collapsed: false,
    label: '',
    meta: {},
    ...overrides,
  };
}

function makeActivityEvent(
  event: ActivityEventType,
  activityOverrides: Partial<Activity> = {},
): ActivityEvent {
  return { event, activity: makeActivity(activityOverrides) };
}

describe('activityToolCall', () => {
  it('maps activity tool_* fields to ToolUseEvent (before phase)', () => {
    const ev = makeActivityEvent('created', {
      tool_name: 'skill_run',
      tool_call_id: 'tc-1',
      tool_arguments: '{"skill":"planning"}',
      status: 'running',
      agent_key: 'agent-a',
      agent_name: 'Agent A',
      label: '运行 Skill',
      timestamp: '2026-05-20T10:00:00Z',
      meta: { activity_kind: 'skill', icon_key: 'play_circle', summary: 'planning', started_at: '2026-05-20T10:00:00Z' },
    });
    const event = activityToToolEvent(ev, 'before');
    expect(event?.status).toBe('running');
    expect(event?.activity_kind).toBe('skill');
    expect(event?.display_label).toBe('运行 Skill');
    expect(event?.summary).toBe('planning');
    expect(event?.icon_key).toBe('play_circle');
    expect(activityMessageId(event!)).toBe('act-tc-1');
  });

  it('returns null for non-action activity kind', () => {
    const ev = makeActivityEvent('created', { kind: 'task' });
    expect(activityToToolEvent(ev, 'before')).toBeNull();
  });

  it('returns null when both tool_name and tool_call_id are empty', () => {
    const ev = makeActivityEvent('created', { tool_name: '', tool_call_id: '' });
    expect(activityToToolEvent(ev, 'before')).toBeNull();
  });

  it('merges tool_result into existing card (created + completed)', () => {
    const before = activityToToolEvent(
      makeActivityEvent('created', {
        id: 'act-1',
        tool_name: 'read_file',
        tool_call_id: 'tc-1',
        tool_arguments: '{"path":"/tmp/a.txt"}',
        status: 'running',
        agent_key: 'agent-a',
      }),
      'before',
    )!;
    const after = activityToToolEvent(
      makeActivityEvent('completed', {
        id: 'act-2',
        tool_name: 'read_file',
        tool_call_id: 'tc-1',
        tool_arguments: '{"path":"/tmp/a.txt"}',
        tool_result: '{"content":"hello"}',
        status: 'completed',
        tool_duration_ms: 1200,
        agent_key: 'agent-a',
        timestamp: '2026-05-20T10:00:01Z',
      }),
      'after',
    )!;
    const merged = mergeToolEvents(before, after);
    expect(merged.status).toBe('success');
    expect(merged.duration_ms).toBe(1200);
    expect((merged.arguments as { path?: string } | undefined)?.path).toBe('/tmp/a.txt');
    expect((merged.result as { content?: string } | undefined)?.content).toBe('hello');
  });

  it('upserts by act-{id} without duplicates', () => {
    const sessionId = 'sess-1';
    const callEv = makeActivityEvent('created', {
      tool_name: 'mcp_call',
      tool_call_id: 'tc-9',
      tool_arguments: '{"server_key":"github","tool_name":"list_issues"}',
      status: 'running',
      agent_key: 'agent-a',
      meta: { activity_kind: 'mcp', summary: 'github/list_issues' },
    });
    const resultEv = makeActivityEvent('completed', {
      tool_name: 'mcp_call',
      tool_call_id: 'tc-9',
      tool_arguments: '{"server_key":"github","tool_name":"list_issues"}',
      tool_result: '{"ok":true}',
      status: 'completed',
      tool_duration_ms: 320,
      agent_key: 'agent-a',
      timestamp: '2026-05-20T10:00:01Z',
    });
    let messages = upsertToolMessageFromActivity([], sessionId, callEv);
    expect(messages).toHaveLength(1);
    expect(messages[0].id).toBe('act-tc-9');
    messages = upsertToolMessageFromActivity(messages, sessionId, resultEv);
    expect(messages).toHaveLength(1);
    expect(messages[0].status).toBe('tool_success');
    expect(messages[0].latency_ms).toBe(320);
    const opts = JSON.parse(messages[0].options_json) as { schema?: string };
    expect(opts.schema).toBe('chat.activity/v1');
  });

  it('cancels running tool cards on stop', () => {
    const sessionId = 'sess-1';
    const callEv = makeActivityEvent('created', {
      tool_name: 'read_file',
      tool_call_id: 'tc-cancel',
      tool_arguments: '{}',
      status: 'running',
      agent_key: 'agent-a',
    });
    const running = upsertToolMessageFromActivity([], sessionId, callEv);
    expect(running[0].status).toBe('tool_running');
    const cancelled = cancelRunningToolMessages(running);
    expect(cancelled[0].status).toBe('tool_cancelled');
    const opts = JSON.parse(cancelled[0].options_json) as { tool_event?: { status?: string } };
    expect(opts.tool_event?.status).toBe('cancelled');
  });

  it('preserves error_code on failed tool result (activity status=failed)', () => {
    const ev = makeActivityEvent('failed', {
      tool_name: 'shell_exec',
      tool_call_id: 'tc-err',
      tool_arguments: '{"command":"ls"}',
      tool_result: '{}',
      status: 'failed',
      tool_error_code: 'tool_error',
      tool_duration_ms: 240,
      timestamp: '2026-05-20T10:00:01Z',
    });
    const event = activityToToolEvent(ev, 'after');
    expect(event?.status).toBe('failed');
    expect(event?.error_code).toBe('tool_error');
    expect(event?.error).toBe('tool_error');
  });

  it('prefers result.error string over error_code', () => {
    const ev = makeActivityEvent('failed', {
      tool_name: 'shell_exec',
      tool_call_id: 'tc-err2',
      tool_arguments: '{}',
      tool_result: '{"error":"exit code 1: file not found"}',
      status: 'failed',
      tool_error_code: 'tool_error',
      tool_duration_ms: 100,
    });
    const event = activityToToolEvent(ev, 'after');
    expect(event?.error).toBe('exit code 1: file not found');
  });

  it('preserves array arguments (regression: parseJSONRecord used to wrap arrays as {value: [...]})', () => {
    const ev = makeActivityEvent('created', {
      tool_name: 'batch_task',
      tool_call_id: 'tc-arr',
      tool_arguments: '[{"id":1},{"id":2}]',
      status: 'running',
    });
    const event = activityToToolEvent(ev, 'before');
    expect(Array.isArray(event?.arguments)).toBe(true);
    expect((event?.arguments as unknown[]).length).toBe(2);
  });

  it('truncates oversized tool_arguments to a safe preview (regression: SEC-04-style LLM payload bomb)', () => {
    const huge = 'a'.repeat(2000);
    const ev = makeActivityEvent('created', {
      tool_name: 'read_file',
      tool_call_id: 'tc-huge',
      tool_arguments: huge,
      status: 'running',
    });
    const event = activityToToolEvent(ev, 'before');
    const raw = (event?.arguments as { __raw?: string } | undefined)?.__raw;
    expect(raw).toBeDefined();
    expect(raw!.length).toBeLessThanOrEqual(600);
    expect(raw).toContain('truncated');
  });

  it('maps ActivityStatus partial_failure to wire failed', () => {
    const ev = makeActivityEvent('failed', {
      tool_name: 'shell_exec',
      tool_call_id: 'tc-pf',
      status: 'partial_failure',
    });
    const event = activityToToolEvent(ev, 'after');
    expect(event?.status).toBe('failed');
  });

  it('maps ActivityStatus tool_blocked to wire blocked', () => {
    const ev = makeActivityEvent('created', {
      tool_name: 'shell_exec',
      tool_call_id: 'tc-blk',
      status: 'tool_blocked',
    });
    const event = activityToToolEvent(ev, 'before');
    expect(event?.status).toBe('blocked');
  });

  it('falls back to activity.label then meta.display_label then tool_name for display_label', () => {
    const withLabel = activityToToolEvent(
      makeActivityEvent('created', {
        tool_name: 'shell_exec',
        tool_call_id: 'tc-1',
        label: '执行 Shell',
      }),
      'before',
    );
    expect(withLabel?.display_label).toBe('执行 Shell');

    const withMetaLabel = activityToToolEvent(
      makeActivityEvent('created', {
        tool_name: 'shell_exec',
        tool_call_id: 'tc-2',
        label: '',
        meta: { display_label: 'Meta Label' },
      }),
      'before',
    );
    expect(withMetaLabel?.display_label).toBe('Meta Label');

    const fallback = activityToToolEvent(
      makeActivityEvent('created', {
        tool_name: 'shell_exec',
        tool_call_id: 'tc-3',
        label: '',
      }),
      'before',
    );
    expect(fallback?.display_label).toBe('shell_exec');
  });

  it('defaults activity_kind to "tool" when meta.activity_kind is absent', () => {
    const ev = makeActivityEvent('created', {
      tool_name: 'read_file',
      tool_call_id: 'tc-default',
      status: 'running',
    });
    const event = activityToToolEvent(ev, 'before');
    expect(event?.activity_kind).toBe('tool');
  });
});
