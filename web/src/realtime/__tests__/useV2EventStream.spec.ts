import { describe, it, expect, vi } from 'vitest';
import { createV2EventStream } from '../useV2EventStream';
import { createMonitorStream } from '../useMonitorStream';
import { GLOBAL_WS_SESSION_ID } from '../../config/runtime';

class MockWS {
  onopen: (() => void) | null = null;
  onmessage: ((ev: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: ((e: unknown) => void) | null = null;
  readyState = 1;
  sent: string[] = [];
  send(data: string) {
    this.sent.push(data);
  }
  close() {}
}

describe('createV2EventStream', () => {
  it('dispatches v2_event to onV2Event', () => {
    const mock = new MockWS();
    const OriginalWS = globalThis.WebSocket;
    globalThis.WebSocket = class {
      constructor() {
        return mock as unknown as WebSocket;
      }
    } as unknown as typeof WebSocket;

    const onV2Event = vi.fn();
    const stream = createV2EventStream({
      sessionId: 'sess-graph-1',
      channels: ['graph', 'system'],
      autoConnect: false,
      onV2Event,
    });
    stream.connect();
    mock.onopen?.();

    mock.onmessage?.({
      data: JSON.stringify({
        type: 'v2_event',
        kind: 'system.notice',
        session_id: 'sess-graph-1',
        payload: { NoticeType: 'node_start', Meta: { node_id: 'n1' } },
      }),
    });

    expect(onV2Event).toHaveBeenCalledTimes(1);
    expect(onV2Event.mock.calls[0][0].kind).toBe('system.notice');
    stream.disconnect();
    globalThis.WebSocket = OriginalWS;
  });
});

describe('createMonitorStream', () => {
  it('dispatches monitor_event and ignores v2_event', () => {
    const mock = new MockWS();
    const OriginalWS = globalThis.WebSocket;
    globalThis.WebSocket = class {
      constructor() {
        return mock as unknown as WebSocket;
      }
    } as unknown as typeof WebSocket;

    const onMonitorEvent = vi.fn();
    const stream = createMonitorStream({
      sessionId: 'sess-mon-1',
      channels: ['monitor', 'system'],
      autoConnect: false,
      onMonitorEvent,
    });
    stream.connect();
    mock.onopen?.();

    mock.onmessage?.({
      data: JSON.stringify({
        direction: 'server_to_client',
        channel: 'monitor',
        monitor_event: { id: 'm1', type: 'flow_log', timestamp: 't', message: 'ok' },
      }),
    });
    mock.onmessage?.({
      data: JSON.stringify({ type: 'v2_event', kind: 'task.created', payload: {} }),
    });

    expect(onMonitorEvent).toHaveBeenCalledTimes(1);
    expect(onMonitorEvent.mock.calls[0][0].type).toBe('flow_log');
    stream.disconnect();
    globalThis.WebSocket = OriginalWS;
  });
});

describe('createV2EventStream global hub', () => {
  it('uses session_id=* without a dedicated per-stream transport', () => {
    const mock = new MockWS();
    const OriginalWS = globalThis.WebSocket;
    globalThis.WebSocket = class {
      constructor() {
        return mock as unknown as WebSocket;
      }
    } as unknown as typeof WebSocket;

    const onV2Event = vi.fn();
    const stream = createV2EventStream({
      sessionId: GLOBAL_WS_SESSION_ID,
      channels: ['team', 'system'],
      autoConnect: false,
      onV2Event,
    });
    stream.connect();
    expect(stream.transport.value).toBeNull();
    stream.disconnect();
    globalThis.WebSocket = OriginalWS;
  });
});
