import { describe, expect, it } from 'vitest';
import type { Envelope } from '../envelope';
import {
  activityMessageId,
  cancelRunningToolMessages,
  envelopeToToolEvent,
  mergeToolEvents,
  upsertToolMessage,
} from '../envelopeToolCall';

describe('envelopeToolCall', () => {
  it('maps v2 tool_call fields', () => {
    const env: Envelope = {
      id: 'env-1',
      type: 'tool_call',
      author: 'agent-a',
      session_id: 'sess-1',
      timestamp: '2026-05-20T10:00:00Z',
      version: 1,
      tool_call: {
        id: 'tc-1',
        name: 'skill_run',
        arguments_json: '{"skill":"planning"}',
        status: 'calling',
        activity_kind: 'skill',
        display_label: '运行 Skill',
        icon_key: 'play_circle',
        summary: 'planning',
        started_at: '2026-05-20T10:00:00Z',
      },
    };
    const event = envelopeToToolEvent(env, 'before');
    expect(event?.status).toBe('running');
    expect(event?.activity_kind).toBe('skill');
    expect(event?.display_label).toBe('运行 Skill');
    expect(event?.summary).toBe('planning');
    expect(activityMessageId(event!)).toBe('act-tc-1');
  });

  it('merges tool_result into existing card', () => {
    const before = envelopeToToolEvent(
      {
        id: 'env-1',
        type: 'tool_call',
        author: 'agent-a',
        session_id: 'sess-1',
        timestamp: '2026-05-20T10:00:00Z',
        version: 1,
        tool_call: {
          id: 'tc-1',
          name: 'read_file',
          arguments_json: '{"path":"/tmp/a.txt"}',
          status: 'calling',
        },
      },
      'before',
    )!;
    const after = envelopeToToolEvent(
      {
        id: 'env-2',
        type: 'tool_result',
        author: 'agent-a',
        session_id: 'sess-1',
        timestamp: '2026-05-20T10:00:01Z',
        version: 1,
        tool_call: {
          id: 'tc-1',
          name: 'read_file',
          arguments_json: '{"path":"/tmp/a.txt"}',
          result_json: '{"content":"hello"}',
          status: 'success',
          duration_ms: 1200,
        },
      },
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
    const callEnv: Envelope = {
      id: 'env-1',
      type: 'tool_call',
      author: 'agent-a',
      session_id: sessionId,
      timestamp: '2026-05-20T10:00:00Z',
      version: 1,
      tool_call: {
        id: 'tc-9',
        name: 'mcp_call',
        arguments_json: '{"server_key":"github","tool_name":"list_issues"}',
        status: 'calling',
        activity_kind: 'mcp',
        summary: 'github/list_issues',
      },
    };
    const resultEnv: Envelope = {
      id: 'env-2',
      type: 'tool_result',
      author: 'agent-a',
      session_id: sessionId,
      timestamp: '2026-05-20T10:00:01Z',
      version: 1,
      tool_call: {
        id: 'tc-9',
        name: 'mcp_call',
        arguments_json: '{"server_key":"github","tool_name":"list_issues"}',
        result_json: '{"ok":true}',
        status: 'success',
        duration_ms: 320,
      },
    };
    let messages = upsertToolMessage([], sessionId, callEnv, 'before');
    expect(messages).toHaveLength(1);
    expect(messages[0].id).toBe('act-tc-9');
    messages = upsertToolMessage(messages, sessionId, resultEnv, 'after');
    expect(messages).toHaveLength(1);
    expect(messages[0].status).toBe('tool_success');
    expect(messages[0].latency_ms).toBe(320);
    const opts = JSON.parse(messages[0].options_json) as { schema?: string };
    expect(opts.schema).toBe('chat.activity/v1');
  });

  it('cancels running tool cards on stop', () => {
    const sessionId = 'sess-1';
    const running = upsertToolMessage(
      [],
      sessionId,
      {
        id: 'env-1',
        type: 'tool_call',
        author: 'agent-a',
        session_id: sessionId,
        timestamp: '2026-05-20T10:00:00Z',
        version: 1,
        tool_call: { id: 'tc-cancel', name: 'read_file', arguments_json: '{}', status: 'calling' },
      },
      'before',
    );
    expect(running[0].status).toBe('tool_running');
    const cancelled = cancelRunningToolMessages(running);
    expect(cancelled[0].status).toBe('tool_cancelled');
    const opts = JSON.parse(cancelled[0].options_json) as { tool_event?: { status?: string } };
    expect(opts.tool_event?.status).toBe('cancelled');
  });

  it('preserves error_code on failed tool result (regression: tc.status === "error" must not break error fallback)', () => {
    const env: Envelope = {
      id: 'env-1',
      type: 'tool_result',
      author: 'agent-a',
      session_id: 'sess-1',
      timestamp: '2026-05-20T10:00:01Z',
      version: 1,
      tool_call: {
        id: 'tc-err',
        name: 'shell_exec',
        arguments_json: '{"command":"ls"}',
        result_json: '{}',
        status: 'error',
        error_code: 'tool_error',
        duration_ms: 240,
        finished_at: '2026-05-20T10:00:01Z',
      },
    };
    const event = envelopeToToolEvent(env, 'after');
    expect(event?.status).toBe('failed');
    expect(event?.error_code).toBe('tool_error');
    expect(event?.error).toBe('tool_error');
  });

  it('prefers result.error string over error_code', () => {
    const env: Envelope = {
      id: 'env-1',
      type: 'tool_result',
      author: 'agent-a',
      session_id: 'sess-1',
      timestamp: '2026-05-20T10:00:01Z',
      version: 1,
      tool_call: {
        id: 'tc-err2',
        name: 'shell_exec',
        arguments_json: '{}',
        result_json: '{"error":"exit code 1: file not found"}',
        status: 'error',
        error_code: 'tool_error',
        duration_ms: 100,
      },
    };
    const event = envelopeToToolEvent(env, 'after');
    expect(event?.error).toBe('exit code 1: file not found');
  });

  it('preserves array arguments (regression: parseJSONRecord used to wrap arrays as {value: [...]})', () => {
    const env: Envelope = {
      id: 'env-1',
      type: 'tool_call',
      author: 'agent-a',
      session_id: 'sess-1',
      timestamp: '2026-05-20T10:00:00Z',
      version: 1,
      tool_call: {
        id: 'tc-arr',
        name: 'batch_task',
        arguments_json: '[{"id":1},{"id":2}]',
        status: 'calling',
      },
    };
    const event = envelopeToToolEvent(env, 'before');
    expect(Array.isArray(event?.arguments)).toBe(true);
    expect((event?.arguments as unknown[]).length).toBe(2);
  });

  it('truncates oversized arguments_json to a safe preview (regression: SEC-04-style LLM payload bomb)', () => {
    // A malicious or pathological LLM response might emit a giant arguments
    // blob. The front end must not allocate the full string into the message
    // store / Vue diffing layer without truncation. Mirrors backend
    // biz.redactActivityJSON (512-byte limit).
    const huge = 'a'.repeat(2000);
    const env: Envelope = {
      id: 'env-1',
      type: 'tool_call',
      author: 'agent-a',
      session_id: 'sess-1',
      timestamp: '2026-05-20T10:00:00Z',
      version: 1,
      tool_call: {
        id: 'tc-huge',
        name: 'read_file',
        arguments_json: huge,
        status: 'calling',
      },
    };
    const event = envelopeToToolEvent(env, 'before');
    const raw = (event?.arguments as { __raw?: string } | undefined)?.__raw;
    expect(raw).toBeDefined();
    expect(raw!.length).toBeLessThanOrEqual(600);
    expect(raw).toContain('truncated');
  });
});
