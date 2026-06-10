import { describe, expect, it } from 'vitest';
import { mergeProgressEvents, readExecutionProgressMetadata } from '../executionProgress';
import type { Envelope } from '../../../realtime/envelope';

function env(partial: Partial<Envelope> & { metadata?: Record<string, unknown> }): Envelope {
  return {
    id: 'env-1',
    type: 'execution_progress',
    author: 'system',
    session_id: 's1',
    timestamp: '2026-06-10T10:00:00.000Z',
    version: 1,
    ...partial,
  } as Envelope;
}

describe('readExecutionProgressMetadata', () => {
  it('returns null for non-execution_progress envelopes', () => {
    expect(
      readExecutionProgressMetadata(env({ type: 'text_delta' as unknown as 'execution_progress' })),
    ).toBeNull();
  });

  it('returns null when metadata is missing', () => {
    expect(readExecutionProgressMetadata(env({ metadata: undefined }))).toBeNull();
  });

  it('returns null when required fields are missing', () => {
    expect(
      readExecutionProgressMetadata(
        env({ metadata: { phase: 'start', message: 'x', category: 'orchestration' } }),
      ),
    ).toBeNull();
  });

  it('returns parsed metadata with all known optional fields', () => {
    const out = readExecutionProgressMetadata(
      env({
        metadata: {
          step_id: 'chat.llm.invoke',
          phase: 'done',
          message: '语言模型已返回',
          category: 'orchestration',
          duration_ms: 1234,
          agent_key: 'agent-1',
          tool_name: 'shell_exec',
          error: 'some non-empty error',
        },
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

  // S3 regression: empty string optional fields must NOT be included in the
  // output object. Downstream code uses `if (out.error)` truthy checks; an
  // empty string would be truthy in JS but semantically a "no value" marker.
  it('omits empty-string optional fields (error / agent_key / tool_name)', () => {
    const out = readExecutionProgressMetadata(
      env({
        metadata: {
          step_id: 'chat.llm.invoke',
          phase: 'start',
          message: '正在调用',
          category: 'orchestration',
          error: '',
          agent_key: '',
          tool_name: '',
        },
      }),
    );
    expect(out).toEqual({
      step_id: 'chat.llm.invoke',
      phase: 'start',
      message: '正在调用',
      category: 'orchestration',
    });
    expect(Object.prototype.hasOwnProperty.call(out, 'error')).toBe(false);
    expect(Object.prototype.hasOwnProperty.call(out, 'agent_key')).toBe(false);
    expect(Object.prototype.hasOwnProperty.call(out, 'tool_name')).toBe(false);
  });

  it('falls back to "orchestration" category when category is unknown', () => {
    const out = readExecutionProgressMetadata(
      env({
        metadata: {
          step_id: 's',
          phase: 'start',
          message: 'm',
          category: 'mystery',
        },
      }),
    );
    expect(out?.category).toBe('orchestration');
  });
});

describe('mergeProgressEvents', () => {
  it('creates a running section from a start envelope', () => {
    const out = mergeProgressEvents(
      [
        env({
          id: 'e1',
          timestamp: '2026-06-10T10:00:00.000Z',
          metadata: {
            step_id: 'chat.llm.invoke',
            phase: 'start',
            message: '正在调用语言模型',
            category: 'orchestration',
          },
        }),
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
    const out = mergeProgressEvents(
      [
        env({
          id: 'e1',
          timestamp: '2026-06-10T10:00:00.000Z',
          metadata: {
            step_id: 's1',
            phase: 'start',
            message: 'm1',
            category: 'orchestration',
          },
        }),
        env({
          id: 'e2',
          timestamp: '2026-06-10T10:00:01.500Z',
          metadata: {
            step_id: 's1',
            phase: 'done',
            message: 'm2',
            category: 'orchestration',
            duration_ms: 1500,
          },
        }),
      ],
      () => 0,
    );
    const section = out.get('s1')!;
    expect(section.status).toBe('done');
    expect(section.durationMs).toBe(1500);
    expect(section.message).toBe('m2');
  });

  it('computes duration from envelope timestamps when duration_ms is absent', () => {
    const out = mergeProgressEvents(
      [
        env({
          id: 'e1',
          timestamp: '2026-06-10T10:00:00.000Z',
          metadata: {
            step_id: 's1',
            phase: 'start',
            message: 'm1',
            category: 'orchestration',
          },
        }),
        env({
          id: 'e2',
          timestamp: '2026-06-10T10:00:02.000Z',
          metadata: {
            step_id: 's1',
            phase: 'done',
            message: 'm2',
            category: 'orchestration',
          },
        }),
      ],
      () => 0,
    );
    expect(out.get('s1')!.durationMs).toBe(2000);
  });

  it('marks running orchestration step as timeout past 15s (S9 per-category threshold)', () => {
    // Per-category threshold: orchestration = 15_000ms. After 16s with no
    // `done` envelope, status flips to "timeout" so the UI can show
    // "(等待中)".
    const startTs = Date.parse('2026-06-10T10:00:00.000Z');
    const out = mergeProgressEvents(
      [
        env({
          id: 'e1',
          timestamp: '2026-06-10T10:00:00.000Z',
          metadata: {
            step_id: 's1',
            phase: 'start',
            message: 'm1',
            category: 'orchestration',
          },
        }),
      ],
      () => startTs + 16_000,
    );
    expect(out.get('s1')!.status).toBe('timeout');
  });

  it('keeps running orchestration step at 14s (under 15s threshold)', () => {
    const startTs = Date.parse('2026-06-10T10:00:00.000Z');
    const out = mergeProgressEvents(
      [
        env({
          id: 'e1',
          timestamp: '2026-06-10T10:00:00.000Z',
          metadata: {
            step_id: 's1',
            phase: 'start',
            message: 'm1',
            category: 'orchestration',
          },
        }),
      ],
      () => startTs + 14_000,
    );
    expect(out.get('s1')!.status).toBe('running');
  });

  it('marks running thinking step as timeout past 8s', () => {
    const startTs = Date.parse('2026-06-10T10:00:00.000Z');
    const out = mergeProgressEvents(
      [
        env({
          id: 'e1',
          timestamp: '2026-06-10T10:00:00.000Z',
          metadata: {
            step_id: 's1',
            phase: 'start',
            message: 'm1',
            category: 'thinking',
          },
        }),
      ],
      () => startTs + 9_000,
    );
    expect(out.get('s1')!.status).toBe('timeout');
  });

  it('marks running tool step as timeout past 60s', () => {
    const startTs = Date.parse('2026-06-10T10:00:00.000Z');
    const out = mergeProgressEvents(
      [
        env({
          id: 'e1',
          timestamp: '2026-06-10T10:00:00.000Z',
          metadata: {
            step_id: 's1',
            phase: 'start',
            message: 'm1',
            category: 'tool',
          },
        }),
      ],
      () => startTs + 61_000,
    );
    expect(out.get('s1')!.status).toBe('timeout');
  });

  it('respects caller-provided per-category timeout overrides', () => {
    const startTs = Date.parse('2026-06-10T10:00:00.000Z');
    const out = mergeProgressEvents(
      [
        env({
          id: 'e1',
          timestamp: '2026-06-10T10:00:00.000Z',
          metadata: {
            step_id: 's1',
            phase: 'start',
            message: 'm1',
            category: 'orchestration',
          },
        }),
      ],
      () => startTs + 6_000,
      // Override: 5s for orchestration (test scenario)
      { orchestration: 5_000, team: 30_000, tool: 60_000, thinking: 8_000 },
    );
    expect(out.get('s1')!.status).toBe('timeout');
  });

  it('respects null override to disable auto-timeout for a category', () => {
    const startTs = Date.parse('2026-06-10T10:00:00.000Z');
    const out = mergeProgressEvents(
      [
        env({
          id: 'e1',
          timestamp: '2026-06-10T10:00:00.000Z',
          metadata: {
            step_id: 's1',
            phase: 'start',
            message: 'm1',
            category: 'orchestration',
          },
        }),
      ],
      () => startTs + 60_000,
      // Disable timeout for orchestration (e.g., during long-running LLM)
      { orchestration: null, team: 30_000, tool: 60_000, thinking: 8_000 },
    );
    expect(out.get('s1')!.status).toBe('running');
  });

  it('disables auto-timeout globally when every category override is null', () => {
    // R3-T2 follow-up: callers can opt out of the UI's "(等待中)" hint
    // entirely by passing `null` for every category. This is the "show me
    // raw timeline, don't make assumptions" mode.
    const startTs = Date.parse('2026-06-10T10:00:00.000Z');
    const out = mergeProgressEvents(
      [
        env({
          id: 'e1',
          timestamp: '2026-06-10T10:00:00.000Z',
          metadata: {
            step_id: 'tool-1',
            phase: 'start',
            message: 'tool running',
            category: 'tool',
          },
        }),
      ],
      () => startTs + 10 * 60_000, // 10 minutes, well past any default
      { orchestration: null, team: null, tool: null, thinking: null },
    );
    expect(out.get('tool-1')!.status).toBe('running');
  });

  it('marks error phase as failed', () => {
    const out = mergeProgressEvents(
      [
        env({
          id: 'e1',
          timestamp: '2026-06-10T10:00:00.000Z',
          metadata: {
            step_id: 's1',
            phase: 'start',
            message: 'm1',
            category: 'orchestration',
          },
        }),
        env({
          id: 'e2',
          timestamp: '2026-06-10T10:00:00.500Z',
          metadata: {
            step_id: 's1',
            phase: 'error',
            message: 'failed',
            category: 'orchestration',
          },
        }),
      ],
      () => 0,
    );
    expect(out.get('s1')!.status).toBe('failed');
  });

  it('keeps separate sections for different step ids', () => {
    const out = mergeProgressEvents(
      [
        env({
          id: 'e1',
          metadata: {
            step_id: 'a',
            phase: 'start',
            message: 'A',
            category: 'orchestration',
          },
        }),
        env({
          id: 'e2',
          metadata: {
            step_id: 'b',
            phase: 'start',
            message: 'B',
            category: 'team',
          },
        }),
      ],
      () => 0,
    );
    expect(out.size).toBe(2);
    expect(out.get('a')!.category).toBe('orchestration');
    expect(out.get('b')!.category).toBe('team');
  });

  it('ignores envelopes that fail metadata validation', () => {
    const out = mergeProgressEvents(
      [
        env({
          id: 'e1',
          metadata: { step_id: 'a' }, // missing phase/message/category
        }),
      ],
      () => 0,
    );
    expect(out.size).toBe(0);
  });
});
