// P3.2: weak-network hardening — zombie connection detection, reconnectNow,
// and online-event immediate reconnect.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createWsTransport } from '../ws-transport';
import { WS_HEARTBEAT_INTERVAL_MS, WS_ZOMBIE_TIMEOUT_MS } from '../../features/constants/timeouts';

class MockWS {
  onopen: (() => void) | null = null;
  onmessage: ((ev: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: ((e: unknown) => void) | null = null;
  readyState = 1; // OPEN
  sent: string[] = [];
  closed = false;
  send(data: string) {
    this.sent.push(data);
  }
  close() {
    this.closed = true;
    this.readyState = 3;
    this.onclose?.();
  }
}

function setup() {
  const sockets: MockWS[] = [];
  const factory = () => {
    const s = new MockWS();
    sockets.push(s);
    return s as unknown as WebSocket;
  };
  const transport = createWsTransport({
    sessionId: 's1',
    url: 'ws://localhost',
    socketFactory: factory,
  });
  return { sockets, transport };
}

function openConnection(sockets: MockWS[]) {
  const ws = sockets[sockets.length - 1];
  ws.readyState = 1;
  ws.onopen?.();
  return ws;
}

/** Mirrors a real socket drop: readyState CLOSED before onclose fires. */
function simulateDrop(ws: MockWS) {
  ws.readyState = 3;
  ws.onclose?.();
}

describe('ws-transport zombie detection (P3.2)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('force-closes the socket when no downstream frame arrives within the zombie window', () => {
    const { sockets, transport } = setup();
    transport.connect();
    const ws = openConnection(sockets);

    // Heartbeat ticks fire pings while the connection is healthy.
    vi.advanceTimersByTime(WS_HEARTBEAT_INTERVAL_MS);
    expect(ws.sent.some((m) => m.includes('"ping"'))).toBe(true);
    expect(ws.closed).toBe(false);

    // No pong / no frames at all — past the zombie window the transport must
    // close the half-open socket so onclose schedules a reconnect.
    vi.advanceTimersByTime(WS_ZOMBIE_TIMEOUT_MS + WS_HEARTBEAT_INTERVAL_MS);
    expect(ws.closed).toBe(true);
    transport.disconnect();
  });

  it('any downstream frame resets the zombie clock', () => {
    const { sockets, transport } = setup();
    transport.connect();
    const ws = openConnection(sockets);

    // Almost reach the zombie window, then a pong arrives.
    vi.advanceTimersByTime(WS_ZOMBIE_TIMEOUT_MS - 1);
    ws.onmessage?.({ data: JSON.stringify({ direction: 'server_to_client', channel: 'system', type: 'pong' }) });
    vi.advanceTimersByTime(WS_HEARTBEAT_INTERVAL_MS * 2);
    expect(ws.closed).toBe(false);
    transport.disconnect();
  });

  it('does not close a healthy connection that keeps receiving frames', () => {
    const { sockets, transport } = setup();
    transport.connect();
    const ws = openConnection(sockets);

    for (let i = 0; i < 5; i++) {
      vi.advanceTimersByTime(WS_HEARTBEAT_INTERVAL_MS);
      ws.onmessage?.({ data: JSON.stringify({ direction: 'server_to_client', channel: 'system', type: 'pong' }) });
    }
    expect(ws.closed).toBe(false);
    transport.disconnect();
  });
});

describe('ws-transport reconnectNow + online event (P3.2)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('reconnectNow skips the backoff timer and reconnects immediately', () => {
    const { sockets, transport } = setup();
    transport.connect();
    const ws = openConnection(sockets);

    // Drop the connection → backoff reconnect scheduled (1s base delay).
    simulateDrop(ws);
    expect(sockets.length).toBe(1);

    // Network recovery triggers an immediate reconnect instead of waiting.
    transport.reconnectNow();
    expect(sockets.length).toBe(2);
    transport.disconnect();
  });

  it('reconnectNow is a no-op while connected', () => {
    const { sockets, transport } = setup();
    transport.connect();
    openConnection(sockets);
    transport.reconnectNow();
    expect(sockets.length).toBe(1);
    transport.disconnect();
  });

  it('reconnects immediately when the browser fires the online event', () => {
    const { sockets, transport } = setup();
    transport.connect();
    const ws = openConnection(sockets);
    simulateDrop(ws); // backoff timer pending
    expect(sockets.length).toBe(1);

    window.dispatchEvent(new Event('online'));
    expect(sockets.length).toBe(2);
    transport.disconnect();
  });

  it('stops reacting to online after disconnect()', () => {
    const { sockets, transport } = setup();
    transport.connect();
    const ws = openConnection(sockets);
    simulateDrop(ws);
    transport.disconnect();

    window.dispatchEvent(new Event('online'));
    expect(sockets.length).toBe(1);
  });
});
