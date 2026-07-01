import { describe, expect, it } from 'vitest';
import { mergeProgressEventsFromActivity, readExecutionProgressMetadataFromActivity } from '../executionProgress';
import type { ActivityEvent, Activity } from '../../../realtime/activityEvent';

function makeActivity(overrides: Partial<Activity> = {}): Activity {
  return {
    id: 'act-1',
    kind: 'task',
    status: 'running',
    session_id: 's1',
    turn_id: 'turn-1',
    parent_activity_id: '',
    timestamp: '2026-06-10T10:00:00.000Z',
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
    stage: 'execution_progress',
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

function activityEvent(meta: Record<string, unknown>, overrides: Partial<Activity> = {}): ActivityEvent {
  return {
    event: 'updated',
    activity: makeActivity({ meta, ...overrides }),
  };
}

// ── ActivityEvent-based functions ──────────────────────────────────────
describe('readExecutionProgressMetadataFromActivity', () => {
  it('returns null for non-execution_progress stage', () => {
    expect(readExecutionProgressMetadataFromActivity(activityEvent({}, { stage: 'run_status' }))).toBeNull();
  });

  it('returns null when meta is missing required fields', () => {
    expect(readExecutionProgressMetadataFromActivity(activityEvent({ phase: 'start', message: 'x' }))).toBeNull();
  });

  it('returns parsed metadata with optional fields', () => {
    const out = readExecutionProgressMetadataFromActivity(
      activityEvent({
        step_id: 'chat.llm.invoke',
        phase: 'done',
        message: '语言模型已返回',
        category: 'orchestration',
        duration_ms: 1234,
        agent_key: 'agent-1',
        tool_name: 'shell_exec',
        error: 'some non-empty error',
      }),
    );
    expect(out).toEqual({
      step_id: 'chat.llm.invoke',
      phase: 'done',
      message: '语言模型已返回',
      category: 'orchestration',
      duration_ms: 1234,
      agent_key: 'agent-1',
      tool_name: 'shell_exec',
      error: 'some non-empty error',
    });
  });

  it('omits empty-string optional fields', () => {
    const out = readExecutionProgressMetadataFromActivity(
      activityEvent({
        step_id: 'chat.llm.invoke',
        phase: 'start',
        message: '正在调用',
        category: 'orchestration',
        error: '',
        agent_key: '',
        tool_name: '',
      }),
    );
    expect(out).toEqual({
      step_id: 'chat.llm.invoke',
      phase: 'start',
      message: '正在调用',
      category: 'orchestration',
    });
  });

  it('falls back to "orchestration" category when category is unknown', () => {
    const out = readExecutionProgressMetadataFromActivity(
      activityEvent({ step_id: 's', phase: 'start', message: 'm', category: 'mystery' }),
    );
    expect(out?.category).toBe('orchestration');
  });
});

describe('mergeProgressEventsFromActivity', () => {
  it('creates a running section from a start event', () => {
    const out = mergeProgressEventsFromActivity(
      [
        activityEvent(
          { step_id: 'chat.llm.invoke', phase: 'start', message: '正在调用语言模型', category: 'orchestration' },
          { timestamp: '2026-06-10T10:00:00.000Z' },
        ),
      ],
      () => 0,
    );
    expect(out.size).toBe(1);
    const section = out.get('chat.llm.invoke')!;
    expect(section.status).toBe('running');
    expect(section.message).toBe('正在调用语言模型');
    expect(section.category).toBe('orchestration');
    expect(section.durationMs).toBeNull();
  });

  it('transitions to done and captures duration_ms when present', () => {
    const out = mergeProgressEventsFromActivity(
      [
        activityEvent(
          { step_id: 's1', phase: 'start', message: 'm1', category: 'orchestration' },
          { timestamp: '2026-06-10T10:00:00.000Z' },
        ),
        activityEvent(
          { step_id: 's1', phase: 'done', message: 'm2', category: 'orchestration', duration_ms: 1500 },
          { timestamp: '2026-06-10T10:00:01.500Z' },
        ),
      ],
      () => 0,
    );
    const section = out.get('s1')!;
    expect(section.status).toBe('done');
    expect(section.durationMs).toBe(1500);
    expect(section.message).toBe('m2');
  });

  it('computes duration from activity timestamps when duration_ms is absent', () => {
    const out = mergeProgressEventsFromActivity(
      [
        activityEvent(
          { step_id: 's1', phase: 'start', message: 'm1', category: 'orchestration' },
          { timestamp: '2026-06-10T10:00:00.000Z' },
        ),
        activityEvent(
          { step_id: 's1', phase: 'done', message: 'm2', category: 'orchestration' },
          { timestamp: '2026-06-10T10:00:02.000Z' },
        ),
      ],
      () => 0,
    );
    expect(out.get('s1')!.durationMs).toBe(2000);
  });

  it('marks running orchestration step as timeout past 15s (S9 per-category threshold)', () => {
    const startTs = Date.parse('2026-06-10T10:00:00.000Z');
    const out = mergeProgressEventsFromActivity(
      [
        activityEvent(
          { step_id: 's1', phase: 'start', message: 'm1', category: 'orchestration' },
          { timestamp: '2026-06-10T10:00:00.000Z' },
        ),
      ],
      () => startTs + 16_000,
    );
    expect(out.get('s1')!.status).toBe('timeout');
  });

  it('marks error phase as failed', () => {
    const out = mergeProgressEventsFromActivity(
      [
        activityEvent(
          { step_id: 's1', phase: 'start', message: 'm1', category: 'orchestration' },
          { timestamp: '2026-06-10T10:00:00.000Z' },
        ),
        activityEvent(
          { step_id: 's1', phase: 'error', message: 'failed', category: 'orchestration' },
          { timestamp: '2026-06-10T10:00:00.500Z' },
        ),
      ],
      () => 0,
    );
    expect(out.get('s1')!.status).toBe('failed');
  });

  it('ignores events that fail metadata validation', () => {
    const out = mergeProgressEventsFromActivity(
      [activityEvent({ step_id: 'a' })], // missing phase/message/category
      () => 0,
    );
    expect(out.size).toBe(0);
  });

  it('respects caller-provided per-category timeout overrides', () => {
    const startTs = Date.parse('2026-06-10T10:00:00.000Z');
    const out = mergeProgressEventsFromActivity(
      [
        activityEvent(
          { step_id: 's1', phase: 'start', message: 'm1', category: 'orchestration' },
          { timestamp: '2026-06-10T10:00:00.000Z' },
        ),
      ],
      () => startTs + 6_000,
      { orchestration: 5_000, team: 30_000, tool: 60_000, thinking: 8_000 },
    );
    expect(out.get('s1')!.status).toBe('timeout');
  });
});
