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
import { RevisionTracker, requestSyncReplay } from './event_replay';
import {
  WS_MAX_RECONNECT_DELAY_MS,
  WS_HEARTBEAT_INTERVAL_MS,
  WS_RECONNECT_BASE_DELAY_MS,
} from '../features/constants/timeouts';

// T1.8: pendingQueue max length. Prevents unbounded memory growth when the
// WebSocket is disconnected for a long time. When the queue is full, new
// messages are rejected (the caller should fall back to HTTP).
const WS_PENDING_QUEUE_MAX_LENGTH = 100;

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
  readonly connected: boolean;
  readonly lastEventId: string | undefined;
};

export function createWsTransport(opts: WsTransportOptions): WsTransport {
  let ws: WebSocket | null = null;
  let _connected = false;
  let _lastEventId: string | undefined = opts.lastEventId;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  let reconnectAttempts = 0;
  let shutdownReceived = false;
  // T3.4: Track whether we've ever connected before, to detect reconnects.
  // reconnectAttempts is reset to 0 in onopen, so we can't rely on it in the
  // connected message handler (which arrives after onopen).
  let hasConnectedBefore = false;
  const pendingQueue: WsUpstream[] = [];
  // T3.4: Per-session revision tracker for sync_request replay after reconnect.
  // Updated on every envelope carrying session_revision; used on reconnect to
  // request replay of envelopes with revision > last known.
  const revisionTracker = new RevisionTracker();

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
          // T3.4: After reconnect, request revision-based sync replay.
          // The server replays envelopes with session_revision > last known.
          // This complements event-ID-based replay (via lastEventId in URL)
          // by ensuring message-level consistency for envelopes persisted
          // during the disconnection window.
          if (hasConnectedBefore) {
            const lastRevision = revisionTracker.get(opts.sessionId);
            if (lastRevision > 0) {
              requestSyncReplay(send, opts.sessionId, lastRevision);
            }
          }
          hasConnectedBefore = true;
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
          // T3.4: Track session_revision for sync_request replay.
          if (msg.envelope.session_revision && msg.envelope.session_revision > 0) {
            revisionTracker.update(opts.sessionId, msg.envelope.session_revision);
          }
          opts.onEnvelope?.(msg.envelope);
        }
      } catch {
        // ignore parse errors
      }
    };

    ws.onclose = () => {
      _connected = false;
      stopHeartbeat();
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
    // T1.8: Unlimited reconnect for same-machine deployment. The exponential
    // backoff already caps the delay at WS_MAX_RECONNECT_DELAY_MS (30s), so
    // unlimited reconnect won't spam the server. The previous
    // WS_MAX_RECONNECT_ATTEMPTS=10 limit caused permanent disconnection
    // after ~5 minutes of network issues, which is unacceptable for the
    // No-Timeout principle.
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

  function disconnect(): void {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    stopHeartbeat();
    pendingQueue.length = 0;
    if (ws) {
      ws.onclose = null;
      ws.close(1000, 'client disconnect');
      ws = null;
    }
    _connected = false;
  }

  function flushPendingQueue(): void {
    // T1.8: try-catch around each send to prevent a single JSON.stringify
    // or send error from breaking the entire flush loop. Messages that
    // fail to send are left in the queue for the next flush attempt.
    while (pendingQueue.length > 0 && ws && ws.readyState === WebSocket.OPEN) {
      const msg = pendingQueue.shift()!;
      try {
        ws.send(JSON.stringify(msg));
      } catch (err) {
        // Send failed — re-enqueue the message and stop flushing.
        // The next onopen or manual flush will retry.
        pendingQueue.unshift(msg);
        console.warn('ws-transport: flushPendingQueue send failed, re-enqueued', err);
        break;
      }
    }
  }

  function send(upstream: WsUpstream): void {
    if (ws && ws.readyState === WebSocket.OPEN) {
      try {
        ws.send(JSON.stringify(upstream));
      } catch (err) {
        // T1.8: send failed — enqueue for retry instead of silently dropping.
        console.warn('ws-transport: send failed, enqueued for retry', err);
        enqueuePending(upstream);
      }
      return;
    }
    enqueuePending(upstream);
    if (!ws || ws.readyState === WebSocket.CLOSED) {
      connect();
    }
  }

  // T1.8: enqueuePending enforces the max queue length. When the queue is
  // full, the oldest message is dropped (FIFO eviction). This prevents
  // unbounded memory growth during long disconnections.
  function enqueuePending(upstream: WsUpstream): void {
    if (pendingQueue.length >= WS_PENDING_QUEUE_MAX_LENGTH) {
      // Drop the oldest message to make room for the new one.
      // This is acceptable because the oldest messages are likely stale
      // (e.g., ping/subscribe) and the caller should fall back to HTTP
      // for important messages.
      pendingQueue.shift();
      console.warn(`ws-transport: pendingQueue full (${WS_PENDING_QUEUE_MAX_LENGTH}), dropped oldest message`);
    }
    pendingQueue.push(upstream);
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
    get connected() {
      return _connected;
    },
    get lastEventId() {
      return _lastEventId;
    },
  };
}
