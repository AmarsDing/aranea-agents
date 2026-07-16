// web/src/realtime/__tests__/ws-transport-v2.spec.ts
import { describe, it, expect, vi } from 'vitest';
import { createWsTransport } from '../ws-transport';

// Minimal WebSocket mock
class MockWS {
  onopen: (() => void) | null = null;
  onmessage: ((ev: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: ((e: unknown) => void) | null = null;
  readyState = 1;
  send() {}
  close() {}
}

describe('ws-transport v2_event', () => {
  it('dispatches v2_event to onV2Event callback', () => {
    const mock = new MockWS();
    const onV2Event = vi.fn();
    const transport = createWsTransport({
      sessionId: 's1',
      url: 'ws://localhost',
      socketFactory: () => mock as unknown as WebSocket,
      onV2Event,
    });
    transport.connect();

    const v2Msg = JSON.stringify({
      type: 'v2_event',
      kind: 'task.created',
      payload: { Task: { ID: 't1' } },
    });
    mock.onmessage!({ data: v2Msg });

    expect(onV2Event).toHaveBeenCalledTimes(1);
    const arg = onV2Event.mock.calls[0][0];
    expect(arg.type).toBe('v2_event');
    expect(arg.kind).toBe('task.created');
    transport.disconnect();
  });

  it('does NOT dispatch v2_event to onActivityEvent', () => {
    const mock = new MockWS();
    const onActivityEvent = vi.fn();
    const onV2Event = vi.fn();
    const transport = createWsTransport({
      sessionId: 's1',
      url: 'ws://localhost',
      socketFactory: () => mock as unknown as WebSocket,
      onActivityEvent,
      onV2Event,
    });
    transport.connect();

    const v2Msg = JSON.stringify({ type: 'v2_event', kind: 'task.created', payload: {} });
    mock.onmessage!({ data: v2Msg });

    expect(onActivityEvent).not.toHaveBeenCalled();
    expect(onV2Event).toHaveBeenCalledTimes(1);
    transport.disconnect();
  });

  it('adapts system.notice to onActivityEvent for non-chat consumers', () => {
    const mock = new MockWS();
    const onActivityEvent = vi.fn();
    const onV2Event = vi.fn();
    const transport = createWsTransport({
      sessionId: 's1',
      url: 'ws://localhost',
      socketFactory: () => mock as unknown as WebSocket,
      onActivityEvent,
      onV2Event,
    });
    transport.connect();

    const v2Msg = JSON.stringify({
      type: 'v2_event',
      kind: 'system.notice',
      session_id: 's1',
      payload: {
        NoticeType: 'node_start',
        Message: '',
        Meta: {
          activity_kind: 'graph_stage',
          activity_status: 'running',
          activity_event: 'created',
          filter_key: 'graph/g1/e1',
          node_id: 'n1',
        },
      },
    });
    mock.onmessage!({ data: v2Msg });

    expect(onV2Event).toHaveBeenCalledTimes(1);
    expect(onActivityEvent).toHaveBeenCalledTimes(1);
    const adapted = onActivityEvent.mock.calls[0][0];
    expect(adapted.activity.kind).toBe('graph_stage');
    expect(adapted.activity.stage).toBe('node_start');
    expect(adapted.activity.session_id).toBe('s1');
    expect(adapted.activity.meta.filter_key).toBe('graph/g1/e1');
    transport.disconnect();
  });
});
