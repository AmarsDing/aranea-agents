// E2E-C2: the transport's internal auto-reconnect must resume from the
// FRESHEST replayable cursor, not the establishment-time snapshot. Live
// frames advance the cursor only in the channelWsCursor table — the
// getLastEventId getter is the reflux path consulted on every (re)connect.
import { describe, it, expect } from 'vitest';
import { createWsTransport } from '../ws-transport';

class MockWS {
  onopen: (() => void) | null = null;
  onmessage: ((ev: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: ((e: unknown) => void) | null = null;
  readyState = 1;
  send() {}
  close() {}
}

describe('ws-transport reconnect cursor (E2E-C2)', () => {
  it('consulting getLastEventId on every (re)connect carries the freshest cursor', () => {
    const urls: string[] = [];
    let liveCursor = 'v2:s1:9:task.completed:t9';
    const sockets: MockWS[] = [];
    const transport = createWsTransport({
      sessionId: 's1',
      lastEventId: 'v2:s1:5:task.completed:t5',
      getLastEventId: () => liveCursor,
      socketFactory: (url) => {
        urls.push(url);
        const s = new MockWS();
        sockets.push(s);
        return s as unknown as WebSocket;
      },
      onDisconnected: () => undefined,
    });

    transport.connect();
    expect(urls).toHaveLength(1);
    expect(decodeURIComponent(urls[0])).toContain('last_event_id=v2:s1:9:task.completed:t9');

    // Live frames advanced the cursor in the channelWsCursor table…
    liveCursor = 'v2:s1:42:task.completed:t42';
    // …then the socket drops. Force the stale socket CLOSED so reconnectNow proceeds.
    sockets[0].readyState = 3;
    transport.reconnectNow();

    expect(urls).toHaveLength(2);
    expect(decodeURIComponent(urls[1])).toContain('last_event_id=v2:s1:42:task.completed:t42');
    transport.disconnect();
  });

  it('falls back to the internal snapshot when no getter is provided', () => {
    const urls: string[] = [];
    const transport = createWsTransport({
      sessionId: 's1',
      lastEventId: 'v2:s1:7:task.completed:t7',
      socketFactory: (url) => {
        urls.push(url);
        return new MockWS() as unknown as WebSocket;
      },
    });
    transport.connect();
    expect(decodeURIComponent(urls[0])).toContain('last_event_id=v2:s1:7:task.completed:t7');
    transport.disconnect();
  });
});
