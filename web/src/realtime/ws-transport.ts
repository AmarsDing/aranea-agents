/**
 * Shared WebSocket transport — the single source of truth for the
 * WS transport infrastructure. Both chat and non-chat features
 * import from here.
 *
 * Previously this module lived in features/chat/ws-transport.ts; it has
 * been lifted to this shared location so that features don't need to
 * reach into the chat domain for transport-level infrastructure.
 */

import { buildWsUrl } from '../config/runtime';
import type { Envelope, WsDownstream, WsUpstream } from './envelope';
import {
  WS_MAX_RECONNECT_DELAY_MS,
  WS_MAX_RECONNECT_ATTEMPTS,
  WS_HEARTBEAT_INTERVAL_MS,
  WS_RECONNECT_BASE_DELAY_MS,
  WS_RUN_STALE_TIMEOUT_MS,
} from '../features/constants/timeouts';

export type WsTransportOptions = {
  sessionId: string;
  lastEventId?: string;
  token?: string;
  logEnabled?: boolean;
  onEnvelope?: (env: Envelope) => void;
  onConnected?: (info: { sessionId: string; lastEventId?: string }) => void;
  onDisconnected?: () => void;
  onError?: (error: Event) => void;
  onServerShutdown?: (reason: string) => void;
  /** Fired when EventBuffer replay starts/ends (reconnect with last_event_id). */
  onReplayState?: (replaying: boolean, count?: number) => void;
  /** Fired when all reconnect attempts have been exhausted. */
  onReconnectFailed?: () => void;
  /** Fired when a run_heartbeat envelope arrives (P1-7). Use to update UI progress. */
  onHeartbeat?: (env: Envelope) => void;
  /** Fired when no run_heartbeat has arrived within WS_RUN_STALE_TIMEOUT_MS (P1-7). */
  onStale?: () => void;
};

export type WsTransport = {
  connect(): void;
  disconnect(): void;
  send(upstream: WsUpstream): void;
  subscribe(channel: string): void;
  unsubscribe(channel: string): void;
  enableLog(enabled: boolean): void;
  ping(): void;
  cancel(): void;
  /** Reset the run-stale timer (P1-7). Call when a run starts to begin stale detection. */
  resetStaleTimer(): void;
  readonly connected: boolean;
  readonly lastEventId: string | undefined;
};

export function createWsTransport(opts: WsTransportOptions): WsTransport {
  let ws: WebSocket | null = null;
  let _connected = false;
  let _lastEventId: string | undefined = opts.lastEventId;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  let staleTimer: ReturnType<typeof setTimeout> | null = null;
  let reconnectAttempts = 0;
  let shutdownReceived = false;
  const pendingQueue: WsUpstream[] = [];

  function connect(): void {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
      return;
    }

    const url = buildWsUrl({
      sessionId: opts.sessionId,
      lastEventId: _lastEventId,
      token: opts.token,
      logEnabled: opts.logEnabled,
    });
    ws = new WebSocket(url);

    ws.onopen = () => {
      _connected = true;
      reconnectAttempts = 0;
      startHeartbeat();
      flushPendingQueue();
    };

    ws.onmessage = (ev: MessageEvent) => {
      try {
        const msg = JSON.parse(ev.data) as WsDownstream;
        if (msg.direction !== 'server_to_client') return;

        if (msg.type === 'connected' && msg.payload) {
          const payload = msg.payload as Record<string, unknown>;
          _lastEventId = (payload.last_event_id as string) || _lastEventId;
          opts.onConnected?.({ sessionId: opts.sessionId, lastEventId: _lastEventId });
          return;
        }

        if (msg.type === 'pong') return;

        if (msg.type === 'replay_start') {
          const payload = msg.payload as Record<string, unknown> | undefined;
          const count = typeof payload?.count === 'number' ? payload.count : undefined;
          opts.onReplayState?.(true, count);
          return;
        }

        if (msg.type === 'replay_end') {
          opts.onReplayState?.(false);
          return;
        }

        if (msg.type === 'server_shutdown') {
          const payload = msg.payload as Record<string, unknown> | undefined;
          const reason = (payload?.reason as string) || 'server_shutting_down';
          shutdownReceived = true;
          opts.onServerShutdown?.(reason);
          return;
        }

        if (msg.envelope) {
          _lastEventId = msg.envelope.id;
          if (msg.envelope.type === 'run_heartbeat') {
            resetStaleTimer();
            opts.onHeartbeat?.(msg.envelope);
          }
          opts.onEnvelope?.(msg.envelope);
        }
      } catch {
        // ignore parse errors
      }
    };

    ws.onclose = (ev) => {
      _connected = false;
      stopHeartbeat();
      stopStaleTimer();
      opts.onDisconnected?.();
      if (!shutdownReceived) {
        scheduleReconnect();
      }
    };

    ws.onerror = (e) => {
      opts.onError?.(e);
    };
  }

  function scheduleReconnect(): void {
    if (reconnectTimer) return;
    if (reconnectAttempts >= WS_MAX_RECONNECT_ATTEMPTS) {
      opts.onReconnectFailed?.();
      return;
    }
    const delay = Math.min(WS_RECONNECT_BASE_DELAY_MS * Math.pow(2, reconnectAttempts), WS_MAX_RECONNECT_DELAY_MS);
    reconnectAttempts++;
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null;
      connect();
    }, delay);
  }

  function startHeartbeat(): void {
    stopHeartbeat();
    heartbeatTimer = setInterval(() => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ping();
      }
    }, WS_HEARTBEAT_INTERVAL_MS);
  }

  function stopHeartbeat(): void {
    if (heartbeatTimer) {
      clearInterval(heartbeatTimer);
      heartbeatTimer = null;
    }
  }

  function resetStaleTimer(): void {
    if (staleTimer) {
      clearTimeout(staleTimer);
    }
    staleTimer = setTimeout(() => {
      staleTimer = null;
      opts.onStale?.();
    }, WS_RUN_STALE_TIMEOUT_MS);
  }

  function stopStaleTimer(): void {
    if (staleTimer) {
      clearTimeout(staleTimer);
      staleTimer = null;
    }
  }

  function disconnect(): void {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    stopHeartbeat();
    stopStaleTimer();
    pendingQueue.length = 0;
    if (ws) {
      ws.onclose = null;
      ws.close(1000, 'client disconnect');
      ws = null;
    }
    _connected = false;
  }

  function flushPendingQueue(): void {
    while (pendingQueue.length > 0 && ws && ws.readyState === WebSocket.OPEN) {
      const msg = pendingQueue.shift()!;
      ws.send(JSON.stringify(msg));
    }
  }

  function send(upstream: WsUpstream): void {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(upstream));
      return;
    }
    pendingQueue.push(upstream);
    if (!ws || ws.readyState === WebSocket.CLOSED) {
      connect();
    }
  }

  function subscribe(channel: string): void {
    send({
      direction: 'client_to_server',
      channel: 'system',
      type: 'subscribe',
      payload: { channel },
    });
  }

  function unsubscribe(channel: string): void {
    send({
      direction: 'client_to_server',
      channel: 'system',
      type: 'unsubscribe',
      payload: { channel },
    });
  }

  function ping(): void {
    send({
      direction: 'client_to_server',
      channel: 'system',
      type: 'ping',
    });
  }

  function cancel(): void {
    send({
      direction: 'client_to_server',
      channel: 'chat',
      type: 'cancel',
    });
  }

  function enableLog(enabled: boolean): void {
    send({
      direction: 'client_to_server',
      channel: 'system',
      type: 'enable_log',
      payload: { enabled },
    });
  }

  return {
    connect,
    disconnect,
    send,
    subscribe,
    unsubscribe,
    enableLog,
    ping,
    cancel,
    resetStaleTimer,
    get connected() {
      return _connected;
    },
    get lastEventId() {
      return _lastEventId;
    },
  };
}
