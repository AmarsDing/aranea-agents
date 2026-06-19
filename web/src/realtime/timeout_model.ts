/**
 * Layered Timeout Model — B3.
 *
 * Organizes the timeout/interval constants from `features/constants/timeouts.ts`
 * into a four-layer model with documented trigger conditions and cancellation
 * relationships.
 *
 * ┌─────────────────────────────────────────────────────────────────────┐
 * │ Layer 1: Connection                                                 │
 * │   WS heartbeat + run-stale detection + server health heartbeat      │
 * │   Trigger: WS connected / run started                               │
 * │   Cancel:  WS disconnected / run completed                          │
 * ├─────────────────────────────────────────────────────────────────────┤
 * │ Layer 2: Request (No-Timeout principle — disabled)                  │
 * │   Dispatch + turn-ack timeouts were removed in T1.5.                │
 * │   Tasks run until completion or user cancel.                        │
 * │   Trigger: N/A (removed)                                            │
 * │   Cancel:  N/A                                                      │
 * ├─────────────────────────────────────────────────────────────────────┤
 * │ Layer 3: Response (notification-only)                               │
 * │   First-byte notice + stall notice — never interrupt the run.       │
 * │   Trigger: run accepted (first-byte) / run event gap (stall)        │
 * │   Cancel:  first byte arrived / run completed                       │
 * ├─────────────────────────────────────────────────────────────────────┤
 * │ Layer 4: Session                                                    │
 * │   UI debounce / rebind delays / notification durations.             │
 * │   Trigger: session switch / message hydration / user action         │
 * │   Cancel: next session switch / debounce flush                      │
 * └─────────────────────────────────────────────────────────────────────┘
 *
 * Cancellation relationships:
 *   - Connection Layer 1 heartbeat cancels on disconnect.
 *   - Response Layer 3 first-byte cancels when the first byte arrives
 *     (onFirstByteArrived) or when the run is accepted (onRunAccepted
 *     restarts the timer).
 *   - Response Layer 3 stall check cancels on markSendingDone() or
 *     stopStreaming().
 *   - Session Layer 4 debounces cancel on next invocation (single-shot).
 */
import {
  WS_HEARTBEAT_INTERVAL_MS,
  WS_RUN_STALE_TIMEOUT_MS,
  WS_RECONNECT_BASE_DELAY_MS,
  WS_MAX_RECONNECT_DELAY_MS,
  HEARTBEAT_PING_INTERVAL_MS,
  HEARTBEAT_PONG_TIMEOUT_MS,
  HEARTBEAT_RECONNECT_BASE_DELAY_MS,
  HEARTBEAT_RECONNECT_MAX_DELAY_MS,
  HEARTBEAT_INITIAL_CONNECT_TIMEOUT_MS,
  CHAT_RUN_STALL_CHECK_INTERVAL_MS,
  CHAT_RUN_STALL_NOTIFY_THRESHOLD_MS,
  CHAT_STALL_NOTIFY_DURATION_MS,
  CHAT_FIRST_BYTE_NOTIFY_THRESHOLD_MS,
  CHAT_FIRST_BYTE_NOTIFY_DURATION_MS,
  CHAT_HYDRATE_DEBOUNCE_MS,
  CHAT_REBIND_DELAY_MS,
} from '../features/constants/timeouts';

/**
 * Layered timeout model.
 *
 * Each layer groups related timeouts by scope and lifecycle:
 * - `connection`: transport-level keepalive and health detection
 * - `request`: command dispatch and acknowledgment (disabled per No-Timeout)
 * - `response`: streaming progress notifications (notification-only)
 * - `session`: UI-level debounce and notification durations
 */
export const TIMEOUT_LAYERS = {
  /**
   * Layer 1: Connection-level timeouts.
   * Manage WS keepalive, run-stale detection, and server health.
   * Trigger: WS connected / run started.
   * Cancel: WS disconnected / run completed.
   */
  connection: {
    /** WS business ping interval — keeps the connection alive. */
    wsHeartbeatInterval: WS_HEARTBEAT_INTERVAL_MS,
    /** Run stale threshold — no run_heartbeat within this window means the run may be stuck. */
    wsRunStaleTimeout: WS_RUN_STALE_TIMEOUT_MS,
    /** WS reconnect base delay (exponential backoff start). */
    wsReconnectBaseDelay: WS_RECONNECT_BASE_DELAY_MS,
    /** WS reconnect max delay (exponential backoff cap). */
    wsReconnectMaxDelay: WS_MAX_RECONNECT_DELAY_MS,
    /** Server health-check ping interval. */
    heartbeatPingInterval: HEARTBEAT_PING_INTERVAL_MS,
    /** Server pong timeout — server considered offline after this. */
    heartbeatPongTimeout: HEARTBEAT_PONG_TIMEOUT_MS,
    /** Heartbeat reconnect base delay. */
    heartbeatReconnectBaseDelay: HEARTBEAT_RECONNECT_BASE_DELAY_MS,
    /** Heartbeat reconnect max delay. */
    heartbeatReconnectMaxDelay: HEARTBEAT_RECONNECT_MAX_DELAY_MS,
    /** Initial heartbeat connection timeout. */
    heartbeatInitialConnectTimeout: HEARTBEAT_INITIAL_CONNECT_TIMEOUT_MS,
  },

  /**
   * Layer 2: Request-level timeouts (DISABLED — No-Timeout principle, T1.5).
   *
   * Dispatch and turn-ack timeouts were removed. Tasks run until completion
   * or user cancel. The backend processes messages and sends run.status=running
   * as an early ack; failures arrive as error events.
   *
   * These fields are documented for reference only — they are NOT enforced.
   */
  request: {
    /** @deprecated Removed in T1.5 — No-Timeout principle. Tasks run until completion. */
    dispatchTimeout: null,
    /** @deprecated Removed in T1.5 — No-Timeout principle. Backend acks via run.status event. */
    turnAckTimeout: null,
  },

  /**
   * Layer 3: Response-level timeouts (notification-only).
   *
   * First-byte and stall notices inform the user but NEVER interrupt the run
   * or mark messages as failed. They are purely UX hints.
   *
   * Trigger: run accepted (first-byte) / run event gap (stall).
   * Cancel: first byte arrived / run completed / markSendingDone.
   */
  response: {
    /** First-byte notice threshold — show "model thinking" after this delay. */
    firstByteNotifyThreshold: CHAT_FIRST_BYTE_NOTIFY_THRESHOLD_MS,
    /** First-byte notice display duration. */
    firstByteNotifyDuration: CHAT_FIRST_BYTE_NOTIFY_DURATION_MS,
    /** Stall check polling interval. */
    stallCheckInterval: CHAT_RUN_STALL_CHECK_INTERVAL_MS,
    /** Stall notify threshold — show "please wait" after this delay without events. */
    stallNotifyThreshold: CHAT_RUN_STALL_NOTIFY_THRESHOLD_MS,
    /** Stall notice display duration. */
    stallNotifyDuration: CHAT_STALL_NOTIFY_DURATION_MS,
  },

  /**
   * Layer 4: Session-level timeouts.
   *
   * UI debounce, rebind delays, and notification durations. These manage
   * user-facing timing, not run lifecycle.
   *
   * Trigger: session switch / message hydration / user action.
   * Cancel: next session switch / debounce flush / notification timeout.
   */
  session: {
    /** Inbound message hydration debounce. */
    hydrateDebounce: CHAT_HYDRATE_DEBOUNCE_MS,
    /** Page-visible session rebind delay. */
    rebindDelay: CHAT_REBIND_DELAY_MS,
  },
} as const;

/** Connection-layer timeout keys. */
export type ConnectionTimeoutKey = keyof typeof TIMEOUT_LAYERS.connection;

/** Response-layer timeout keys. */
export type ResponseTimeoutKey = keyof typeof TIMEOUT_LAYERS.response;

/** Session-layer timeout keys. */
export type SessionTimeoutKey = keyof typeof TIMEOUT_LAYERS.session;
