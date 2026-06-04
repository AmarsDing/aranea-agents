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
// #region debug-point (web-chat-no-response) reporter import
import { reportDebug } from '../../../.dbg/debug-reporter';
// #endregion debug-point

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
  const maxReconnectDelay = 30_000;
  const maxReconnectAttempts = 10;
  const heartbeatInterval = 25_000;
  const pendingQueue: WsUpstream[] = [];

  function connect(): void {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
      // #region debug-point (web-chat-no-response) ws-connect-skip
      reportDebug({ hypothesisId: 'H2', source: 'ws-transport.connect', data: { readyState: ws.readyState, url: buildWsUrl({ sessionId: opts.sessionId }) } });
      // #endregion debug-point
      return;
    }

    const url = buildWsUrl({
      sessionId: opts.sessionId,
      lastEventId: _lastEventId,
      token: opts.token,
      logEnabled: opts.logEnabled,
    });
    // #region debug-point (web-chat-no-response) ws-connect-new
    reportDebug({ hypothesisId: 'H2', source: 'ws-transport.connect', data: { url, lastEventId: _lastEventId ?? null, logEnabled: !!opts.logEnabled } });
    // #endregion debug-point
    ws = new WebSocket(url);

    ws.onopen = () => {
      _connected = true;
      reconnectAttempts = 0;
      startHeartbeat();
      // #region debug-point (web-chat-no-response) ws-onopen
      reportDebug({ hypothesisId: 'H2', source: 'ws-transport.connect', data: { event: 'open', queueDepth: pendingQueue.length } });
      // #endregion debug-point
      flushPendingQueue();
    };

    ws.onmessage = (ev: MessageEvent) => {
      // #region debug-point (web-chat-no-response) ws-onmessage
      try {
        const parsed = JSON.parse(ev.data);
        reportDebug({ hypothesisId: 'H3', source: 'ws-transport.onmessage', data: {
          type: parsed.type,
          channel: parsed.channel,
          envelopeType: parsed.envelope?.type,
          envelopeChannel: parsed.envelope?.channel,
        } });
      } catch {
        reportDebug({ hypothesisId: 'H3', source: 'ws-transport.onmessage', data: { rawLen: ev.data?.length ?? 0 } });
      }
      // #endregion debug-point
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
          opts.onEnvelope?.(msg.envelope);
        }
      } catch {
        // ignore parse errors
      }
    };

    ws.onclose = (ev) => {
      _connected = false;
      stopHeartbeat();
      // #region debug-point (web-chat-no-response) ws-onclose
      reportDebug({ hypothesisId: 'H2', source: 'ws-transport.connect', data: { event: 'close', code: ev.code, reason: ev.reason, wasClean: ev.wasClean } });
      // #endregion debug-point
      opts.onDisconnected?.();
      if (!shutdownReceived) {
        scheduleReconnect();
      }
    };

    ws.onerror = (e) => {
      // #region debug-point (web-chat-no-response) ws-onerror
      reportDebug({ hypothesisId: 'H2', source: 'ws-transport.connect', level: 'error', data: { event: 'error' } });
      // #endregion debug-point
      opts.onError?.(e);
    };
  }

  function scheduleReconnect(): void {
    if (reconnectTimer) return;
    if (reconnectAttempts >= maxReconnectAttempts) {
      opts.onReconnectFailed?.();
      return;
    }
    const delay = Math.min(1000 * Math.pow(2, reconnectAttempts), maxReconnectDelay);
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
    }, heartbeatInterval);
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
    while (pendingQueue.length > 0 && ws && ws.readyState === WebSocket.OPEN) {
      const msg = pendingQueue.shift()!;
      ws.send(JSON.stringify(msg));
    }
  }

  function send(upstream: WsUpstream): void {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(upstream));
      // #region debug-point (web-chat-no-response) ws-send-immediate
      reportDebug({ hypothesisId: 'H2', source: 'ws-transport.send', data: { type: upstream.type, readyState: ws.readyState, channel: upstream.channel } });
      // #endregion debug-point
      return;
    }
    pendingQueue.push(upstream);
    // #region debug-point (web-chat-no-response) ws-send-queued
    reportDebug({ hypothesisId: 'H2', source: 'ws-transport.send', level: 'warn', data: {
      type: upstream.type,
      readyState: ws?.readyState ?? null,
      queueDepth: pendingQueue.length,
      hasWs: !!ws,
    } });
    // #endregion debug-point
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
    get connected() {
      return _connected;
    },
    get lastEventId() {
      return _lastEventId;
    },
  };
}
