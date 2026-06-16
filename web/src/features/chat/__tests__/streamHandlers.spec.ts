import { describe, expect, it, vi } from 'vitest';
import { EnvelopeDispatcher } from '../dispatcher';
import type { Envelope } from '../envelope';
import { bindStreamHandlers, patchStreamingEnvelope } from '../streamHandlers';
import type { Message } from '../types';

function env(partial: Partial<Envelope> & { type: Envelope['type'] }): Envelope {
  return {
    id: 'e1',
    author: 'test',
    session_id: 'sess-1',
    timestamp: '',
    version: 1,
    ...partial,
  };
}

describe('bindStreamHandlers', () => {
  it('ignores deltas for inactive session', () => {
    const rows: Record<string, Message[]> = { 'sess-1': [] };
    const dispatcher = new EnvelopeDispatcher();
    const stream = {
      onType: (type: string | string[], handler: (e: Envelope) => void) =>
        dispatcher.onType(type as Envelope['type'] | Envelope['type'][], handler),
    };

    bindStreamHandlers(stream as any, {
      sessionId: 'sess-1',
      resolveActiveSessionId: () => 'sess-2',
      getMessages: (sid) => rows[sid] ?? [],
      setMessages: (sid, next) => {
        rows[sid] = next;
      },
      markSendingDone: vi.fn(),
      onRunStatus: vi.fn(),
      onErrorNotify: vi.fn(),
      onReloadAfterCompletion: vi.fn(),
    });

    dispatcher.dispatch(
      env({
        type: 'activity_delta',
        metadata: { delta_field: 'content_markdown', delta_chunk: 'skip', activity_id: 'act-1' },
      }),
    );
    expect(rows['sess-1']).toEqual([]);
  });

  it('patchStreamingEnvelope creates and updates streaming row', () => {
    const next = patchStreamingEnvelope(
      [],
      'sess-1',
      'ws-stream-sess-1',
      {
        id: 'e1',
        type: 'text_delta',
        author: 'test',
        session_id: 'sess-1',
        timestamp: '',
        version: 1,
        content: { text: 'Hi', reasoning: '', is_partial: true },
      },
      false,
    );
    expect(next).toHaveLength(1);
    expect(next[0]?.content_markdown).toBe('Hi');
    expect(next[0]?.status).toBe('streaming');
  });

  it('calls onSessionContextPatch on context_usage mid-turn', () => {
    const onSessionContextPatch = vi.fn();
    const dispatcher = new EnvelopeDispatcher();
    const stream = {
      onType: (type: string | string[], handler: (e: Envelope) => void) =>
        dispatcher.onType(type as Envelope['type'] | Envelope['type'][], handler),
    };

    bindStreamHandlers(stream as any, {
      sessionId: 'sess-1',
      resolveActiveSessionId: () => 'sess-1',
      getMessages: () => [],
      setMessages: vi.fn(),
      markSendingDone: vi.fn(),
      onRunStatus: vi.fn(),
      onErrorNotify: vi.fn(),
      onReloadAfterCompletion: vi.fn(),
      onSessionContextPatch,
    });

    dispatcher.dispatch(
      env({
        type: 'context_usage',
        usage: {
          prompt_tokens: 80_000,
          context_prompt_tokens: 80_000,
          completion_tokens: 100,
          total_tokens: 80_100,
          max_tokens: 128_000,
          turn_total_tokens: 80_100,
        },
      }),
    );

    expect(onSessionContextPatch).toHaveBeenCalledWith('sess-1', {
      context_used_ratio: expect.closeTo(80_000 / 128_000),
      context_used_tokens: 80_000,
      context_status: 'warning',
      last_context_window_tokens: 128_000,
    });
    expect(onSessionContextPatch.mock.calls[0]?.[1]).not.toHaveProperty('total_tokens');
  });
});
