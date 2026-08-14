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

  it('does not synthesize activity_event from v2_event frames', () => {
    const mock = new MockWS();
    const onV2Event = vi.fn();
    const onMonitorEvent = vi.fn();
    const transport = createWsTransport({
      sessionId: 's1',
      url: 'ws://localhost',
      socketFactory: () => mock as unknown as WebSocket,
      onV2Event,
      onMonitorEvent,
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
          filter_key: 'graph/g1/e1',
          node_id: 'n1',
        },
      },
    });
    mock.onmessage!({ data: v2Msg });

    expect(onV2Event).toHaveBeenCalledTimes(1);
    expect(onV2Event.mock.calls[0][0].kind).toBe('system.notice');
    expect(onMonitorEvent).not.toHaveBeenCalled();
    transport.disconnect();
  });
});
