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
import type { MonitorEvent } from './monitorEvent';
import type { V2WsEnvelope } from '../features/chat/v2Types';

/**
 * WS downstream message shape. The single source of truth for what the
 * backend sends over `/v1/ws`. Carries one of:
 * - control messages (connected/pong/server_shutdown/monitor.backpressure)
 * - monitor_event for monitor channel
 * Chat / graph / team / knowledge business events arrive as separate
 * `v2_event` frames (not WsDownstream). There is no activity_event field.
 */
export type WsDownstream = {
  direction: 'server_to_client';
  channel: string;
  type?: string;
  payload?: unknown;
  monitor_event?: MonitorEvent;
};

/** Payload for MON-OPT-04 monitor.backpressure frames. */
export type MonitorBackpressurePayload = {
  dropped_high?: number;
  dropped_normal?: number;
  dropped_low?: number;
  window_seconds?: number;
  advice?: string;
};

/**
 * M74 V2-T4: client_tool.invoke downstream payload (design 74 §6.2).
 * Fanned out by the backend only to session connections that advertised
 * the desktop_companion capability via register_capabilities.
 */
export type ClientToolInvokePayload = {
  invocation_id: string;
  session_id: string;
  /** Full runtime tool name, e.g. client_open_app / client_open_url. */
  tool: string;
  /** Raw tool arguments ({target} / {url}); absent for arg-less tools. */
  args?: Record<string, unknown>;
};

/**
 * WS upstream message shape. Sent by the client over `/v1/ws` for
 * user_message/cancel/subscribe/unsubscribe/ping/enable_log/sync_request.
 */
export type WsUpstream = {
  direction: 'client_to_server';
  channel: string;
  type: string;
  request_id?: string;
  payload?: unknown;
};
import {
  WS_MAX_RECONNECT_DELAY_MS,
  WS_HEARTBEAT_INTERVAL_MS,
  WS_RECONNECT_BASE_DELAY_MS,
  WS_ZOMBIE_TIMEOUT_MS,
} from '../features/constants/timeouts';

// T1.8 + P1 #2: 队列分级。控制消息（ping/subscribe 等）可丢弃；业务消息
// （user_message/cancel 等）不丢弃，满时通过 onDrop 回调通知调用方走 HTTP 回退。
const WS_CONTROL_QUEUE_MAX_LENGTH = 50;
const WS_BUSINESS_QUEUE_MAX_LENGTH = 200;

/**
 * P1 #2: 按消息 type 分类优先级。
 * - control：系统控制消息（ping/subscribe/unsubscribe/enable_log/sync_request），
 *   断连堆积时可安全丢弃（重连后会重新订阅/同步）。
 * - business：用户业务消息（user_message/cancel/user_feedback 等），不可丢弃。
 */
function classifyMessagePriority(upstream: WsUpstream): 'control' | 'business' {
  switch (upstream.type) {
    case 'ping':
    case 'subscribe':
    case 'unsubscribe':
    case 'enable_log':
    case 'sync_request':
    case 'register_capabilities':
      return 'control';
    default:
      return 'business';
  }
}

export type WsTransportOptions = {
  sessionId: string;
  lastEventId?: string;
  token?: string;
  logEnabled?: boolean;
  /**
   * Optional explicit WS URL. When omitted, the URL is built from
   * sessionId/lastEventId/token/logEnabled via buildWsUrl.
   */
  url?: string;
  /**
   * Optional factory used to construct the underlying WebSocket. Primarily
   * intended for tests to inject a mock socket. When omitted, the standard
   * `new WebSocket(url)` constructor is used.
   */
  socketFactory?: (url: string) => WebSocket;
  /**
   * Monitor channel: called when a downstream message carries a
   * monitor_event payload (log, flow_log, mcp, alert).
   */
  onMonitorEvent?: (event: MonitorEvent) => void;
  /**
   * MON-OPT-04: server reports WS send-queue drops via type=monitor.backpressure.
   */
  onBackpressure?: (payload: MonitorBackpressurePayload) => void;
  /**
   * v2 chat events: dispatched when a downstream message carries a v2_event
   * envelope (type="v2_event"). The payload is the raw envelope object.
   */
  onV2Event?: (envelope: V2WsEnvelope) => void;
  /**
   * M74 V2-T4: client tool bridge invoke (system channel,
   * type="client_tool.invoke"). Only connections that registered the
   * desktop_companion capability receive these frames.
   */
  onClientToolInvoke?: (msg: ClientToolInvokePayload) => void;
  onConnected?: (info: { sessionId: string; lastEventId?: string }) => void;
  onDisconnected?: () => void;
  onError?: (error: Event) => void;
  onServerShutdown?: (reason: string) => void;
  /**
   * P1 #2: 业务消息因队列满而被拒绝入队时触发。调用方应回退到 HTTP。
   * 仅对 business 优先级消息触发；control 消息满时静默丢弃最旧的。
   */
  onDrop?: (upstream: WsUpstream) => void;
};

export type WsTransport = {
  connect(): void;
  disconnect(): void;
  /** P3.2: skip the backoff timer and reconnect immediately (network recovery). No-op while connected or after server shutdown. */
  reconnectNow(): void;
  send(upstream: WsUpstream): void;
  subscribe(channel: string): void;
  unsubscribe(channel: string): void;
  enableLog(enabled: boolean): void;
  /** M74 V2-T4: announce client capabilities (e.g. desktop_companion). */
  registerCapabilities(capabilities: string[]): void;
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
  // P3.2: timestamp of the last downstream frame — drives zombie detection.
  let lastActivityAt = 0;
  let onlineListener: (() => void) | null = null;
  // P1 #2: 分级队列。control 可丢弃，business 不丢弃。
  const controlQueue: WsUpstream[] = [];
  const businessQueue: WsUpstream[] = [];

  function connect(): void {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
      return;
    }
    ensureOnlineListener();

    const url =
      opts.url ??
      buildWsUrl({
        sessionId: opts.sessionId,
        lastEventId: _lastEventId,
        token: opts.token,
        logEnabled: opts.logEnabled,
      });
    ws = opts.socketFactory ? opts.socketFactory(url) : new WebSocket(url);

    ws.onopen = () => {
      _connected = true;
      reconnectAttempts = 0;
      lastActivityAt = Date.now();
      startHeartbeat();
      flushPendingQueue();
    };

    ws.onmessage = (ev: MessageEvent) => {
      lastActivityAt = Date.now();
      try {
        const raw = JSON.parse(ev.data) as Record<string, unknown>;

        // v2 chat events: envelope { type: "v2_event", kind, payload }.
        // The v2 envelope has NO `direction` field, so it must be dispatched
        // BEFORE the legacy `server_to_client` direction check below —
        // otherwise v2 events would be silently dropped.
        if (raw.type === 'v2_event') {
          const envelope = raw as unknown as V2WsEnvelope;
          opts.onV2Event?.(envelope);
          return;
        }

        const msg = raw as WsDownstream;
        if (msg.direction !== 'server_to_client') return;

        if (msg.type === 'connected' && msg.payload) {
          const payload = msg.payload as Record<string, unknown>;
          _lastEventId = (payload.last_event_id as string) || _lastEventId;
          opts.onConnected?.({ sessionId: opts.sessionId, lastEventId: _lastEventId });
          return;
        }

        if (msg.type === 'pong') return;

        if (msg.type === 'server_shutdown') {
          const payload = msg.payload as Record<string, unknown> | undefined;
          const reason = (payload?.reason as string) || 'server_shutting_down';
          shutdownReceived = true;
          opts.onServerShutdown?.(reason);
          return;
        }

        if (msg.type === 'monitor.backpressure') {
          opts.onBackpressure?.((msg.payload ?? {}) as MonitorBackpressurePayload);
          return;
        }

        // M74 V2-T4: client tool bridge invoke — payload must be present.
        if (msg.type === 'client_tool.invoke') {
          if (msg.payload) {
            opts.onClientToolInvoke?.(msg.payload as ClientToolInvokePayload);
          }
          return;
        }

        // Monitor channel: dispatch monitor events (log, flow_log, mcp,
        // alert). This replaces the legacy envelope-based dispatch for
        // monitor events on the "monitor" channel.
        if (msg.monitor_event) {
          opts.onMonitorEvent?.(msg.monitor_event);
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
        // P3.2: weak-network zombie detection. A half-open TCP socket (mobile
        // network drop, NAT timeout) never fires close/error, so the transport
        // would sit OPEN forever. Any downstream frame (pong included) resets
        // lastActivityAt; exceeding the window forces close → normal reconnect.
        if (lastActivityAt > 0 && Date.now() - lastActivityAt >= WS_ZOMBIE_TIMEOUT_MS) {
          console.warn('ws-transport: zombie connection detected (no downstream frames), force closing');
          ws.close(4000, 'zombie connection timeout');
          return;
        }
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

  // P3.2: every transport self-heals on network recovery — the browser
  // `online` event skips the exponential backoff (up to 30s) and reconnects
  // immediately. Registered lazily on first connect; removed on disconnect().
  function ensureOnlineListener(): void {
    if (onlineListener || typeof window === 'undefined' || !window.addEventListener) return;
    onlineListener = () => reconnectNow();
    window.addEventListener('online', onlineListener);
  }

  function reconnectNow(): void {
    if (shutdownReceived) return;
    if (ws && ws.readyState === WebSocket.OPEN) return;
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    reconnectAttempts = 0;
    if (ws) {
      const stale = ws;
      ws = null;
      stale.onclose = null; // reconnect handled here — avoid double scheduling
      try {
        stale.close();
      } catch {
        // already closed
      }
    }
    connect();
  }

  function disconnect(): void {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    stopHeartbeat();
    if (onlineListener && typeof window !== 'undefined') {
      window.removeEventListener('online', onlineListener);
      onlineListener = null;
    }
    lastActivityAt = 0;
    controlQueue.length = 0;
    businessQueue.length = 0;
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
    // P1 #2: business 优先刷新（用户消息优先于控制消息）。
    flushQueue(businessQueue);
    flushQueue(controlQueue);
  }

  function flushQueue(queue: WsUpstream[]): void {
    while (queue.length > 0 && ws && ws.readyState === WebSocket.OPEN) {
      const msg = queue.shift()!;
      try {
        ws.send(JSON.stringify(msg));
      } catch (err) {
        // Send failed — re-enqueue the message and stop flushing.
        // The next onopen or manual flush will retry.
        queue.unshift(msg);
        console.warn('ws-transport: flushQueue send failed, re-enqueued', err);
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

  // P1 #2: 分级入队策略。
  // - control 消息（ping/subscribe 等）：满时丢弃最旧的（重连后会重新订阅）。
  // - business 消息（user_message/cancel 等）：满时不丢弃，通过 onDrop 通知调用方
  //   走 HTTP 回退，避免用户消息静默丢失而前端仍显示"已发送"。
  function enqueuePending(upstream: WsUpstream): void {
    const priority = classifyMessagePriority(upstream);
    if (priority === 'control') {
      if (controlQueue.length >= WS_CONTROL_QUEUE_MAX_LENGTH) {
        controlQueue.shift();
        console.warn(
          `ws-transport: controlQueue full (${WS_CONTROL_QUEUE_MAX_LENGTH}), dropped oldest control message`,
        );
      }
      controlQueue.push(upstream);
      return;
    }
    // business 优先级
    if (businessQueue.length >= WS_BUSINESS_QUEUE_MAX_LENGTH) {
      // 不丢弃业务消息：通知调用方走 HTTP 回退
      console.warn(
        `ws-transport: businessQueue full (${WS_BUSINESS_QUEUE_MAX_LENGTH}), rejecting business message (caller should HTTP fallback)`,
      );
      opts.onDrop?.(upstream);
      return;
    }
    businessQueue.push(upstream);
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

  function registerCapabilities(capabilities: string[]): void {
    send({
      direction: 'client_to_server',
      channel: 'system',
      type: 'register_capabilities',
      payload: { capabilities },
    });
  }

  return {
    connect,
    disconnect,
    reconnectNow,
    send,
    subscribe,
    unsubscribe,
    enableLog,
    registerCapabilities,
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
