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
  it('patches text_delta then text_done and completes with reload', async () => {
    const rows: Record<string, Message[]> = { 'sess-1': [] };
    const markSendingDone = vi.fn();
    const onReloadAfterCompletion = vi.fn().mockResolvedValue(undefined);
    const dispatcher = new EnvelopeDispatcher();

    const stream = {
      onType: (type: string | string[], handler: (e: Envelope) => void) =>
        dispatcher.onType(type as Envelope['type'] | Envelope['type'][], handler),
    };

    bindStreamHandlers(stream as any, {
      sessionId: 'sess-1',
      resolveActiveSessionId: () => 'sess-1',
      getMessages: (sid) => rows[sid] ?? [],
      setMessages: (sid, next) => {
        rows[sid] = next;
      },
      markSendingDone,
      onRunStatus: vi.fn(),
      onErrorNotify: vi.fn(),
      onReloadAfterCompletion,
      setLastIntentPass: vi.fn(),
    });

    dispatcher.dispatch(
      env({
        type: 'text_delta',
        content: { text: 'Hello', reasoning: '', is_partial: true },
      }),
    );
    expect(rows['sess-1']!.some((m) => m.id === 'ws-stream-sess-1')).toBe(true);
    expect(rows['sess-1']!.find((m) => m.id === 'ws-stream-sess-1')?.content_markdown).toBe('Hello');

    dispatcher.dispatch(
      env({
        type: 'text_done',
        content: { text: 'Hello world', reasoning: '', is_partial: false },
      }),
    );
    expect(rows['sess-1']!.find((m) => m.id === 'ws-stream-sess-1')?.status).toBe('ok');

    dispatcher.dispatch(env({ type: 'runner_completion' }));
    expect(markSendingDone).toHaveBeenCalled();
    expect(onReloadAfterCompletion).toHaveBeenCalledWith('sess-1');
  });

  it('skips channel-owned stream and reload on session WS', () => {
    const rows: Record<string, Message[]> = { 'sess-1': [] };
    const markSendingDone = vi.fn();
    const onReloadAfterCompletion = vi.fn().mockResolvedValue(undefined);
    const dispatcher = new EnvelopeDispatcher();
    const stream = {
      onType: (type: string | string[], handler: (e: Envelope) => void) =>
        dispatcher.onType(type as Envelope['type'] | Envelope['type'][], handler),
    };

    bindStreamHandlers(stream as any, {
      sessionId: 'sess-1',
      resolveActiveSessionId: () => 'sess-1',
      getMessages: (sid) => rows[sid] ?? [],
      setMessages: (sid, next) => {
        rows[sid] = next;
      },
      markSendingDone,
      onRunStatus: vi.fn(),
      onErrorNotify: vi.fn(),
      onReloadAfterCompletion,
      setLastIntentPass: vi.fn(),
    });

    dispatcher.dispatch(
      env({
        type: 'text_delta',
        source: 'channel',
        content: { text: 'skip', reasoning: '', is_partial: true },
      }),
    );
    expect(rows['sess-1']).toEqual([]);

    dispatcher.dispatch(env({ type: 'runner_completion', source: 'channel' }));
    expect(markSendingDone).toHaveBeenCalled();
    expect(onReloadAfterCompletion).not.toHaveBeenCalled();
  });

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
      setLastIntentPass: vi.fn(),
    });

    dispatcher.dispatch(
      env({
        type: 'text_delta',
        content: { text: 'skip', reasoning: '', is_partial: true },
      }),
    );
    expect(rows['sess-1']).toEqual([]);
  });

  it('marks pending user failed and reloads on stream error', async () => {
    const pending = {
      id: 'pending-user-1',
      session_id: 'sess-1',
      parent_message_id: '',
      turn_id: '',
      turn_number: 1,
      seq_in_turn: 0,
      role: 'user',
      content_markdown: 'hello',
      model_name: 'mock',
      token_in: 0,
      token_out: 0,
      latency_ms: 0,
      status: 'ok',
      attachments_count: 0,
      options_json: '',
      error_message: '',
      created_at: '',
    } satisfies Message;
    const rows: Record<string, Message[]> = { 'sess-1': [pending] };
    const onReloadAfterCompletion = vi.fn().mockResolvedValue(undefined);
    const dispatcher = new EnvelopeDispatcher();
    const stream = {
      onType: (type: string | string[], handler: (e: Envelope) => void) =>
        dispatcher.onType(type as Envelope['type'] | Envelope['type'][], handler),
    };

    bindStreamHandlers(stream as any, {
      sessionId: 'sess-1',
      resolveActiveSessionId: () => 'sess-1',
      getMessages: (sid) => rows[sid] ?? [],
      setMessages: (sid, next) => {
        rows[sid] = next;
      },
      markSendingDone: vi.fn(),
      onRunStatus: vi.fn(),
      onErrorNotify: vi.fn(),
      onReloadAfterCompletion,
      setLastIntentPass: vi.fn(),
    });

    dispatcher.dispatch(
      env({
        type: 'error',
        request_id: 'pending-user-1',
        error: { type: 'LLM_CALL_FAILED', message: '模型调用失败' },
      }),
    );
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(rows['sess-1']?.[0]?.status).toBe('failed');
    expect(rows['sess-1']?.[0]?.error_message).toBe('模型调用失败');
    expect(onReloadAfterCompletion).toHaveBeenCalledWith('sess-1');
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
      setLastIntentPass: vi.fn(),
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

  it('calls onSessionContextPatch on runner_completion usage', () => {
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
      onReloadAfterCompletion: vi.fn().mockResolvedValue(undefined),
      setLastIntentPass: vi.fn(),
      onSessionContextPatch,
      getSessionMetrics: () => ({
        input_tokens: 500,
        output_tokens: 500,
        total_tokens: 1000,
        max_context_used_ratio: 0.1,
      }),
    });

    dispatcher.dispatch(
      env({
        type: 'runner_completion',
        usage: {
          prompt_tokens: 64_000,
          context_prompt_tokens: 64_000,
          completion_tokens: 512,
          total_tokens: 64_512,
          max_tokens: 128_000,
          turn_total_tokens: 64_512,
        },
      }),
    );

    expect(onSessionContextPatch).toHaveBeenCalledWith(
      'sess-1',
      expect.objectContaining({
        context_used_ratio: expect.closeTo(0.5),
        context_status: 'normal',
        total_tokens: 1000 + 64_512,
      }),
    );
  });

  it('calls onSessionContextPatch on compress text_done', () => {
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
      setLastIntentPass: vi.fn(),
      onSessionContextPatch,
    });

    dispatcher.dispatch(
      env({
        type: 'text_done',
        content: { text: '会话上下文已自动压缩', reasoning: '', is_partial: false },
        metadata: {
          kind: 'system.session.compress',
          context_used_ratio: 0.22,
          context_used_tokens: 28_000,
          context_status: 'normal',
        },
      }),
    );

    expect(onSessionContextPatch).toHaveBeenCalledWith('sess-1', {
      context_used_ratio: 0.22,
      context_used_tokens: 28_000,
      context_status: 'normal',
    });
  });
});
