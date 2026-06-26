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
import type { ActivityEvent } from './activityEvent';
import type { MonitorEvent } from './monitorEvent';
import { RevisionTracker, requestSyncReplay } from './event_replay';
import {
  WS_MAX_RECONNECT_DELAY_MS,
  WS_HEARTBEAT_INTERVAL_MS,
  WS_RECONNECT_BASE_DELAY_MS,
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
  onEnvelope?: (env: Envelope) => void;
  /**
   * Activity-First (AF): called when a downstream message carries an
   * activity_event payload (business-semantic Activity lifecycle event).
   * This replaces the legacy activity_start/delta/done/child_start envelopes
   * for chat events.
   */
  onActivityEvent?: (ev: ActivityEvent) => void;
  /**
   * Monitor channel: called when a downstream message carries a
   * monitor_event payload (log, flow_log, mcp, alert). This replaces
   * the legacy envelope-based dispatch for monitor events.
   */
  onMonitorEvent?: (event: MonitorEvent) => void;
  onConnected?: (info: { sessionId: string; lastEventId?: string }) => void;
  onDisconnected?: () => void;
  onError?: (error: Event) => void;
  onServerShutdown?: (reason: string) => void;
  /** Fired when EventBuffer replay starts/ends (reconnect with last_event_id). */
  onReplayState?: (replaying: boolean, count?: number) => void;
  /**
   * P1 #2: 业务消息因队列满而被拒绝入队时触发。调用方应回退到 HTTP。
   * 仅对 business 优先级消息触发；control 消息满时静默丢弃最旧的。
   */
  onDrop?: (upstream: WsUpstream) => void;
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
  // P1 #2: 分级队列。control 可丢弃，business 不丢弃。
  const controlQueue: WsUpstream[] = [];
  const businessQueue: WsUpstream[] = [];
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

        // Activity-First (AF): dispatch business-semantic ActivityEvent.
        // This replaces the legacy activity_start/delta/done/child_start
        // envelopes for chat events. The activity_event carries a full
        // Activity snapshot, eliminating the need for metadata packing.
        if (msg.activity_event) {
          opts.onActivityEvent?.(msg.activity_event);
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
