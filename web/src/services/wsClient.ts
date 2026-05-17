/**
 * Unified WebSocket client for the `/v1/ws` endpoint.
 *
 * Features:
 * - Automatic reconnection with exponential back-off (base 1s, max 30s)
 * - Channel-scoped message subscriptions via `subscribe(channel, handler)`
 * - Graceful close via `disconnect()`
 * - Reconnect count exposed for monitoring via useHeartbeatStore
 */

import { buildWsUrl, readAccessTokenCookie } from "../config/runtime";

type MessageHandler = (data: unknown) => void;

type WsMessage = {
  channel?: string;
  type?: string;
  [key: string]: unknown;
};

const BASE_DELAY_MS = 1000;
const MAX_DELAY_MS = 30000;

class WsClient {
  private ws: WebSocket | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempt = 0;
  private intentionalClose = false;
  private subscriptions = new Map<string, Set<MessageHandler>>();
  private wildcardHandlers = new Set<MessageHandler>();

  /** Connect (or reconnect) to the backend WebSocket. */
  connect(sessionId: string, teamId?: string) {
    this.intentionalClose = false;
    this._open(sessionId, teamId);
  }

  /** Gracefully close the WebSocket; no reconnect attempt is made. */
  disconnect() {
    this.intentionalClose = true;
    this._clearReconnectTimer();
    if (this.ws) {
      this.ws.close(1000, "client_disconnect");
      this.ws = null;
    }
  }

  /**
   * Subscribe to messages for a specific channel.
   * Returns an unsubscribe function.
   */
  subscribe(channel: string, handler: MessageHandler): () => void {
    if (!this.subscriptions.has(channel)) {
      this.subscriptions.set(channel, new Set());
    }
    this.subscriptions.get(channel)!.add(handler);
    return () => {
      this.subscriptions.get(channel)?.delete(handler);
    };
  }

  /**
   * Subscribe to all messages regardless of channel.
   * Returns an unsubscribe function.
   */
  subscribeAll(handler: MessageHandler): () => void {
    this.wildcardHandlers.add(handler);
    return () => {
      this.wildcardHandlers.delete(handler);
    };
  }

  get isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  get reconnectCount(): number {
    return this.reconnectAttempt;
  }

  private _sessionId = "";
  private _teamId: string | undefined = undefined;

  private _open(sessionId?: string, teamId?: string) {
    if (sessionId) this._sessionId = sessionId;
    if (teamId !== undefined) this._teamId = teamId;
    if (!this._sessionId) return;
    if (this.ws && this.ws.readyState <= WebSocket.OPEN) return;

    const token = readAccessTokenCookie() ?? "";
    const url = buildWsUrl({ sessionId: this._sessionId, token });

    this.ws = new WebSocket(url);

    this.ws.onopen = () => {
      this.reconnectAttempt = 0;
    };

    this.ws.onmessage = (event: MessageEvent) => {
      let parsed: WsMessage | null = null;
      try {
        parsed = JSON.parse(event.data as string) as WsMessage;
      } catch {
        return;
      }

      // Deliver to wildcard handlers.
      for (const h of this.wildcardHandlers) {
        try { h(parsed); } catch { /* ignore handler errors */ }
      }

      // Deliver to channel-scoped handlers.
      const channel = parsed?.channel;
      if (channel) {
        const handlers = this.subscriptions.get(channel);
        if (handlers) {
          for (const h of handlers) {
            try { h(parsed); } catch { /* ignore handler errors */ }
          }
        }
      }
    };

    this.ws.onclose = (ev) => {
      if (this.intentionalClose || ev.code === 1000) return;
      this._scheduleReconnect();
    };

    this.ws.onerror = () => {
      // onerror is always followed by onclose; reconnect is handled there.
    };
  }

  private _scheduleReconnect() {
    this._clearReconnectTimer();
    const delay = Math.min(BASE_DELAY_MS * 2 ** this.reconnectAttempt, MAX_DELAY_MS);
    this.reconnectAttempt++;
    this.reconnectTimer = setTimeout(() => {
      if (!this.intentionalClose) {
        this._open();
      }
    }, delay);
  }

  private _clearReconnectTimer() {
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }
}

/** Singleton WS client shared across the app. */
export const wsClient = new WsClient();
